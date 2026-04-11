package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
)

// IsEmailAllowed checks if an email is in the allowlist
func (r *AuthRepository) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	query := `SELECT COUNT(*) FROM allowed_emails WHERE email = ?`
	var count int
	if err := r.db.QueryRowContext(ctx, query, email).Scan(&count); err != nil {
		return false, fmt.Errorf("query allowed email: %w", err)
	}
	return count > 0, nil
}

// ListAllowedEmails returns all emails in the allowlist
func (r *AuthRepository) ListAllowedEmails(ctx context.Context) (_ []auth.AllowedEmail, err error) {
	query := `SELECT email, added_by, created_at, notes FROM allowed_emails ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query allowed emails: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var emails []auth.AllowedEmail
	for rows.Next() {
		var ae auth.AllowedEmail
		var addedBy sql.NullInt64
		var notes sql.NullString
		if err := rows.Scan(&ae.Email, &addedBy, &ae.CreatedAt, &notes); err != nil {
			return nil, fmt.Errorf("scan allowed email row: %w", err)
		}
		if addedBy.Valid {
			ae.AddedBy = &addedBy.Int64
		}
		if notes.Valid {
			ae.Notes = notes.String
		}
		emails = append(emails, ae)
	}
	return emails, rows.Err()
}

// AddAllowedEmail adds an email to the allowlist
func (r *AuthRepository) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	query := `INSERT INTO allowed_emails (email, added_by, created_at, notes) VALUES (?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET notes = excluded.notes, added_by = excluded.added_by`
	_, err := r.db.ExecContext(ctx, query, email, addedBy, time.Now(), notes)
	if err != nil {
		return fmt.Errorf("add allowed email: %w", err)
	}
	return nil
}

// RemoveAllowedEmail removes an email from the allowlist
func (r *AuthRepository) RemoveAllowedEmail(ctx context.Context, email string) error {
	query := `DELETE FROM allowed_emails WHERE email = ?`
	_, err := r.db.ExecContext(ctx, query, email)
	if err != nil {
		return fmt.Errorf("remove allowed email: %w", err)
	}
	return nil
}

// ListUsers returns all registered users
func (r *AuthRepository) ListUsers(ctx context.Context) (_ []auth.User, err error) {
	query := `SELECT id, google_id, username, email, avatar_url, is_admin, created_at, updated_at, last_login_at
		FROM users ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var users []auth.User
	for rows.Next() {
		var u auth.User
		var avatarURL sql.NullString
		var lastLoginAt sql.NullTime
		if err := rows.Scan(&u.ID, &u.GoogleID, &u.Username, &u.Email, &avatarURL,
			&u.IsAdmin, &u.CreatedAt, &u.UpdatedAt, &lastLoginAt); err != nil {
			return nil, fmt.Errorf("scan user row: %w", err)
		}
		if avatarURL.Valid {
			u.AvatarURL = avatarURL.String
		}
		if lastLoginAt.Valid {
			u.LastLoginAt = &lastLoginAt.Time
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// SetUserAdmin sets the admin flag on a user
func (r *AuthRepository) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	query := `UPDATE users SET is_admin = ?, updated_at = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, isAdmin, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("update user admin flag: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}
