package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
)

// StoreTokens stores encrypted OAuth tokens for a user and session (multi-device support)
func (r *AuthRepository) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	// Encrypt tokens
	encryptedAccess, err := r.encryptor.Encrypt(tokens.AccessToken)
	if err != nil {
		return err
	}

	encryptedRefresh, err := r.encryptor.Encrypt(tokens.RefreshToken)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO user_tokens (user_id, session_id, access_token, refresh_token, token_type, expires_at, scope, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_type = excluded.token_type,
			expires_at = excluded.expires_at,
			scope = excluded.scope,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	_, err = r.db.ExecContext(
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
	)

	return err
}

// GetTokens retrieves and decrypts OAuth tokens for a user and session
func (r *AuthRepository) GetTokens(ctx context.Context, userID int64, sessionID string) (*auth.UserTokens, error) {
	query := `
		SELECT access_token, refresh_token, token_type, expires_at, scope
		FROM user_tokens
		WHERE user_id = ? AND session_id = ?
	`

	var encryptedAccess, encryptedRefresh string
	tokens := &auth.UserTokens{}

	err := r.db.QueryRowContext(ctx, query, userID, hashSessionID(sessionID)).Scan(
		&encryptedAccess,
		&encryptedRefresh,
		&tokens.TokenType,
		&tokens.ExpiresAt,
		&tokens.Scope,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}

	// Decrypt tokens
	tokens.AccessToken, err = r.encryptor.Decrypt(encryptedAccess)
	if err != nil {
		return nil, err
	}

	tokens.RefreshToken, err = r.encryptor.Decrypt(encryptedRefresh)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetTokensByUserID retrieves the most recent tokens for a user (for backward compatibility)
func (r *AuthRepository) GetTokensByUserID(ctx context.Context, userID int64) (*auth.UserTokens, error) {
	query := `
		SELECT access_token, refresh_token, token_type, expires_at, scope
		FROM user_tokens
		WHERE user_id = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`

	var encryptedAccess, encryptedRefresh string
	tokens := &auth.UserTokens{}

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&encryptedAccess,
		&encryptedRefresh,
		&tokens.TokenType,
		&tokens.ExpiresAt,
		&tokens.Scope,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}

	// Decrypt tokens
	tokens.AccessToken, err = r.encryptor.Decrypt(encryptedAccess)
	if err != nil {
		return nil, err
	}

	tokens.RefreshToken, err = r.encryptor.Decrypt(encryptedRefresh)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// UpdateTokens updates OAuth tokens for a user and session
func (r *AuthRepository) UpdateTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	// Encrypt tokens
	encryptedAccess, err := r.encryptor.Encrypt(tokens.AccessToken)
	if err != nil {
		return err
	}

	encryptedRefresh, err := r.encryptor.Encrypt(tokens.RefreshToken)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_tokens
		SET access_token = ?, refresh_token = ?, token_type = ?, expires_at = ?, scope = ?, updated_at = ?
		WHERE user_id = ? AND session_id = ?
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		encryptedAccess,
		encryptedRefresh,
		tokens.TokenType,
		tokens.ExpiresAt,
		tokens.Scope,
		time.Now(),
		userID,
		hashSessionID(sessionID),
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// DeleteTokens deletes OAuth tokens for a user and session
func (r *AuthRepository) DeleteTokens(ctx context.Context, userID int64, sessionID string) error {
	query := `DELETE FROM user_tokens WHERE user_id = ? AND session_id = ?`

	result, err := r.db.ExecContext(ctx, query, userID, hashSessionID(sessionID))
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// DeleteAllUserTokens deletes all OAuth tokens for a user (all devices)
func (r *AuthRepository) DeleteAllUserTokens(ctx context.Context, userID int64) error {
	query := `DELETE FROM user_tokens WHERE user_id = ?`

	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}
