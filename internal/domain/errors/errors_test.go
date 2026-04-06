package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		contains []string
	}{
		{
			name: "basic error",
			err: &AppError{
				Code:    ErrCodeProviderAuth,
				Message: "Authentication failed",
			},
			contains: []string{"ERR_PROV_AUTH", "Authentication failed"},
		},
		{
			name: "error with cause truncated",
			err: &AppError{
				Code:    ErrCodeProviderUnavailable,
				Message: "Provider down",
				Cause:   fmt.Errorf("HTTP 500"),
			},
			contains: []string{"ERR_PROV_UNAVAILABLE", "Provider down", "HTTP 500"},
		},
		{
			name: "error with long cause truncated",
			err: &AppError{
				Code:    ErrCodeCacheCorrupted,
				Message: "Cache corrupted",
				Cause:   fmt.Errorf("%s", strings.Repeat("x", 200)),
			},
			contains: []string{"ERR_CACHE_CORRUPTED", "Cache corrupted", "..."},
		},
		{
			name: "error with multi-byte UTF-8 cause truncated",
			err: &AppError{
				Code:    ErrCodeProviderInvalidResp,
				Message: "Invalid response",
				Cause:   fmt.Errorf("Error: %s", strings.Repeat("你好世界", 50)), // Multi-byte UTF-8 characters
			},
			contains: []string{"ERR_PROV_INVALID_RESP", "Invalid response", "..."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, expected := range tt.contains {
				if !strings.Contains(got, expected) {
					t.Errorf("Error() missing expected text %q\nGot: %s", expected, got)
				}
			}
			// Error() should be concise (no "Suggestion:" or "Caused by:" labels)
			if strings.Contains(got, "Suggestion:") {
				t.Error("Error() should not contain 'Suggestion:'")
			}
			if strings.Contains(got, "Caused by:") {
				t.Error("Error() should not contain 'Caused by:'")
			}
		})
	}
}

func TestAppError_HTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		expected int
	}{
		{
			name:     "no cause",
			err:      &AppError{Code: ErrCodeProviderAuth, Message: "Auth failed"},
			expected: 0,
		},
		{
			name:     "cause without status fields",
			err:      &AppError{Code: ErrCodeProviderTimeout, Message: "Timeout", Cause: fmt.Errorf("connection timeout")},
			expected: 0,
		},

		// Explicit HTTPStatusCode field
		{
			name:     "explicit status code",
			err:      &AppError{Code: ErrCodeProviderUnavailable, Message: "Down", HTTPStatusCode: 502},
			expected: 502,
		},
		{
			name:     "explicit status code overrides context",
			err:      &AppError{Code: ErrCodeProviderUnavailable, Message: "Down", HTTPStatusCode: 502, Context: map[string]any{"http_status": 503}},
			expected: 502,
		},

		// Context map
		{
			name:     "context with int status",
			err:      &AppError{Code: ErrCodeProviderNotFound, Message: "Not found", Context: map[string]any{"http_status": 404}},
			expected: 404,
		},
		{
			name:     "context with int64 status",
			err:      &AppError{Code: ErrCodeProviderUnavailable, Message: "Down", Context: map[string]any{"http_status": int64(503)}},
			expected: 503,
		},
		{
			name:     "context with float64 status",
			err:      &AppError{Code: ErrCodeProviderAuth, Message: "Auth failed", Context: map[string]any{"http_status": float64(401)}},
			expected: 401,
		},

		// Validation
		{
			name:     "invalid status code too low",
			err:      &AppError{Code: ErrCodeProviderAuth, Message: "Auth failed", HTTPStatusCode: 99},
			expected: 0,
		},
		{
			name:     "invalid status code too high",
			err:      &AppError{Code: ErrCodeProviderAuth, Message: "Auth failed", HTTPStatusCode: 600},
			expected: 0,
		},
		{
			name:     "invalid status in context",
			err:      &AppError{Code: ErrCodeProviderAuth, Message: "Auth failed", Context: map[string]any{"http_status": 999}},
			expected: 0,
		},

		// Valid status code boundaries
		{
			name:     "status code 100",
			err:      &AppError{Code: ErrCodeProviderAuth, Message: "Continue", HTTPStatusCode: 100},
			expected: 100,
		},
		{
			name:     "status code 599",
			err:      &AppError{Code: ErrCodeProviderAuth, Message: "Network timeout", HTTPStatusCode: 599},
			expected: 599,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.HTTPStatus()
			if got != tt.expected {
				t.Errorf("HTTPStatus() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestAppError_WithHTTPStatus(t *testing.T) {
	err := NewAppError(ErrCodeProviderUnavailable, "Service down")
	result := err.WithHTTPStatus(503)

	// Should return the same error (for chaining)
	if result != err {
		t.Error("WithHTTPStatus() should return the same error for chaining")
	}

	if err.HTTPStatusCode != 503 {
		t.Errorf("HTTPStatusCode = %d, want 503", err.HTTPStatusCode)
	}

	// Test method chaining
	err2 := NewAppError(ErrCodeProviderAuth, "Auth failed").
		WithHTTPStatus(401).
		WithContext("provider", "doubleholo")

	if err2.HTTPStatusCode != 401 {
		t.Errorf("HTTPStatusCode = %d, want 401", err2.HTTPStatusCode)
	}
	if err2.Context["provider"] != "doubleholo" {
		t.Errorf("Context[provider] = %v, want PriceCharting", err2.Context["provider"])
	}
}

func TestAppError_WithHTTPStatus_Validation(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedStored int
		expectedReturn int
	}{
		// Invalid status codes - should be set to 0
		{
			name:           "invalid status too low (99)",
			statusCode:     99,
			expectedStored: 0,
			expectedReturn: 0,
		},
		{
			name:           "invalid status too high (600)",
			statusCode:     600,
			expectedStored: 0,
			expectedReturn: 0,
		},
		{
			name:           "invalid status way too high (700)",
			statusCode:     700,
			expectedStored: 0,
			expectedReturn: 0,
		},
		{
			name:           "invalid negative status",
			statusCode:     -1,
			expectedStored: 0,
			expectedReturn: 0,
		},
		{
			name:           "invalid zero status",
			statusCode:     0,
			expectedStored: 0,
			expectedReturn: 0,
		},
		{
			name:           "invalid large positive status",
			statusCode:     9999,
			expectedStored: 0,
			expectedReturn: 0,
		},

		// Valid status codes - should be stored as-is
		{
			name:           "valid status 100 (lower boundary)",
			statusCode:     100,
			expectedStored: 100,
			expectedReturn: 100,
		},
		{
			name:           "valid status 200",
			statusCode:     200,
			expectedStored: 200,
			expectedReturn: 200,
		},
		{
			name:           "valid status 404",
			statusCode:     404,
			expectedStored: 404,
			expectedReturn: 404,
		},
		{
			name:           "valid status 500",
			statusCode:     500,
			expectedStored: 500,
			expectedReturn: 500,
		},
		{
			name:           "valid status 599 (upper boundary)",
			statusCode:     599,
			expectedStored: 599,
			expectedReturn: 599,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewAppError(ErrCodeProviderUnavailable, "Service down")
			result := err.WithHTTPStatus(tt.statusCode)

			// Verify it returns the same error for chaining
			if result != err {
				t.Error("WithHTTPStatus() should return the same error for chaining")
			}

			// Verify the stored HTTPStatusCode
			if err.HTTPStatusCode != tt.expectedStored {
				t.Errorf("HTTPStatusCode field = %d, want %d", err.HTTPStatusCode, tt.expectedStored)
			}

			// Verify HTTPStatus() method returns the expected value
			httpStatus := err.HTTPStatus()
			if httpStatus != tt.expectedReturn {
				t.Errorf("HTTPStatus() = %d, want %d", httpStatus, tt.expectedReturn)
			}
		})
	}
}

func TestAppError_WithHTTPStatus_OverwritePreviousValue(t *testing.T) {
	// Test that setting an invalid status overwrites a previously valid status
	err := NewAppError(ErrCodeProviderUnavailable, "Service down")
	err.WithHTTPStatus(503) // Set valid status

	if err.HTTPStatusCode != 503 {
		t.Errorf("Initial HTTPStatusCode = %d, want 503", err.HTTPStatusCode)
	}

	// Now set an invalid status
	err.WithHTTPStatus(99)

	if err.HTTPStatusCode != 0 {
		t.Errorf("After invalid status, HTTPStatusCode = %d, want 0", err.HTTPStatusCode)
	}

	if err.HTTPStatus() != 0 {
		t.Errorf("After invalid status, HTTPStatus() = %d, want 0", err.HTTPStatus())
	}

	// Set valid status again
	err.WithHTTPStatus(502)

	if err.HTTPStatusCode != 502 {
		t.Errorf("After re-setting valid status, HTTPStatusCode = %d, want 502", err.HTTPStatusCode)
	}
}

func TestIsValidHTTPStatus(t *testing.T) {
	tests := []struct {
		status int
		valid  bool
	}{
		{99, false},
		{100, true},
		{200, true},
		{404, true},
		{500, true},
		{599, true},
		{600, false},
		{0, false},
		{-1, false},
		{1000, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.status), func(t *testing.T) {
			got := isValidHTTPStatus(tt.status)
			if got != tt.valid {
				t.Errorf("isValidHTTPStatus(%d) = %v, want %v", tt.status, got, tt.valid)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("underlying error")
	err := &AppError{Code: ErrCodeProviderUnavailable, Message: "Provider down", Cause: cause}

	if !errors.Is(err, cause) {
		t.Error("errors.Is() failed to find cause")
	}

	unwrapped := errors.Unwrap(err)
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestAppError_WithContext(t *testing.T) {
	err := NewAppError(ErrCodeCacheWriteFailed, "Read failed")
	_ = err.WithContext("path", "/tmp/cache.json")
	_ = err.WithContext("size", 1024)

	if err.Context["path"] != "/tmp/cache.json" {
		t.Errorf("Context[path] = %v, want /tmp/cache.json", err.Context["path"])
	}

	if err.Context["size"] != 1024 {
		t.Errorf("Context[size] = %v, want 1024", err.Context["size"])
	}
}

func TestNewAppError(t *testing.T) {
	err := NewAppError(ErrCodeConfigInvalid, "Invalid config")

	if err.Code != ErrCodeConfigInvalid {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeConfigInvalid)
	}

	if err.Message != "Invalid config" {
		t.Errorf("Message = %s, want %s", err.Message, "Invalid config")
	}
}

func TestAppError_WithCause(t *testing.T) {
	cause := fmt.Errorf("original error")
	err := &AppError{Code: ErrCodeNetworkTimeout, Message: "Request timeout", Cause: cause}

	if err.Code != ErrCodeNetworkTimeout {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeNetworkTimeout)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}

	if !errors.Is(err, cause) {
		t.Error("AppError should preserve cause for errors.Is()")
	}
}

func TestProviderAuthFailed(t *testing.T) {
	cause := fmt.Errorf("401 Unauthorized")
	err := ProviderAuthFailed("doubleholo", cause)

	if err.Code != ErrCodeProviderAuth {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeProviderAuth)
	}

	if !strings.Contains(err.Message, "doubleholo") {
		t.Errorf("Message should contain provider name, got: %s", err.Message)
	}
}

func TestProviderRateLimited(t *testing.T) {
	resetTime := "2024-01-01T12:00:00Z"
	err := ProviderRateLimited("PokemonTCGIO", resetTime)

	if err.Code != ErrCodeProviderRateLimit {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeProviderRateLimit)
	}

	if err.Context["reset_time"] != resetTime {
		t.Errorf("Context[reset_time] = %v, want %s", err.Context["reset_time"], resetTime)
	}
}

func TestProviderUnavailable(t *testing.T) {
	cause := fmt.Errorf("network error")
	err := ProviderUnavailable("eBay", cause)

	if err.Code != ErrCodeProviderUnavailable {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeProviderUnavailable)
	}

	if !strings.Contains(err.Message, "eBay") {
		t.Errorf("Message should contain provider name, got: %s", err.Message)
	}

	if !errors.Is(err, cause) {
		t.Error("Should preserve cause")
	}
}

func TestConfigInvalid(t *testing.T) {
	err := ConfigInvalid("top", -5, "must be positive")

	if err.Code != ErrCodeConfigInvalid {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeConfigInvalid)
	}

	if err.Context["field"] != "top" {
		t.Errorf("Context[field] = %v, want 'top'", err.Context["field"])
	}

	if err.Context["value"] != -5 {
		t.Errorf("Context[value] = %v, want -5", err.Context["value"])
	}

	if !strings.Contains(err.Message, "top") {
		t.Errorf("Message should contain field name, got: %s", err.Message)
	}
}

func TestConfigMissing(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		envVar string
	}{
		{
			name:   "with env var",
			field:  "api-key",
			envVar: "API_KEY",
		},
		{
			name:   "without env var",
			field:  "set",
			envVar: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConfigMissing(tt.field, tt.envVar)

			if err.Code != ErrCodeConfigMissing {
				t.Errorf("Code = %s, want %s", err.Code, ErrCodeConfigMissing)
			}

			if !strings.Contains(err.Message, tt.field) {
				t.Errorf("Message should contain field name, got: %s", err.Message)
			}
		})
	}
}

func TestCacheCorrupted(t *testing.T) {
	path := "/tmp/cache.json"
	cause := fmt.Errorf("invalid JSON")
	err := CacheCorrupted(path, cause)

	if err.Code != ErrCodeCacheCorrupted {
		t.Errorf("Code = %s, want %s", err.Code, ErrCodeCacheCorrupted)
	}

	if !strings.Contains(err.Message, path) {
		t.Errorf("Message should contain path, got: %s", err.Message)
	}

	if err.Context["path"] != path {
		t.Errorf("Context[path] = %v, want %s", err.Context["path"], path)
	}
}

func TestErrorCodeConstants(t *testing.T) {
	// Verify all error codes follow the expected format
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrCodeProviderUnavailable, "ERR_PROV_UNAVAILABLE"},
		{ErrCodeProviderAuth, "ERR_PROV_AUTH"},
		{ErrCodeProviderRateLimit, "ERR_PROV_RATE_LIMIT"},
		{ErrCodeConfigInvalid, "ERR_CFG_INVALID"},
		{ErrCodeConfigMissing, "ERR_CFG_MISSING"},
		{ErrCodeCacheCorrupted, "ERR_CACHE_CORRUPTED"},
		{ErrCodeNetworkTimeout, "ERR_NET_TIMEOUT"},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if string(tt.code) != tt.expected {
				t.Errorf("Code value = %s, want %s", tt.code, tt.expected)
			}
		})
	}
}

// TestAppError_UTF8Truncation verifies that error message truncation
// does not split multi-byte UTF-8 characters, ensuring valid UTF-8 output
func TestAppError_UTF8Truncation(t *testing.T) {
	tests := []struct {
		name        string
		causeMsg    string
		description string
	}{
		{
			name:        "Chinese characters",
			causeMsg:    "Error: " + strings.Repeat("你好世界", 50), // 4 chars × 3 bytes each = 12 bytes per repeat
			description: "Chinese characters (3 bytes each)",
		},
		{
			name:        "Japanese characters",
			causeMsg:    "エラー: " + strings.Repeat("こんにちは", 30), // Japanese hiragana (3 bytes each)
			description: "Japanese hiragana (3 bytes each)",
		},
		{
			name:        "Emoji characters",
			causeMsg:    "Error: " + strings.Repeat("🔥💯🎉", 30), // Emoji (4 bytes each)
			description: "Emoji characters (4 bytes each)",
		},
		{
			name:        "Mixed ASCII and multi-byte",
			causeMsg:    strings.Repeat("Error 错误 エラー ", 20), // Mix of 1, 3, and 3 byte chars
			description: "Mixed ASCII and multi-byte UTF-8",
		},
		{
			name:        "Exactly at boundary",
			causeMsg:    strings.Repeat("a", 147) + "你好", // 147 + 6 = 153 bytes (just over limit)
			description: "String that ends with multi-byte char just past limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &AppError{
				Code:    ErrCodeProviderInvalidResp,
				Message: "Test error",
				Cause:   fmt.Errorf("%s", tt.causeMsg),
			}

			result := err.Error()

			// 1. Verify the result is valid UTF-8
			if !utf8.ValidString(result) {
				t.Errorf("Result contains invalid UTF-8: %q", result)
			}

			// 2. If the original cause was long enough to be truncated, verify truncation occurred
			if len(tt.causeMsg) > maxCauseMessageLength {
				if !strings.Contains(result, "...") {
					t.Errorf("Expected truncation marker '...' in result: %q", result)
				}
			}

			t.Logf("%s: Original=%d bytes, Result=%d bytes, Valid UTF-8=%v",
				tt.description, len(tt.causeMsg), len(result), utf8.ValidString(result))
		})
	}
}

// TestAppError_UTF8TruncationEdgeCases tests edge cases for UTF-8 truncation
func TestAppError_UTF8TruncationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		causeMsg string
	}{
		{
			name:     "empty cause",
			causeMsg: "",
		},
		{
			name:     "short ASCII cause",
			causeMsg: "short error",
		},
		{
			name:     "exactly at limit with ASCII",
			causeMsg: strings.Repeat("x", maxCauseMessageLength),
		},
		{
			name: "one byte over limit with multi-byte at boundary",
			// Create a string that's exactly maxCauseMessageLength with a multi-byte char at the end
			// that would be split if we naively truncate
			causeMsg: strings.Repeat("a", maxCauseMessageLength-2) + "你", // Last char would be split
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &AppError{
				Code:    ErrCodeProviderInvalidResp,
				Message: "Test",
				Cause:   fmt.Errorf("%s", tt.causeMsg),
			}

			result := err.Error()

			// Verify valid UTF-8
			if !utf8.ValidString(result) {
				t.Errorf("Result contains invalid UTF-8: %q", result)
			}

			// If original was longer than limit, should have "..."
			if len(tt.causeMsg) > maxCauseMessageLength {
				if !strings.Contains(result, "...") {
					t.Errorf("Expected '...' in truncated result: %q", result)
				}
			}
		})
	}
}

// TestHasErrorCode verifies that HasErrorCode walks the entire error chain
// and finds matching error codes at any depth.
func TestHasErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     ErrorCode
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			code:     ErrCodeProviderRateLimit,
			expected: false,
		},
		{
			name: "direct AppError match",
			err: &AppError{
				Code:    ErrCodeProviderRateLimit,
				Message: "rate limited",
			},
			code:     ErrCodeProviderRateLimit,
			expected: true,
		},
		{
			name: "direct AppError no match",
			err: &AppError{
				Code:    ErrCodeProviderAuth,
				Message: "auth failed",
			},
			code:     ErrCodeProviderRateLimit,
			expected: false,
		},
		{
			name: "wrapped AppError match at depth 1",
			err: fmt.Errorf("wrapper: %w", &AppError{
				Code:    ErrCodeProviderCircuitOpen,
				Message: "circuit open",
			}),
			code:     ErrCodeProviderCircuitOpen,
			expected: true,
		},
		{
			name: "wrapped AppError match at depth 2",
			err: fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", &AppError{
				Code:    ErrCodeProviderRateLimit,
				Message: "rate limited",
			})),
			code:     ErrCodeProviderRateLimit,
			expected: true,
		},
		{
			name: "wrapped AppError no match at any depth",
			err: fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", &AppError{
				Code:    ErrCodeProviderAuth,
				Message: "auth failed",
			})),
			code:     ErrCodeProviderRateLimit,
			expected: false,
		},
		{
			name:     "plain error no AppError",
			err:      errors.New("plain error"),
			code:     ErrCodeProviderRateLimit,
			expected: false,
		},
		{
			name: "multiple AppErrors in chain - first matches",
			err: &AppError{
				Code:    ErrCodeProviderCircuitOpen,
				Message: "outer error",
				Cause: &AppError{
					Code:    ErrCodeProviderRateLimit,
					Message: "inner error",
				},
			},
			code:     ErrCodeProviderCircuitOpen,
			expected: true,
		},
		{
			name: "multiple AppErrors in chain - second matches",
			err: &AppError{
				Code:    ErrCodeProviderAuth,
				Message: "outer error",
				Cause: &AppError{
					Code:    ErrCodeProviderRateLimit,
					Message: "inner error",
				},
			},
			code:     ErrCodeProviderRateLimit,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasErrorCode(tt.err, tt.code)
			if got != tt.expected {
				t.Errorf("HasErrorCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}
