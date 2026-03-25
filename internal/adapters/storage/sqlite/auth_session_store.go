package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
)

// hashSessionID returns the SHA-256 hex digest of a session ID.
// Session tokens are stored hashed so a database leak (including admin backups)
// does not expose replayable session credentials.
func hashSessionID(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// CreateSession creates a new user session.
// The session ID stored in the database is a SHA-256 hash of the plaintext token.
func (r *AuthRepository) CreateSession(ctx context.Context, session *auth.Session) error {
	query := `
		INSERT INTO user_sessions (id, user_id, expires_at, created_at, last_accessed_at, user_agent, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		hashSessionID(session.ID),
		session.UserID,
		session.ExpiresAt,
		session.CreatedAt,
		session.LastAccessedAt,
		session.UserAgent,
		session.IPAddress,
	)

	return err
}

// GetSession retrieves a session by ID.
// The caller passes the plaintext session token; it is hashed before lookup.
// For backward compatibility with pre-hashing sessions, falls back to a
// plaintext lookup and migrates the row to the hashed format on success.
func (r *AuthRepository) GetSession(ctx context.Context, sessionID string) (*auth.Session, error) {
	query := `
		SELECT id, user_id, expires_at, created_at, last_accessed_at, user_agent, ip_address
		FROM user_sessions
		WHERE id = ? AND expires_at > ?
	`

	hashedID := hashSessionID(sessionID)
	session := &auth.Session{}
	err := r.db.QueryRowContext(ctx, query, hashedID, time.Now()).Scan(
		&session.ID,
		&session.UserID,
		&session.ExpiresAt,
		&session.CreatedAt,
		&session.LastAccessedAt,
		&session.UserAgent,
		&session.IPAddress,
	)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		// Found by hashed ID — return plaintext ID as callers expect.
		session.ID = sessionID
		return session, nil
	}

	// Fallback: try plaintext lookup for pre-migration sessions.
	session = &auth.Session{}
	err = r.db.QueryRowContext(ctx, query, sessionID, time.Now()).Scan(
		&session.ID,
		&session.UserID,
		&session.ExpiresAt,
		&session.CreatedAt,
		&session.LastAccessedAt,
		&session.UserAgent,
		&session.IPAddress,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	// Migrate the row to hashed format so future lookups use the fast path.
	// Use a transaction so user_sessions and user_tokens stay consistent.
	if tx, txErr := r.db.BeginTx(ctx, nil); txErr == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE user_sessions SET id = ? WHERE id = ?`, hashedID, sessionID); err != nil {
			//nolint:errcheck // best-effort migration rollback
			tx.Rollback()
		} else if _, err := tx.ExecContext(ctx, `UPDATE user_tokens SET session_id = ? WHERE session_id = ?`, hashedID, sessionID); err != nil {
			//nolint:errcheck // best-effort migration rollback
			tx.Rollback()
		} else if err := tx.Commit(); err != nil {
			//nolint:errcheck // commit failed; rollback is best-effort
			tx.Rollback()
		}
	}

	session.ID = sessionID
	return session, nil
}

// resolveSessionID returns the database ID for a session, trying the hashed
// form first and falling back to the raw plaintext ID for legacy rows.
func (r *AuthRepository) resolveSessionID(ctx context.Context, sessionID string) (string, error) {
	hashedID := hashSessionID(sessionID)

	var exists int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_sessions WHERE id = ?`, hashedID).Scan(&exists)
	if err == nil {
		return hashedID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	// Fallback: check for legacy plaintext ID.
	err = r.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_sessions WHERE id = ?`, sessionID).Scan(&exists)
	if err == nil {
		return sessionID, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSessionNotFound
	}
	return "", err
}

// UpdateSessionAccess updates the last accessed time of a session.
// The caller passes the plaintext session token; it is hashed before lookup.
// Falls back to plaintext lookup for pre-migration legacy sessions.
func (r *AuthRepository) UpdateSessionAccess(ctx context.Context, sessionID string) error {
	dbID, err := r.resolveSessionID(ctx, sessionID)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_sessions
		SET last_accessed_at = ?
		WHERE id = ?
	`

	_, err = r.db.ExecContext(ctx, query, time.Now(), dbID)
	return err
}

// DeleteSession deletes a session.
// The caller passes the plaintext session token; it is hashed before lookup.
// Falls back to plaintext lookup for pre-migration legacy sessions.
func (r *AuthRepository) DeleteSession(ctx context.Context, sessionID string) error {
	dbID, err := r.resolveSessionID(ctx, sessionID)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE id = ?`, dbID)
	return err
}

// DeleteExpiredSessions deletes all expired sessions
func (r *AuthRepository) DeleteExpiredSessions(ctx context.Context) (int, error) {
	query := `DELETE FROM user_sessions WHERE expires_at <= ?`

	result, err := r.db.ExecContext(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(rows), nil
}
