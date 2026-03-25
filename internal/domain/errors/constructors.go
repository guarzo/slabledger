// Package errors provides typed error constructors for the application.
//
// Context Exemption: The error constructor functions in this package (e.g., ProviderUnavailable,
// ConfigMissing, ValidationError, etc.) intentionally do not accept context.Context as a parameter.
// This is a deliberate design decision because:
//   - Error constructors are pure value builders that create immutable error values
//   - They perform no I/O, no cancellable operations, and no blocking work
//   - They execute in nanoseconds and cannot benefit from cancellation
//   - Adding context would add unnecessary API complexity without functional benefit
//
// See: https://go.dev/blog/context (context is for request-scoped data and cancellation)
package errors

import (
	"fmt"
)

// Provider Errors

func ProviderUnavailable(provider string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeProviderUnavailable,
		Message: fmt.Sprintf("Provider '%s' is unavailable", provider),
		Cause:   cause,
	}
}

func ProviderAuthFailed(provider string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeProviderAuth,
		Message: fmt.Sprintf("Authentication failed for provider '%s'", provider),
		Cause:   cause,
	}
}

func ProviderRateLimited(provider string, resetTime string) *AppError {
	err := &AppError{
		Code:    ErrCodeProviderRateLimit,
		Message: fmt.Sprintf("Rate limit exceeded for provider '%s'", provider),
	}
	if resetTime != "" {
		err.Context = map[string]any{"reset_time": resetTime}
	}
	return err
}

func ProviderTimeout(provider string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeProviderTimeout,
		Message: fmt.Sprintf("Request timeout for provider '%s'", provider),
		Cause:   cause,
	}
}

func ProviderNotFound(provider string, resource string) *AppError {
	return &AppError{
		Code:    ErrCodeProviderNotFound,
		Message: fmt.Sprintf("Resource '%s' not found in provider '%s'", resource, provider),
	}
}

func ProviderInvalidRequest(provider string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeProviderInvalidReq,
		Message: fmt.Sprintf("Invalid request to provider '%s'", provider),
		Cause:   cause,
	}
}

func ProviderInvalidResponse(provider string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeProviderInvalidResp,
		Message: fmt.Sprintf("Invalid response from provider '%s'", provider),
		Cause:   cause,
	}
}

func ProviderCircuitOpen(provider string) *AppError {
	return &AppError{
		Code:    ErrCodeProviderCircuitOpen,
		Message: fmt.Sprintf("Circuit breaker is open for provider '%s'", provider),
	}
}

// Configuration Errors

func ConfigInvalid(field string, value any, reason string) *AppError {
	return &AppError{
		Code:    ErrCodeConfigInvalid,
		Message: fmt.Sprintf("Invalid configuration for '%s': %s", field, reason),
		Context: map[string]any{"field": field, "value": value},
	}
}

func ConfigMissing(field string, envVar string) *AppError {
	ctx := map[string]any{"field": field}
	if envVar != "" {
		ctx["env_var"] = envVar
	}
	return &AppError{
		Code:    ErrCodeConfigMissing,
		Message: fmt.Sprintf("Required configuration '%s' is missing", field),
		Context: ctx,
	}
}

// Cache Errors

func CacheCorrupted(path string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeCacheCorrupted,
		Message: fmt.Sprintf("Cache file corrupted: %s", path),
		Cause:   cause,
		Context: map[string]any{"path": path},
	}
}

func CacheWriteFailed(key string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeCacheWriteFailed,
		Message: fmt.Sprintf("Failed to write cache for key: %s", key),
		Cause:   cause,
		Context: map[string]any{"key": key},
	}
}

// Validation Errors

// ValidationError creates a typed error for input validation failures.
// Use this for invalid parameters, empty required fields, or constraint violations.
func ValidationError(field string, reason string) *AppError {
	return &AppError{
		Code:    ErrCodeValidation,
		Message: fmt.Sprintf("Validation failed for '%s': %s", field, reason),
		Context: map[string]any{"field": field},
	}
}

// Resource Errors

// NotFoundError creates a typed error for resource not found scenarios.
// Use this when a requested resource (job, snapshot, record, etc.) cannot be found.
func NotFoundError(resourceType string, resourceID string) *AppError {
	return &AppError{
		Code:    ErrCodeNotFound,
		Message: fmt.Sprintf("%s not found: %s", resourceType, resourceID),
		Context: map[string]any{"resource_type": resourceType, "resource_id": resourceID},
	}
}

// Session Errors

// SessionExpired creates a typed error for expired user sessions.
func SessionExpired() *AppError {
	return &AppError{
		Code:    ErrCodeSessionExpired,
		Message: "session expired",
	}
}

// Storage Errors

// StorageError creates a typed error for database/storage operation failures.
func StorageError(operation string, cause error) *AppError {
	return &AppError{
		Code:    ErrCodeInternal,
		Message: fmt.Sprintf("Storage operation failed: %s", operation),
		Cause:   cause,
	}
}
