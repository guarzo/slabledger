package httpx

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/platform/resilience"
)

// HTTPClient defines the interface for HTTP operations.
// This allows for dependency injection and testing with mock implementations.
//
// Production code uses httpx.Client which implements this interface.
// Test code can use MockHTTPClient which provides immediate responses
// without retry logic or circuit breakers.
type HTTPClient interface {
	// GetJSON performs a GET request and decodes the response as JSON.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - url: Full URL to request
	//   - headers: Optional HTTP headers (X-Api-Key, etc.)
	//   - timeout: Request timeout (0 for default)
	//   - dest: Pointer to struct for JSON decoding
	//
	// Returns error if request fails, times out, or JSON decoding fails.
	GetJSON(ctx context.Context, url string, headers map[string]string,
		timeout time.Duration, dest interface{}) error

	// Get performs a GET request and returns the raw response.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - url: Full URL to request
	//   - headers: Optional HTTP headers
	//   - timeout: Request timeout (0 for default)
	//
	// Returns Response with status code, headers, and body.
	Get(ctx context.Context, url string, headers map[string]string,
		timeout time.Duration) (*Response, error)

	// Post performs a POST request.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - url: Full URL to request
	//   - headers: Optional HTTP headers (will set Content-Type if not present)
	//   - body: Request body bytes
	//   - timeout: Request timeout (0 for default)
	//
	// Returns Response with status code, headers, and body.
	Post(ctx context.Context, url string, headers map[string]string,
		body []byte, timeout time.Duration) (*Response, error)

	// PostJSON performs a POST request with JSON body and decodes JSON response.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - url: Full URL to request
	//   - headers: Optional HTTP headers
	//   - body: Request body (will be JSON encoded)
	//   - timeout: Request timeout (0 for default)
	//   - dest: Pointer to struct for response JSON decoding (can be nil)
	//
	// Returns error if encoding/decoding fails or request fails.
	PostJSON(ctx context.Context, url string, headers map[string]string,
		body interface{}, timeout time.Duration, dest interface{}) error

	// Do performs a custom HTTP request.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - req: Request configuration (method, URL, headers, body, timeout)
	//
	// Returns Response with status code, headers, and body.
	Do(ctx context.Context, req Request) (*Response, error)

	// GetCircuitBreakerStats returns circuit breaker statistics.
	// Mock implementations may return zero values.
	GetCircuitBreakerStats() resilience.CircuitBreakerStats
}

// Compile-time verification that Client implements HTTPClient
var _ HTTPClient = (*Client)(nil)
