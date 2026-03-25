package errors

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// ErrorCode represents a categorized error type
type ErrorCode string

const (
	// Validation Errors (ERR_VALIDATION)
	ErrCodeValidation ErrorCode = "ERR_VALIDATION"

	// Provider Errors (ERR_PROV_*)
	ErrCodeProviderUnavailable ErrorCode = "ERR_PROV_UNAVAILABLE"
	ErrCodeProviderAuth        ErrorCode = "ERR_PROV_AUTH"
	ErrCodeProviderRateLimit   ErrorCode = "ERR_PROV_RATE_LIMIT"
	ErrCodeProviderTimeout     ErrorCode = "ERR_PROV_TIMEOUT"
	ErrCodeProviderNotFound    ErrorCode = "ERR_PROV_NOT_FOUND"
	ErrCodeProviderInvalidResp ErrorCode = "ERR_PROV_INVALID_RESP"
	ErrCodeProviderInvalidReq  ErrorCode = "ERR_PROV_INVALID_REQ"
	ErrCodeProviderCircuitOpen ErrorCode = "ERR_PROV_CIRCUIT_OPEN"

	// Configuration Errors (ERR_CFG_*)
	ErrCodeConfigInvalid    ErrorCode = "ERR_CFG_INVALID"
	ErrCodeConfigMissing    ErrorCode = "ERR_CFG_MISSING"
	ErrCodeConfigValidation ErrorCode = "ERR_CFG_VALIDATION"

	// Cache Errors (ERR_CACHE_*)
	ErrCodeCacheCorrupted   ErrorCode = "ERR_CACHE_CORRUPTED"
	ErrCodeCacheWriteFailed ErrorCode = "ERR_CACHE_WRITE_FAILED"

	// Network Errors (ERR_NET_*)
	ErrCodeNetworkUnavailable ErrorCode = "ERR_NET_UNAVAILABLE"
	ErrCodeNetworkTimeout     ErrorCode = "ERR_NET_TIMEOUT"

	// Session Errors (ERR_SESSION_*)
	ErrCodeSessionExpired ErrorCode = "ERR_SESSION_EXPIRED"

	// Generic Errors (ERR_GENERIC_*)
	ErrCodeInternal ErrorCode = "ERR_INTERNAL"
	ErrCodeNotFound ErrorCode = "ERR_NOT_FOUND"
)

const (
	// maxCauseMessageLength is the maximum length for cause error messages before truncation.
	// Long cause messages (e.g., HTML responses) are truncated to prevent log bloat.
	maxCauseMessageLength = 150
)

// AppError is a structured error with code, message, and context
type AppError struct {
	Code           ErrorCode
	Message        string
	Cause          error
	Context        map[string]any
	HTTPStatusCode int // Explicit HTTP status code (0 if not HTTP-related)
}

// Error implements the error interface
// Returns a concise error message suitable for logs
func (e *AppError) Error() string {
	msg := fmt.Sprintf("[%s] %s", e.Code, e.Message)

	// For structured logging, include cause concisely
	if e.Cause != nil {
		causeMsg := e.Cause.Error()
		// Truncate very long cause messages (e.g., HTML responses)
		// Use rune-aware truncation to avoid splitting multi-byte UTF-8 characters
		if len(causeMsg) > maxCauseMessageLength {
			truncIndex := 0
			for i, r := range causeMsg {
				runeLen := utf8.RuneLen(r)
				if i+runeLen > maxCauseMessageLength {
					break
				}
				truncIndex = i + runeLen
			}
			causeMsg = causeMsg[:truncIndex] + "..."
		}
		msg += fmt.Sprintf(": %s", causeMsg)
	}

	return msg
}

// HTTPStatus extracts HTTP status code from the error.
// Priority:
// 1. Explicit HTTPStatusCode field
// 2. Context map "http_status" key
// Returns 0 if no valid HTTP status is found.
func (e *AppError) HTTPStatus() int {
	// 1. Check explicit HTTPStatusCode field
	if e.HTTPStatusCode != 0 && isValidHTTPStatus(e.HTTPStatusCode) {
		return e.HTTPStatusCode
	}

	// 2. Check Context map for http_status
	if e.Context != nil {
		if status, ok := e.Context["http_status"]; ok {
			switch v := status.(type) {
			case int:
				if isValidHTTPStatus(v) {
					return v
				}
			case int64:
				if v >= 100 && v <= 599 {
					return int(v)
				}
			case float64:
				iv := int(v)
				if v == float64(iv) && isValidHTTPStatus(iv) {
					return iv
				}
			}
		}
	}

	return 0
}

// isValidHTTPStatus validates that a status code is in the valid HTTP range (100-599)
func isValidHTTPStatus(status int) bool {
	return status >= 100 && status <= 599
}

// Unwrap returns the cause error for errors.Is/As
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithContext adds context data to the error
func (e *AppError) WithContext(key string, value any) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// WithHTTPStatus sets the explicit HTTP status code for the error.
// Only valid HTTP status codes (100-599) are accepted.
// Invalid status codes are ignored and HTTPStatusCode is set to 0.
func (e *AppError) WithHTTPStatus(statusCode int) *AppError {
	if isValidHTTPStatus(statusCode) {
		e.HTTPStatusCode = statusCode
	} else {
		e.HTTPStatusCode = 0
	}
	return e
}

// NewAppError creates a new AppError
func NewAppError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// HasErrorCode checks if an error (or any error in its chain) has the specified error code.
// This is useful for checking provider errors like ErrCodeProviderCircuitOpen or ErrCodeProviderRateLimit
// without relying on string matching in error messages.
// It walks the entire error chain to find any AppError with the matching code.
func HasErrorCode(err error, code ErrorCode) bool {
	if err == nil {
		return false
	}

	// Walk the error chain checking each error for the code
	for e := err; e != nil; e = errors.Unwrap(e) {
		var appErr *AppError
		if errors.As(e, &appErr) && appErr.Code == code {
			return true
		}
	}
	return false
}
