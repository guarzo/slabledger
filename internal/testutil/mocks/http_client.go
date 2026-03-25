package mocks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

// MockHTTPClient is a test double for httpx.HTTPClient that provides
// immediate responses without retry logic, circuit breakers, or network I/O.
//
// Use this in tests to:
// - Eliminate test slowness from retry backoffs (1s, 2s, 4s, 8s...)
// - Avoid actual network calls
// - Control exact response data and error conditions
// - Make tests fast and deterministic
//
// Example:
//
//	mockClient := NewMockHTTPClient()
//
//	// Configure response for specific URL
//	mockClient.AddResponse("https://api.tcgdex.net/v2/en/sets",
//	  MockHTTPResponse{
//	    StatusCode: 200,
//	    Body: `[{"id": "base1", "name": "Base Set"}]`,
//	  })
type MockHTTPClient struct {
	mu sync.RWMutex

	// responses maps URL patterns to mock responses
	responses map[string]*MockHTTPResponse

	// defaultResponse is returned when no specific response is configured
	defaultResponse *MockHTTPResponse

	// behavior controls mock behavior (errors, delays, etc.)
	behavior *MockBehavior

	// callLog tracks all requests for verification
	callLog []MockHTTPCall

	// stats tracks call statistics
	stats MockHTTPStats
}

// MockHTTPResponse represents a mock HTTP response
type MockHTTPResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       string
	Error      error // If set, return this error instead of response
}

// MockHTTPCall represents a recorded HTTP call
type MockHTTPCall struct {
	Method    string
	URL       string
	Headers   map[string]string
	Body      []byte
	Timestamp time.Time
}

// MockHTTPStats tracks mock client statistics
type MockHTTPStats struct {
	GetCount      int
	PostCount     int
	GetJSONCount  int
	PostJSONCount int
	TotalCalls    int
	TotalErrors   int
}

// NewMockHTTPClient creates a new mock HTTP client with optional behaviors.
//
// Options:
//   - WithError(err): All requests return this error
//   - WithDelay(duration): Add artificial delay (for timeout testing)
//   - WithFailAfterN(n): First N calls succeed, rest fail
//   - WithRateLimit(): Simulate rate limiting (429 errors)
//
// Example:
//
//	// Basic mock
//	mock := NewMockHTTPClient()
//
//	// With error simulation
//	mock := NewMockHTTPClient(WithError(fmt.Errorf("network error")))
//
//	// With delay for timeout testing
//	mock := NewMockHTTPClient(WithDelay(5 * time.Second))
func NewMockHTTPClient(opts ...MockOption) *MockHTTPClient {
	behavior := &MockBehavior{}
	for _, opt := range opts {
		opt(behavior)
	}

	return &MockHTTPClient{
		responses: make(map[string]*MockHTTPResponse),
		defaultResponse: &MockHTTPResponse{
			StatusCode: 200,
			Body:       `{"data": []}`,
		},
		behavior: behavior,
		callLog:  make([]MockHTTPCall, 0),
	}
}

// AddResponse configures a mock response for a specific URL or URL pattern.
//
// URL patterns support:
//   - Exact match: "https://api.tcgdex.net/v2/en/sets"
//   - Prefix match: "https://api.tcgdex.net" (matches all URLs starting with this)
//   - Contains match: "/cards" (matches any URL containing "/cards")
//
// Example:
//
//	mock.AddResponse("https://api.tcgdex.net/v2/en/sets", MockHTTPResponse{
//	  StatusCode: 200,
//	  Body: `[{"id": "base1", "name": "Base Set"}]`,
//	})
//
//	// Pattern matching
//	mock.AddResponse("/cards?q=set.id:sv1", MockHTTPResponse{
//	  StatusCode: 200,
//	  Body: `{"data": [...], "totalCount": 198}`,
//	})
//
//	// Error response
//	mock.AddResponse("/sets/invalid", MockHTTPResponse{
//	  StatusCode: 404,
//	  Body: `{"error": "not found"}`,
//	})
func (m *MockHTTPClient) AddResponse(urlPattern string, response MockHTTPResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.responses[urlPattern] = &response
}

// SetDefaultResponse sets the default response for URLs without specific responses.
func (m *MockHTTPClient) SetDefaultResponse(response MockHTTPResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.defaultResponse = &response
}

// GetJSON implements httpx.HTTPClient interface
func (m *MockHTTPClient) GetJSON(ctx context.Context, url string, headers map[string]string,
	timeout time.Duration, dest interface{}) error {
	m.mu.Lock()
	m.stats.GetJSONCount++
	m.stats.TotalCalls++
	m.mu.Unlock()

	// Check for configured error/delay using checkBehavior
	if m.behavior != nil {
		if err := m.behavior.checkBehavior(); err != nil {
			m.mu.Lock()
			m.stats.TotalErrors++
			m.mu.Unlock()
			return err
		}
	}

	// Check context cancellation (after potential delay)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Record call
	m.recordCall("GET", url, headers, nil)

	// Find response
	response := m.findResponse(url)
	if response.Error != nil {
		m.mu.Lock()
		m.stats.TotalErrors++
		m.mu.Unlock()
		return response.Error
	}

	// Handle HTTP errors
	if response.StatusCode >= 400 {
		m.mu.Lock()
		m.stats.TotalErrors++
		m.mu.Unlock()
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, response.Body)
	}

	// Decode JSON
	if dest != nil {
		if err := json.Unmarshal([]byte(response.Body), dest); err != nil {
			return fmt.Errorf("decoding JSON: %w", err)
		}
	}

	return nil
}

// Get implements httpx.HTTPClient interface
func (m *MockHTTPClient) Get(ctx context.Context, url string, headers map[string]string,
	timeout time.Duration) (*httpx.Response, error) {
	m.mu.Lock()
	m.stats.GetCount++
	m.stats.TotalCalls++
	m.mu.Unlock()

	// Check for configured error/delay using checkBehavior
	if m.behavior != nil {
		if err := m.behavior.checkBehavior(); err != nil {
			m.mu.Lock()
			m.stats.TotalErrors++
			m.mu.Unlock()
			return nil, err
		}
	}

	// Check context cancellation (after potential delay)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.recordCall("GET", url, headers, nil)

	response := m.findResponse(url)
	if response.Error != nil {
		m.mu.Lock()
		m.stats.TotalErrors++
		m.mu.Unlock()
		return nil, response.Error
	}

	httpHeaders := make(http.Header)
	for k, v := range response.Headers {
		httpHeaders.Set(k, v)
	}

	return &httpx.Response{
		StatusCode: response.StatusCode,
		Headers:    httpHeaders,
		Body:       []byte(response.Body),
	}, nil
}

// Post implements httpx.HTTPClient interface
func (m *MockHTTPClient) Post(ctx context.Context, url string, headers map[string]string,
	body []byte, timeout time.Duration) (*httpx.Response, error) {
	m.mu.Lock()
	m.stats.PostCount++
	m.stats.TotalCalls++
	m.mu.Unlock()

	// Check for configured error/delay using checkBehavior
	if m.behavior != nil {
		if err := m.behavior.checkBehavior(); err != nil {
			m.mu.Lock()
			m.stats.TotalErrors++
			m.mu.Unlock()
			return nil, err
		}
	}

	// Check context cancellation (after potential delay)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.recordCall("POST", url, headers, body)

	response := m.findResponse(url)
	if response.Error != nil {
		m.mu.Lock()
		m.stats.TotalErrors++
		m.mu.Unlock()
		return nil, response.Error
	}

	httpHeaders := make(http.Header)
	for k, v := range response.Headers {
		httpHeaders.Set(k, v)
	}

	return &httpx.Response{
		StatusCode: response.StatusCode,
		Headers:    httpHeaders,
		Body:       []byte(response.Body),
	}, nil
}

// PostJSON implements httpx.HTTPClient interface
func (m *MockHTTPClient) PostJSON(ctx context.Context, url string, headers map[string]string,
	body interface{}, timeout time.Duration, dest interface{}) error {
	m.mu.Lock()
	m.stats.PostJSONCount++
	m.stats.TotalCalls++
	m.mu.Unlock()

	// Check for configured error/delay using checkBehavior
	if m.behavior != nil {
		if err := m.behavior.checkBehavior(); err != nil {
			m.mu.Lock()
			m.stats.TotalErrors++
			m.mu.Unlock()
			return err
		}
	}

	// Check context cancellation (after potential delay)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	bodyBytes, _ := json.Marshal(body) //nolint:errcheck // Test code - marshal error not critical
	m.recordCall("POST", url, headers, bodyBytes)

	response := m.findResponse(url)
	if response.Error != nil {
		m.mu.Lock()
		m.stats.TotalErrors++
		m.mu.Unlock()
		return response.Error
	}

	if response.StatusCode >= 400 {
		m.mu.Lock()
		m.stats.TotalErrors++
		m.mu.Unlock()
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, response.Body)
	}

	if dest != nil {
		if err := json.Unmarshal([]byte(response.Body), dest); err != nil {
			return fmt.Errorf("decoding JSON: %w", err)
		}
	}

	return nil
}

// Do implements httpx.HTTPClient interface
func (m *MockHTTPClient) Do(ctx context.Context, req httpx.Request) (*httpx.Response, error) {
	switch req.Method {
	case "GET", "":
		return m.Get(ctx, req.URL, req.Headers, req.Timeout)
	case "POST":
		return m.Post(ctx, req.URL, req.Headers, req.Body, req.Timeout)
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}
}

// GetCircuitBreakerStats implements httpx.HTTPClient interface
// Returns zero values since mock doesn't have a circuit breaker
func (m *MockHTTPClient) GetCircuitBreakerStats() resilience.CircuitBreakerStats {
	return resilience.CircuitBreakerStats{
		Name:  "Mock",
		State: "closed",
	}
}

// GetCallLog returns all recorded HTTP calls for verification
func (m *MockHTTPClient) GetCallLog() []MockHTTPCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return append([]MockHTTPCall{}, m.callLog...)
}

// GetStats returns call statistics
func (m *MockHTTPClient) GetStats() MockHTTPStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.stats
}

// Reset clears all call logs and statistics
func (m *MockHTTPClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callLog = make([]MockHTTPCall, 0)
	m.stats = MockHTTPStats{}
}

// findResponse finds the best matching response for a URL
func (m *MockHTTPClient) findResponse(url string) *MockHTTPResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for exact match
	if resp, ok := m.responses[url]; ok {
		return resp
	}

	// Check for pattern matches (contains), prioritizing longer/more specific patterns
	var bestMatch *MockHTTPResponse
	var bestMatchLen int

	for pattern, resp := range m.responses {
		if strings.Contains(url, pattern) {
			// Prefer longer patterns (more specific)
			if len(pattern) > bestMatchLen {
				bestMatch = resp
				bestMatchLen = len(pattern)
			}
		}
	}

	if bestMatch != nil {
		return bestMatch
	}

	// Return default response
	return m.defaultResponse
}

// recordCall records an HTTP call for verification
func (m *MockHTTPClient) recordCall(method, url string, headers map[string]string, body []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callLog = append(m.callLog, MockHTTPCall{
		Method:    method,
		URL:       url,
		Headers:   headers,
		Body:      body,
		Timestamp: time.Now(),
	})
}

// Compile-time verification that MockHTTPClient implements httpx.HTTPClient
var _ httpx.HTTPClient = (*MockHTTPClient)(nil)
