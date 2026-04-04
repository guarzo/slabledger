package auth

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	ErrCodeUserNotFound errors.ErrorCode = "ERR_AUTH_USER_NOT_FOUND"
)

var (
	ErrUserNotFound = errors.NewAppError(ErrCodeUserNotFound, "user not found")
)

// NewErrUserNotFound returns a fresh copy of ErrUserNotFound that is safe to
// mutate (e.g. WithContext, WithHTTPStatus) without affecting the sentinel.
func NewErrUserNotFound() *errors.AppError { return ErrUserNotFound.Clone() }

func IsUserNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodeUserNotFound) }
