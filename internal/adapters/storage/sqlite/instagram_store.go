package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// InstagramConfig holds the stored Instagram connection configuration.
type InstagramConfig struct {
	AccessToken string
	IGUserID    string
	Username    string
	ExpiresAt   time.Time
	ConnectedAt time.Time
	IsConnected bool
}

// InstagramStore manages Instagram token persistence with encryption.
type InstagramStore struct {
	db        *sql.DB
	encryptor crypto.Encryptor
}

// NewInstagramStore creates a new Instagram store.
func NewInstagramStore(db *sql.DB, encryptor crypto.Encryptor) *InstagramStore {
	return &InstagramStore{db: db, encryptor: encryptor}
}

// Get returns the current Instagram configuration, or nil if not connected.
func (s *InstagramStore) Get(ctx context.Context) (*InstagramConfig, error) {
	var (
		encToken    string
		igUserID    string
		username    string
		expiresAt   string
		connectedAt string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT access_token, ig_user_id, username, token_expires_at, connected_at
		 FROM instagram_config WHERE id = 1`,
	).Scan(&encToken, &igUserID, &username, &expiresAt, &connectedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if encToken == "" || igUserID == "" {
		return nil, nil
	}

	token, err := s.encryptor.Decrypt(encToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt access token: %w", err)
	}

	cfg := &InstagramConfig{
		AccessToken: token,
		IGUserID:    igUserID,
		Username:    username,
		IsConnected: true,
	}
	var parseErr error
	cfg.ExpiresAt, parseErr = time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil {
		return nil, fmt.Errorf("parse token expiry %q: %w", expiresAt, parseErr)
	}
	cfg.ConnectedAt, _ = time.Parse(time.RFC3339, connectedAt) //nolint:errcheck // cosmetic field, best-effort
	return cfg, nil
}

// Save stores Instagram connection info. Upserts the singleton row.
func (s *InstagramStore) Save(ctx context.Context, token, igUserID, username string, expiresAt time.Time) error {
	encToken, err := s.encryptor.Encrypt(token)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO instagram_config (id, access_token, ig_user_id, username, token_expires_at, connected_at, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   access_token = excluded.access_token,
		   ig_user_id = excluded.ig_user_id,
		   username = excluded.username,
		   token_expires_at = excluded.token_expires_at,
		   connected_at = excluded.connected_at,
		   updated_at = excluded.updated_at`,
		encToken, igUserID, username, expiresAt.Format(time.RFC3339), now, now,
	)
	if err != nil {
		return fmt.Errorf("save instagram config: %w", err)
	}
	return nil
}

// UpdateToken updates just the access token and expiry (for refresh).
func (s *InstagramStore) UpdateToken(ctx context.Context, token string, expiresAt time.Time) error {
	encToken, err := s.encryptor.Encrypt(token)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE instagram_config SET access_token = ?, token_expires_at = ?, updated_at = ? WHERE id = 1`,
		encToken, expiresAt.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("update instagram token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("no instagram config found to update")
	}
	return nil
}

// GetToken returns token info in the format needed by instagram.TokenStore.
func (s *InstagramStore) GetToken(ctx context.Context) (accessToken, igUserID string, expiresAt time.Time, connected bool, err error) {
	cfg, err := s.Get(ctx)
	if err != nil {
		return "", "", time.Time{}, false, err
	}
	if cfg == nil || !cfg.IsConnected {
		return "", "", time.Time{}, false, nil
	}
	return cfg.AccessToken, cfg.IGUserID, cfg.ExpiresAt, true, nil
}

// Delete removes the Instagram connection.
func (s *InstagramStore) Delete(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM instagram_config WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("delete instagram config: %w", err)
	}
	return nil
}
