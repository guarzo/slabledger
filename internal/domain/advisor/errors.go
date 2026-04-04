package advisor

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	ErrCodeMaxRoundsExceeded errors.ErrorCode = "ERR_ADVISOR_MAX_ROUNDS"
	ErrCodeUnsupportedType   errors.ErrorCode = "ERR_ADVISOR_UNSUPPORTED_TYPE"
)

func IsMaxRoundsExceeded(err error) bool { return errors.HasErrorCode(err, ErrCodeMaxRoundsExceeded) }
func IsUnsupportedType(err error) bool   { return errors.HasErrorCode(err, ErrCodeUnsupportedType) }
