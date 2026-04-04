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

func IsUserNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodeUserNotFound) }
