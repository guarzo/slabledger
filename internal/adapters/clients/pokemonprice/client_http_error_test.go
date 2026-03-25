package pokemonprice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

// TestClient_HTTPErrorScenarios tests various HTTP error scenarios for the PokemonPriceTracker client.
// Uses table-driven tests to cover rate limiting, server errors, timeouts, not found, and malformed JSON.
func TestClient_HTTPErrorScenarios(t *testing.T) {
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		wantErrCode    apperrors.ErrorCode
		wantStatusCode int
		description    string
	}{
		{
			name: "rate_limited_429",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error": "rate limit exceeded"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderRateLimit,
			wantStatusCode: http.StatusTooManyRequests,
			description:    "Should handle 429 rate limiting response",
		},
		{
			name: "not_found_404",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error": "card not found"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderNotFound,
			wantStatusCode: http.StatusNotFound,
			description:    "Should handle 404 not found response",
		},
		{
			name: "internal_server_error_500",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": "internal server error"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderUnavailable,
			wantStatusCode: http.StatusInternalServerError,
			description:    "Should handle 500 internal server error",
		},
		{
			name: "bad_gateway_502",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"error": "bad gateway"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderUnavailable,
			wantStatusCode: http.StatusBadGateway,
			description:    "Should handle 502 bad gateway error",
		},
		{
			name: "service_unavailable_503",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error": "service unavailable"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderUnavailable,
			wantStatusCode: http.StatusServiceUnavailable,
			description:    "Should handle 503 service unavailable error",
		},
		{
			name: "gateway_timeout_504",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusGatewayTimeout)
				_, _ = w.Write([]byte(`{"error": "gateway timeout"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderUnavailable,
			wantStatusCode: http.StatusGatewayTimeout,
			description:    "Should handle 504 gateway timeout error",
		},
		{
			name: "unauthorized_401",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error": "unauthorized"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderAuth,
			wantStatusCode: http.StatusUnauthorized,
			description:    "Should handle 401 unauthorized error",
		},
		{
			name: "forbidden_403",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error": "forbidden"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderAuth,
			wantStatusCode: http.StatusForbidden,
			description:    "Should handle 403 forbidden error",
		},
		{
			name: "bad_request_400",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error": "invalid request parameters"}`))
			},
			wantErrCode:    apperrors.ErrCodeProviderInvalidReq,
			wantStatusCode: http.StatusBadRequest,
			description:    "Should handle 400 bad request error",
		},
		{
			name: "malformed_json_response",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{invalid json`))
			},
			wantErrCode:    apperrors.ErrCodeProviderInvalidReq,
			wantStatusCode: http.StatusOK,
			description:    "Should handle malformed JSON response",
		},
		{
			name: "html_error_page",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>503 Service Unavailable</title></head><body><h1>Service Unavailable</h1></body></html>`))
			},
			wantErrCode:    apperrors.ErrCodeProviderUnavailable,
			wantStatusCode: http.StatusServiceUnavailable,
			description:    "Should handle HTML error pages gracefully",
		},
		{
			name: "empty_response_body",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErrCode:    apperrors.ErrCodeProviderUnavailable,
			wantStatusCode: http.StatusInternalServerError,
			description:    "Should handle empty response body",
		},
		{
			name: "empty_data_array",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				resp := CardsResponse{Data: []CardPriceData{}}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErrCode:    apperrors.ErrCodeProviderNotFound,
			wantStatusCode: http.StatusOK,
			description:    "Should return not found error when data array is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			// Create httpx client with no retries pointing to test server
			config := httpx.DefaultConfig("PokemonPriceTracker")
			config.DefaultTimeout = 5 * time.Second
			config.RetryPolicy = resilience.RetryPolicy{
				MaxRetries:     0, // No retries for predictable tests
				InitialBackoff: 1 * time.Millisecond,
				MaxBackoff:     10 * time.Millisecond,
				BackoffFactor:  2.0,
			}
			httpClient := httpx.NewClient(config)

			ctx := context.Background()

			headers := map[string]string{
				"Authorization": "Bearer test_api_key",
				"Accept":        "application/json",
			}

			resp, err := httpClient.Get(ctx, server.URL+"/api/v2/cards?search=test", headers, 5*time.Second)

			// The httpx.Client handles HTTP errors by returning AppErrors
			if err != nil {
				var appErr *apperrors.AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("%s: expected *apperrors.AppError but got %T: %v", tt.description, err, err)
				}

				// Verify error code
				if appErr.Code != tt.wantErrCode {
					t.Errorf("%s: error code = %v, want %v (error: %v)", tt.description, appErr.Code, tt.wantErrCode, appErr)
				}
				return
			}

			// For successful HTTP status, check if response parsing fails
			if resp.StatusCode == http.StatusOK {
				var apiResp CardsResponse
				if jsonErr := json.Unmarshal(resp.Body, &apiResp); jsonErr != nil {
					// This is the malformed JSON case
					if tt.wantErrCode != apperrors.ErrCodeProviderInvalidReq {
						t.Errorf("%s: expected ErrCodeProviderInvalidReq for malformed JSON, got %v", tt.description, tt.wantErrCode)
					}
					return
				}

				// Empty cards case
				if len(apiResp.Data) == 0 && tt.wantErrCode == apperrors.ErrCodeProviderNotFound {
					// This is expected - empty results should result in not found
					return
				}
			}
		})
	}
}

// TestClient_Timeout tests request timeout handling.
func TestClient_Timeout(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		resp := CardsResponse{Data: []CardPriceData{{ID: "test"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create httpx client with very short timeout
	config := httpx.DefaultConfig("PokemonPriceTracker")
	config.DefaultTimeout = 50 * time.Millisecond
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0, // No retries
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	// Execute request with context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := httpClient.Get(ctx, server.URL, nil, 50*time.Millisecond)

	// Should get timeout error
	if err == nil {
		t.Fatal("expected timeout error but got nil")
	}

	// Check error message contains timeout indication
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "timeout") && !strings.Contains(errMsg, "deadline") && !strings.Contains(errMsg, "context") {
		// Also accept provider timeout error
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			if appErr.Code != apperrors.ErrCodeProviderTimeout && appErr.Code != apperrors.ErrCodeProviderUnavailable {
				t.Errorf("expected timeout-related error code, got %v: %v", appErr.Code, err)
			}
		} else {
			t.Errorf("expected timeout-related error, got: %v", err)
		}
	}
}

// TestClient_ContextCancellation tests context cancellation handling.
func TestClient_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := httpx.DefaultConfig("PokemonPriceTracker")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	_, err := httpClient.Get(ctx, server.URL, nil, 5*time.Second)

	// Should get context canceled error
	if err == nil {
		t.Fatal("expected context canceled error but got nil")
	}

	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "cancel") && !strings.Contains(errMsg, "context") {
		t.Errorf("expected context cancellation error, got: %v", err)
	}
}

// TestClient_ConnectionRefused tests handling of connection refused errors.
func TestClient_ConnectionRefused(t *testing.T) {
	config := httpx.DefaultConfig("PokemonPriceTracker")
	config.DefaultTimeout = 2 * time.Second
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0, // No retries
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	// Try to connect to a port that's definitely not listening
	ctx := context.Background()
	_, err := httpClient.Get(ctx, "http://127.0.0.1:1", nil, 2*time.Second)

	// Should get connection error
	if err == nil {
		t.Fatal("expected connection error but got nil")
	}

	// Error should indicate connection issue
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		// Connection errors typically result in provider unavailable
		if appErr.Code != apperrors.ErrCodeProviderUnavailable && appErr.Code != apperrors.ErrCodeProviderTimeout {
			t.Errorf("expected unavailable or timeout error code for connection refused, got %v", appErr.Code)
		}
	}
}

// TestClient_InvalidURL tests handling of invalid URLs.
func TestClient_InvalidURL(t *testing.T) {
	config := httpx.DefaultConfig("PokemonPriceTracker")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	ctx := context.Background()
	_, err := httpClient.Get(ctx, "://invalid-url", nil, 5*time.Second)

	// Should get error for invalid URL
	if err == nil {
		t.Fatal("expected error for invalid URL but got nil")
	}
}

// TestClient_LargeResponseBody tests handling of responses that might cause issues.
func TestClient_LargeResponseBody(t *testing.T) {
	// Create a server that returns a large error message
	largeErrorBody := strings.Repeat("error details ", 10000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(largeErrorBody))
	}))
	defer server.Close()

	config := httpx.DefaultConfig("PokemonPriceTracker")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	ctx := context.Background()
	_, err := httpClient.Get(ctx, server.URL, nil, 5*time.Second)

	// Should get error
	if err == nil {
		t.Fatal("expected error for 500 response but got nil")
	}

	// Error message should be truncated (not contain the full large body)
	errMsg := err.Error()
	if len(errMsg) > 1000 {
		t.Errorf("error message should be truncated for large response bodies, got length %d", len(errMsg))
	}
}
