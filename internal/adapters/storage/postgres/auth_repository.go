package postgres

import (
	"database/sql"
	"errors"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)

var (
	// ErrTokenNotFound is returned when tokens are not found
	ErrTokenNotFound = errors.New("tokens not found")
	// ErrSessionNotFound is returned when a session is not found
	ErrSessionNotFound = errors.New("session not found")
)

// AuthRepository implements auth.Repository using Postgres
type AuthRepository struct {
	db        *sql.DB
	encryptor crypto.Encryptor
}

// NewAuthRepository creates a new Postgres auth repository
func NewAuthRepository(db *sql.DB, encryptor crypto.Encryptor) *AuthRepository {
	return &AuthRepository{
		db:        db,
		encryptor: encryptor,
	}
}

// Ensure AuthRepository implements auth.Repository
var _ auth.Repository = (*AuthRepository)(nil)
