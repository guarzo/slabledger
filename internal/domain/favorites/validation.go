package favorites

import (
	stderrors "errors"
	"strings"
	"unicode/utf8"

	"github.com/guarzo/slabledger/internal/domain/errors"
)

// Validation constants
const (
	MaxCardNameLength   = 200
	MaxSetNameLength    = 100
	MaxCardNumberLength = 50
	MaxImageURLLength   = 500
	MaxNotesLength      = 1000
)

// Validation errors for favorite input
var (
	ErrInvalidInput      = errors.NewAppError(errors.ErrCodeValidation, "nil input")
	ErrCardNameRequired  = errors.NewAppError(errors.ErrCodeValidation, "card_name is required")
	ErrSetNameRequired   = errors.NewAppError(errors.ErrCodeValidation, "set_name is required")
	ErrCardNameTooLong   = errors.NewAppError(errors.ErrCodeValidation, "card_name exceeds maximum length")
	ErrSetNameTooLong    = errors.NewAppError(errors.ErrCodeValidation, "set_name exceeds maximum length")
	ErrCardNumberTooLong = errors.NewAppError(errors.ErrCodeValidation, "card_number exceeds maximum length")
	ErrImageURLTooLong   = errors.NewAppError(errors.ErrCodeValidation, "image_url exceeds maximum length")
	ErrNotesTooLong      = errors.NewAppError(errors.ErrCodeValidation, "notes exceeds maximum length")
)

// ValidateAndNormalizeInput validates a FavoriteInput and returns an error if invalid.
// It trims all string fields and assigns them back so the stored struct matches what was validated.
func ValidateAndNormalizeInput(input *FavoriteInput) error {
	// Guard against nil input to avoid panic
	if input == nil {
		return ErrInvalidInput
	}

	// Trim all string fields and assign back for consistent storage
	input.CardName = strings.TrimSpace(input.CardName)
	input.SetName = strings.TrimSpace(input.SetName)
	input.CardNumber = strings.TrimSpace(input.CardNumber)
	input.ImageURL = strings.TrimSpace(input.ImageURL)
	input.Notes = strings.TrimSpace(input.Notes)

	// Required fields
	if input.CardName == "" {
		return ErrCardNameRequired
	}

	if input.SetName == "" {
		return ErrSetNameRequired
	}

	// Length checks using utf8.RuneCountInString for proper Unicode character counting
	if utf8.RuneCountInString(input.CardName) > MaxCardNameLength {
		return ErrCardNameTooLong
	}
	if utf8.RuneCountInString(input.SetName) > MaxSetNameLength {
		return ErrSetNameTooLong
	}
	if utf8.RuneCountInString(input.CardNumber) > MaxCardNumberLength {
		return ErrCardNumberTooLong
	}
	if utf8.RuneCountInString(input.ImageURL) > MaxImageURLLength {
		return ErrImageURLTooLong
	}
	if utf8.RuneCountInString(input.Notes) > MaxNotesLength {
		return ErrNotesTooLong
	}

	return nil
}

// IsValidationError returns true if the error is a validation error
func IsValidationError(err error) bool {
	var appErr *errors.AppError
	if stderrors.As(err, &appErr) {
		return appErr.Code == errors.ErrCodeValidation
	}
	return false
}
