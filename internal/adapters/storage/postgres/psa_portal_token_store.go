package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// PSAPortalTokenStore persists the single most-recent portal access token,
// encrypted at rest. The id column is locked to 1 by a CHECK constraint
// (migration 000016), so the table holds at most one row.
type PSAPortalTokenStore struct {
	db  *sql.DB
	enc crypto.Encryptor
}

func NewPSAPortalTokenStore(db *sql.DB, enc crypto.Encryptor) *PSAPortalTokenStore {
	return &PSAPortalTokenStore{db: db, enc: enc}
}

// PSAPortalTokenStore structurally satisfies psaportal.TokenStore and
// psaportal.TokenRepository; that coupling is enforced at wiring time in
// cmd/slabledger (where both are passed to NewStoredTokenProvider/NewHarvester),
// so this storage adapter avoids importing the client adapter (hexagonal rule).

// CurrentToken returns the stored token (decrypted) and its expiry.
// No row yet → ("", zero time, nil).
func (s *PSAPortalTokenStore) CurrentToken(ctx context.Context) (string, time.Time, error) {
	const q = `SELECT access_token, expires_at FROM psa_portal_token WHERE id = 1`
	var enc string
	var exp time.Time
	err := s.db.QueryRowContext(ctx, q).Scan(&enc, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("psa_portal_token: query: %w", err)
	}
	tok, err := s.enc.Decrypt(enc)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("psa_portal_token: decrypt: %w", err)
	}
	return tok, exp, nil
}

// SaveToken upserts the single token row (encrypting the token at rest).
func (s *PSAPortalTokenStore) SaveToken(ctx context.Context, token string, expiresAt time.Time) error {
	enc, err := s.enc.Encrypt(token)
	if err != nil {
		return fmt.Errorf("psa_portal_token: encrypt: %w", err)
	}
	const q = `
		INSERT INTO psa_portal_token (id, access_token, expires_at, updated_at)
		VALUES (1, $1, $2, now())
		ON CONFLICT (id) DO UPDATE
		   SET access_token = EXCLUDED.access_token,
		       expires_at   = EXCLUDED.expires_at,
		       updated_at   = now()`
	if _, err := s.db.ExecContext(ctx, q, enc, expiresAt); err != nil {
		return fmt.Errorf("psa_portal_token: upsert: %w", err)
	}
	return nil
}
