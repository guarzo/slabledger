package advisor

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	ErrCodeMaxRoundsExceeded errors.ErrorCode = "ERR_ADVISOR_MAX_ROUNDS"
	ErrCodeToolPanic         errors.ErrorCode = "ERR_ADVISOR_TOOL_PANIC"
	ErrCodeUnsupportedType   errors.ErrorCode = "ERR_ADVISOR_UNSUPPORTED_TYPE"
)

var (
	ErrMaxRoundsExceeded = errors.NewAppError(ErrCodeMaxRoundsExceeded, "exceeded maximum tool call rounds")
	ErrToolPanic         = errors.NewAppError(ErrCodeToolPanic, "tool executor panicked")
	ErrUnsupportedType   = errors.NewAppError(ErrCodeUnsupportedType, "unsupported factor data type")
)

func IsMaxRoundsExceeded(err error) bool { return errors.HasErrorCode(err, ErrCodeMaxRoundsExceeded) }
func IsToolPanic(err error) bool         { return errors.HasErrorCode(err, ErrCodeToolPanic) }
func IsUnsupportedType(err error) bool   { return errors.HasErrorCode(err, ErrCodeUnsupportedType) }
