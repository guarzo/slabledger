package favorites

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

// Domain-specific error codes for favorites
const (
	ErrCodeFavoriteNotFound      errors.ErrorCode = "ERR_FAV_NOT_FOUND"
	ErrCodeFavoriteAlreadyExists errors.ErrorCode = "ERR_FAV_ALREADY_EXISTS"
)

// Sentinel errors for favorites operations
var (
	// ErrFavoriteNotFound is returned when a favorite is not found
	ErrFavoriteNotFound = errors.NewAppError(ErrCodeFavoriteNotFound, "favorite not found")

	// ErrFavoriteAlreadyExists is returned when trying to add a duplicate favorite
	ErrFavoriteAlreadyExists = errors.NewAppError(ErrCodeFavoriteAlreadyExists, "favorite already exists")
)
