package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
)

// CreateUser creates a new user
func (r *AuthRepository) CreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	query := `
		INSERT INTO users (google_id, username, email, avatar_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, google_id, username, email, avatar_url, is_admin, created_at, updated_at, last_login_at
	`

	now := time.Now()
	user := &auth.User{}

	var lastLoginAt sql.NullTime
	var avatarURLNull sql.NullString
	err := r.db.QueryRowContext(
		ctx,
		query,
		googleID,
		username,
		email,
		avatarURL,
		now,
		now,
	).Scan(
		&user.ID,
		&user.GoogleID,
		&user.Username,
		&user.Email,
		&avatarURLNull,
		&user.IsAdmin,
		&user.CreatedAt,
		&user.UpdatedAt,
		&lastLoginAt,
	)

	if err != nil {
		return nil, err
	}

	if avatarURLNull.Valid {
		user.AvatarURL = avatarURLNull.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return user, nil
}

// GetUserByGoogleID retrieves a user by Google user ID
func (r *AuthRepository) GetUserByGoogleID(ctx context.Context, googleID string) (*auth.User, error) {
	query := `
		SELECT id, google_id, username, email, avatar_url, is_admin, created_at, updated_at, last_login_at
		FROM users
		WHERE google_id = ?
	`

	user := &auth.User{}
	var lastLoginAt sql.NullTime
	var avatarURLNull sql.NullString

	err := r.db.QueryRowContext(ctx, query, googleID).Scan(
		&user.ID,
		&user.GoogleID,
		&user.Username,
		&user.Email,
		&avatarURLNull,
		&user.IsAdmin,
		&user.CreatedAt,
		&user.UpdatedAt,
		&lastLoginAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}
		return nil, err
	}

	if avatarURLNull.Valid {
		user.AvatarURL = avatarURLNull.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return user, nil
}

// GetUserByID retrieves a user by internal ID
func (r *AuthRepository) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	query := `
		SELECT id, google_id, username, email, avatar_url, is_admin, created_at, updated_at, last_login_at
		FROM users
		WHERE id = ?
	`

	user := &auth.User{}
	var lastLoginAt sql.NullTime
	var avatarURLNull sql.NullString

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.GoogleID,
		&user.Username,
		&user.Email,
		&avatarURLNull,
		&user.IsAdmin,
		&user.CreatedAt,
		&user.UpdatedAt,
		&lastLoginAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}
		return nil, err
	}

	if avatarURLNull.Valid {
		user.AvatarURL = avatarURLNull.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return user, nil
}

// UpdateUser updates user information
func (r *AuthRepository) UpdateUser(ctx context.Context, user *auth.User) error {
	query := `
		UPDATE users
		SET username = ?, email = ?, avatar_url = ?, updated_at = ?, last_login_at = ?
		WHERE id = ?
	`

	var lastLoginAt interface{}
	if user.LastLoginAt != nil {
		lastLoginAt = *user.LastLoginAt
	}

	result, err := r.db.ExecContext(
		ctx,
		query,
		user.Username,
		user.Email,
		user.AvatarURL,
		time.Now(),
		lastLoginAt,
		user.ID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return auth.ErrUserNotFound
	}

	return nil
}
