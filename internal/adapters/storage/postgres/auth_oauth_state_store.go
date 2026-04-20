package postgres

import (
	"context"
	"fmt"
	"time"
)

// StoreOAuthState stores a one-time state token with expiration for CSRF protection.
func (r *AuthRepository) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	query := `INSERT INTO oauth_states (state, expires_at) VALUES ($1, $2)`

	_, err := r.db.ExecContext(ctx, query, state, expiresAt)
	if err != nil {
		return fmt.Errorf("store oauth state: %w", err)
	}
	return nil
}

// ConsumeOAuthState atomically consumes and deletes a state token.
// Returns true if the state was valid and not expired, false otherwise.
// This is atomic: the DELETE will only affect rows where state matches AND expires_at > now.
func (r *AuthRepository) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	query := `DELETE FROM oauth_states WHERE state = $1 AND expires_at > $2`

	result, err := r.db.ExecContext(ctx, query, state, time.Now())
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	// If rows == 1, the state was valid and consumed
	// If rows == 0, the state was invalid, expired, or already consumed
	return rows > 0, nil
}

// CleanupExpiredOAuthStates removes expired state tokens.
func (r *AuthRepository) CleanupExpiredOAuthStates(ctx context.Context) (int, error) {
	query := `DELETE FROM oauth_states WHERE expires_at <= $1`

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
