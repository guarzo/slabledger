package social

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	ErrCodePostNotFound      errors.ErrorCode = "ERR_POST_NOT_FOUND"
	ErrCodeNotConfigured     errors.ErrorCode = "ERR_NOT_CONFIGURED"
	ErrCodeNotPublishable    errors.ErrorCode = "ERR_NOT_PUBLISHABLE"
	ErrCodeInsufficientCards errors.ErrorCode = "ERR_INSUFFICIENT_CARDS"
)

var (
	ErrPostNotFound      = errors.NewAppError(ErrCodePostNotFound, "post not found")
	ErrNotConfigured     = errors.NewAppError(ErrCodeNotConfigured, "instagram publishing not configured")
	ErrNotPublishable    = errors.NewAppError(ErrCodeNotPublishable, "cannot publish: caption not ready")
	ErrInsufficientCards = errors.NewAppError(ErrCodeInsufficientCards, "insufficient cards for post")
)

func IsPostNotFound(err error) bool      { return errors.HasErrorCode(err, ErrCodePostNotFound) }
func IsNotConfigured(err error) bool     { return errors.HasErrorCode(err, ErrCodeNotConfigured) }
func IsNotPublishable(err error) bool    { return errors.HasErrorCode(err, ErrCodeNotPublishable) }
func IsInsufficientCards(err error) bool { return errors.HasErrorCode(err, ErrCodeInsufficientCards) }
