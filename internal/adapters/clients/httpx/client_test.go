package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/resilience"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObserver is a test implementation of Observer
type TestObserver struct {
	attempts  int32
	successes int32
	errors    int32
}

func (o *TestObserver) OnAttempt(ctx context.Context, req *http.Request, attempt int) {
	atomic.AddInt32(&o.attempts, 1)
}

func (o *TestObserver) OnSuccess(ctx context.Context, req *http.Request, statusCode int, attempt int, duration time.Duration) {
	atomic.AddInt32(&o.successes, 1)
}

func (o *TestObserver) OnError(ctx context.Context, req *http.Request, err error, attempt int, duration time.Duration) {
	atomic.AddInt32(&o.errors, 1)
}

func TestNewClient(t *testing.T) {
	config := DefaultConfig("test")
	client := NewClient(config)

	assert.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.breaker)
	assert.Equal(t, "slabledger/1.0", client.userAgent)
}

func TestNewClient_WithLogger(t *testing.T) {
	config := DefaultConfig("test")
	logger := observability.NewNoopLogger()
	client := NewClient(config, WithLogger(logger))

	assert.NotNil(t, client)
	assert.Equal(t, logger, client.logger)
}

func TestClient_Get_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "slabledger/1.0", r.Header.Get("User-Agent"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	client := NewClient(config)

	// Execute request
	resp, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(resp.Body), "success")
}

func TestClient_GetJSON_Success(t *testing.T) {
	type Response struct {
		Status string `json:"status"`
		Value  int    `json:"value"`
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response{Status: "ok", Value: 42})
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	client := NewClient(config)

	// Execute request
	var result Response
	err := client.GetJSON(context.Background(), server.URL, nil, 5*time.Second, &result)

	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Equal(t, 42, result.Value)
}

func TestClient_Post_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created": true}`))
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	client := NewClient(config)

	// Execute request
	body := []byte(`{"name": "test"}`)
	resp, err := client.Post(context.Background(), server.URL, nil, body, 5*time.Second)

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Contains(t, string(resp.Body), "created")
}

func TestClient_PostJSON_Success(t *testing.T) {
	type Request struct {
		Name string `json:"name"`
	}
	type Response struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Response{ID: 123, Name: req.Name})
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	client := NewClient(config)

	// Execute request
	req := Request{Name: "test"}
	var result Response
	err := client.PostJSON(context.Background(), server.URL, nil, req, 5*time.Second, &result)

	require.NoError(t, err)
	assert.Equal(t, 123, result.ID)
	assert.Equal(t, "test", result.Name)
}

func TestClient_Retry_Success(t *testing.T) {
	attempts := 0

	// Create test server that fails twice then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	// Create client with retry
	config := DefaultConfig("test")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	client := NewClient(config)

	// Execute request
	resp, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, attempts, "Should have retried twice before succeeding")
}

func TestClient_Retry_ExhaustedRetries(t *testing.T) {
	// Create test server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	// Create client with limited retries
	config := DefaultConfig("test")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	client := NewClient(config)

	// Execute request
	_, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation failed after")
}

func TestClient_CircuitBreaker_OpensOnFailures(t *testing.T) {
	// Create test server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	// Create client with aggressive circuit breaker
	config := DefaultConfig("test")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0, // No retries to speed up test
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	config.CircuitBreakerConfig = resilience.CircuitBreakerConfig{
		Name:         "test",
		MaxRequests:  1,
		Interval:     100 * time.Millisecond,
		Timeout:      200 * time.Millisecond,
		MinRequests:  3,
		FailureRatio: 0.5,
	}
	client := NewClient(config)

	// Make requests until circuit opens
	for i := 0; i < 10; i++ {
		_, err := client.Get(context.Background(), server.URL, nil, 1*time.Second)
		if err != nil {
			// Check if circuit breaker is open by checking stats
			stats := client.GetCircuitBreakerStats()
			if strings.ToLower(stats.State) == "open" {
				// Circuit opened!
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("Circuit breaker should have opened")
}

func TestClient_Observer_CallbacksInvoked(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create client with observer
	observer := &TestObserver{}
	config := DefaultConfig("test")
	config.Observer = observer
	client := NewClient(config)

	// Execute request
	_, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)

	require.NoError(t, err)
	assert.Greater(t, atomic.LoadInt32(&observer.attempts), int32(0))
	assert.Greater(t, atomic.LoadInt32(&observer.successes), int32(0))
	assert.Equal(t, int32(0), atomic.LoadInt32(&observer.errors))
}

func TestClient_Observer_ErrorCallbackInvoked(t *testing.T) {
	// Create test server that fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Create client with observer
	observer := &TestObserver{}
	config := DefaultConfig("test")
	config.Observer = observer
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0, // No retries
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	client := NewClient(config)

	// Execute request
	_, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)

	require.Error(t, err)
	assert.Greater(t, atomic.LoadInt32(&observer.attempts), int32(0))
	assert.Greater(t, atomic.LoadInt32(&observer.errors), int32(0))
}

func TestClient_CustomHeaders(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
		assert.Equal(t, "custom-value", r.Header.Get("X-Custom-Header"))

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	client := NewClient(config)

	// Execute request with custom headers
	headers := map[string]string{
		"Authorization":   "Bearer token123",
		"X-Custom-Header": "custom-value",
	}
	_, err := client.Get(context.Background(), server.URL, headers, 5*time.Second)

	require.NoError(t, err)
}

func TestClient_Timeout(t *testing.T) {
	// Create test server that delays
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0, // No retries
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	client := NewClient(config)

	// Execute request with short timeout
	_, err := client.Get(context.Background(), server.URL, nil, 50*time.Millisecond)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestClient_ContextCancellation(t *testing.T) {
	// Create test server that blocks until context is cancelled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	client := NewClient(config)

	// Create cancellable context, cancel after short delay
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Execute request
	_, err := client.Get(ctx, server.URL, nil, 5*time.Second)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

func TestClient_HTTPErrorCodes(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedErrMsg string
	}{
		{"Bad Request", http.StatusBadRequest, "invalid request"},
		{"Unauthorized", http.StatusUnauthorized, "authentication failed"},
		{"Forbidden", http.StatusForbidden, "authentication failed"},
		{"Not Found", http.StatusNotFound, "not found"},
		{"Rate Limited", http.StatusTooManyRequests, "rate limit"},
		{"Internal Server Error", http.StatusInternalServerError, "unavailable"},
		{"Bad Gateway", http.StatusBadGateway, "unavailable"},
		{"Service Unavailable", http.StatusServiceUnavailable, "unavailable"},
		{"Gateway Timeout", http.StatusGatewayTimeout, "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"error": "test error"}`))
			}))
			defer server.Close()

			// Create client
			config := DefaultConfig("test")
			config.RetryPolicy = resilience.RetryPolicy{
				MaxRetries:     0, // No retries
				InitialBackoff: 1 * time.Millisecond,
				MaxBackoff:     10 * time.Millisecond,
				BackoffFactor:  2.0,
			}
			client := NewClient(config)

			// Execute request
			_, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)

			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), tt.expectedErrMsg)
		})
	}
}

func TestDefaultTransport(t *testing.T) {
	transport := DefaultTransport()

	assert.NotNil(t, transport)
	assert.Equal(t, 100, transport.MaxIdleConns)
	assert.Equal(t, 10, transport.MaxIdleConnsPerHost)
	assert.Equal(t, 50, transport.MaxConnsPerHost)
	assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
	assert.False(t, transport.DisableCompression)
	assert.True(t, transport.ForceAttemptHTTP2)
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("test-provider")

	assert.Equal(t, "slabledger/1.0", config.UserAgent)
	assert.Equal(t, 30*time.Second, config.DefaultTimeout)
	assert.NotNil(t, config.Transport)
	assert.Equal(t, "test-provider", config.CircuitBreakerConfig.Name)
	assert.NotNil(t, config.Observer)
}

func TestClient_GetCircuitBreakerStats(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("test")
	client := NewClient(config)

	// Execute request
	_, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)
	require.NoError(t, err)

	// Get stats
	stats := client.GetCircuitBreakerStats()

	assert.Equal(t, "test", stats.Name)
	assert.NotEmpty(t, stats.State)
	assert.Greater(t, stats.Requests, uint32(0))
}

func TestNoopObserver(t *testing.T) {
	observer := NoopObserver{}

	// Should not panic
	observer.OnAttempt(context.Background(), nil, 1)
	observer.OnSuccess(context.Background(), nil, 200, 1, time.Second)
	observer.OnError(context.Background(), nil, fmt.Errorf("test"), 1, time.Second)
}

func TestSanitizeResponseBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		maxLen   int
		expected string
	}{
		{
			name:     "empty response",
			body:     []byte{},
			maxLen:   200,
			expected: "empty response",
		},
		{
			name:     "short plain text",
			body:     []byte("error message"),
			maxLen:   200,
			expected: "error message",
		},
		{
			name:     "long plain text truncated",
			body:     []byte(strings.Repeat("a", 300)),
			maxLen:   100,
			expected: strings.Repeat("a", 100) + "... (truncated)",
		},
		{
			name: "HTML with title extracted",
			body: []byte(`<!DOCTYPE html>
<html>
<head><title>pokemontcg.io | 504: Gateway time-out</title></head>
<body>
<h1>Gateway time-out</h1>
<p>Error code 504</p>
</body>
</html>`),
			maxLen:   200,
			expected: "pokemontcg.io | 504: Gateway time-out",
		},
		{
			name: "HTML with h1 extracted",
			body: []byte(`<!DOCTYPE html>
<html>
<head></head>
<body>
<h1 class="error">Service Unavailable</h1>
<p>Please try again later</p>
</body>
</html>`),
			maxLen:   200,
			expected: "Service Unavailable",
		},
		{
			name: "HTML without extractable content",
			body: []byte(`<!DOCTYPE html>
<html><body><div>Error</div></body></html>`),
			maxLen:   200,
			expected: "HTML error page (see debug logs for details)",
		},
		{
			name:     "HTML entities in title",
			body:     []byte(`<html><head><title>Error &amp; Issues &lt;404&gt;</title></head></html>`),
			maxLen:   200,
			expected: "Error & Issues <404>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeResponseBody(tt.body, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClient_HTML_Error_Sanitization(t *testing.T) {
	// Create test server that returns HTML 504 error
	htmlBody := `<!DOCTYPE html>
<html>
<head>
<title>pokemontcg.io | 504: Gateway time-out</title>
</head>
<body>
<div id="cf-wrapper">
    <div id="cf-error-details" class="p-0">
        <header class="mx-auto pt-10">
            <h1 class="inline-block">
                <span class="inline-block">Gateway time-out</span>
                <span class="code-label">Error code 504</span>
            </h1>
        </header>
    </div>
</div>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(htmlBody))
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("PokemonTCGIO")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0, // No retries for this test
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	client := NewClient(config)

	// Execute request
	_, err := client.Get(context.Background(), server.URL, nil, 5*time.Second)

	require.Error(t, err)
	errMsg := err.Error()

	// Verify error message is concise and doesn't contain full HTML
	assert.Contains(t, errMsg, "unavailable")
	assert.Contains(t, errMsg, "504")
	assert.Contains(t, errMsg, "pokemontcg.io | 504: Gateway time-out")
	assert.NotContains(t, errMsg, "<html>", "Should not contain HTML tags")
	assert.NotContains(t, errMsg, "<div>", "Should not contain HTML tags")
	assert.NotContains(t, errMsg, "cf-wrapper", "Should not contain HTML class names")
	assert.Less(t, len(errMsg), 500, "Error message should be concise for logs")
}

func TestExtractHTMLSummary(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "extract title",
			html:     `<html><head><title>Test Title</title></head><body></body></html>`,
			expected: "Test Title",
		},
		{
			name:     "extract h1 when no title",
			html:     `<html><head></head><body><h1>Main Heading</h1></body></html>`,
			expected: "Main Heading",
		},
		{
			name:     "h1 with attributes",
			html:     `<html><body><h1 class="error" id="main">Error Message</h1></body></html>`,
			expected: "Error Message",
		},
		{
			name:     "no extractable content",
			html:     `<html><body><div>Some content</div></body></html>`,
			expected: "",
		},
		{
			name:     "prefer title over h1",
			html:     `<html><head><title>Page Title</title></head><body><h1>Heading</h1></body></html>`,
			expected: "Page Title",
		},
		{
			name:     "handle HTML entities",
			html:     `<html><head><title>Test &amp; Demo &lt;v2&gt;</title></head></html>`,
			expected: "Test & Demo <v2>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHTMLSummary(tt.html)
			assert.Equal(t, tt.expected, result)
		})
	}
}
