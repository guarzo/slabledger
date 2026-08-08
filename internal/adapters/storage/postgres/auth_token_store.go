package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
)

// StoreTokens stores encrypted OAuth tokens for a user and session (multi-device support)
func (r *AuthRepository) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	// Encrypt tokens
	encryptedAccess, err := r.encryptor.Encrypt(tokens.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}

	encryptedRefresh, err := r.encryptor.Encrypt(tokens.RefreshToken)
	if err != nil {
		return fmt.Errorf("encrypt refresh token: %w", err)
	}

	query := `
		INSERT INTO user_tokens (user_id, session_id, access_token, refresh_token, token_type, expires_at, scope, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(session_id) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_type = excluded.token_type,
			expires_at = excluded.expires_at,
			scope = excluded.scope,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	if _, err := r.db.ExecContext(
		ctx,
		query,
		userID,
		hashSessionID(sessionID),
		encryptedAccess,
		encryptedRefresh,
		tokens.TokenType,
		tokens.ExpiresAt,
		tokens.Scope,
		now,
		now,
	); err != nil {
		return fmt.Errorf("upsert user tokens for user %d: %w", userID, err)
	}
	return nil
}
