package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/resilience"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/sony/gobreaker"
)

// Package-level logger for httpx client
var httpxLogger = telemetry.NewSlogLogger(slog.LevelWarn, "text")

// Observer provides hooks for monitoring HTTP client operations
type Observer interface {
	// OnAttempt is called before each HTTP request attempt
	OnAttempt(ctx context.Context, req *http.Request, attempt int)
	// OnSuccess is called when a request succeeds
	OnSuccess(ctx context.Context, req *http.Request, statusCode int, attempt int, duration time.Duration)
	// OnError is called when a request fails
	OnError(ctx context.Context, req *http.Request, err error, attempt int, duration time.Duration)
}

// NoopObserver is a no-op implementation of Observer
type NoopObserver struct{}

func (NoopObserver) OnAttempt(ctx context.Context, req *http.Request, attempt int) {}
func (NoopObserver) OnSuccess(ctx context.Context, req *http.Request, statusCode int, attempt int, duration time.Duration) {
}
func (NoopObserver) OnError(ctx context.Context, req *http.Request, err error, attempt int, duration time.Duration) {
}

// Config configures the HTTP client
type Config struct {
	// UserAgent sets the User-Agent header for all requests
	UserAgent string
	// DefaultTimeout is the default timeout for requests if not specified per-request
	DefaultTimeout time.Duration
	// RetryPolicy configures retry behavior
	RetryPolicy resilience.RetryPolicy
	// CircuitBreaker configures circuit breaker behavior
	CircuitBreakerConfig resilience.CircuitBreakerConfig
	// Observer provides hooks for metrics and monitoring
	Observer Observer
	// Transport allows customization of the underlying HTTP transport
	Transport *http.Transport
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig(name string) Config {
	return Config{
		UserAgent:            "slabledger/1.0",
		DefaultTimeout:       30 * time.Second,
		RetryPolicy:          resilience.DefaultRetryPolicy(),
		CircuitBreakerConfig: resilience.DefaultCircuitBreakerConfig(name),
		Observer:             NoopObserver{},
		Transport:            DefaultTransport(),
	}
}

// DefaultTransport returns an optimized HTTP transport
func DefaultTransport() *http.Transport {
	return &http.Transport{
		// Connection pooling
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,

		// Timeouts
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0, // Let client/request timeout control deadlines
		ExpectContinueTimeout: 1 * time.Second,

		// Compression
		DisableCompression: false,

		// HTTP/2
		ForceAttemptHTTP2: true,
	}
}

// Option configures a Client after construction.
type Option func(*Client)

// WithLogger sets the logger used for retry and circuit breaker logging.
// A nil logger is ignored (the default logger is kept).
func WithLogger(l observability.Logger) Option {
	if l == nil {
		return func(*Client) {}
	}
	return func(c *Client) { c.logger = l }
}

// Client is a unified HTTP client with retry, circuit breaker, and metrics
type Client struct {
	httpClient   *http.Client
	breaker      *gobreaker.CircuitBreaker
	retryPolicy  resilience.RetryPolicy
	observer     Observer
	userAgent    string
	providerName string
	logger       observability.Logger
}

// NewClient creates a new HTTP client with the given configuration
func NewClient(config Config, opts ...Option) *Client {
	// Create HTTP client with transport
	httpClient := &http.Client{
		Transport: config.Transport,
		Timeout:   config.DefaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	c := &Client{
		httpClient:   httpClient,
		retryPolicy:  config.RetryPolicy,
		observer:     config.Observer,
		userAgent:    config.UserAgent,
		providerName: config.CircuitBreakerConfig.Name,
		logger:       httpxLogger, // default
	}

	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}

	// Create circuit breaker with the (possibly overridden) logger
	c.breaker = resilience.NewCircuitBreaker(config.CircuitBreakerConfig, c.logger)

	return c
}

// Request represents an HTTP request
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
	Timeout time.Duration // Override default timeout for this request
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Do executes an HTTP request with retry and circuit breaker
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()
	var resp *Response
	attempt := 0

	// Execute with retry logic
	err := resilience.RetryWithBackoff(ctx, c.logger, c.retryPolicy, func() error {
		attempt++

		// Execute with circuit breaker
		result, err := c.breaker.Execute(func() (interface{}, error) {
			return c.doRequest(ctx, req, attempt)
		})

		if err != nil {
			// Extract response even on error so callers can inspect status
			// codes (e.g. 429) and headers (e.g. Retry-After).
			if result != nil {
				if r, ok := result.(*Response); ok {
					resp = r
				}
			}
			// Check for circuit breaker errors
			if errors.Is(err, gobreaker.ErrOpenState) {
				return apperrors.ProviderCircuitOpen(c.providerName)
			}
			if errors.Is(err, gobreaker.ErrTooManyRequests) {
				return apperrors.ProviderCircuitOpen(c.providerName)
			}
			return err
		}

		var ok bool
		resp, ok = result.(*Response)
		if !ok {
			return fmt.Errorf("unexpected response type from circuit breaker")
		}
		return nil
	})

	duration := time.Since(start)

	if err != nil {
		// Create a minimal request for observer
		httpReq, _ := http.NewRequestWithContext(ctx, req.Method, req.URL, nil) //nolint:errcheck // Best-effort request creation for error logging
		c.observer.OnError(ctx, httpReq, err, attempt, duration)
		return resp, err
	}

	return resp, nil
}

// doRequest performs a single HTTP request attempt
func (c *Client) doRequest(ctx context.Context, req Request, attempt int) (*Response, error) {
	// Apply request timeout if specified
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Create HTTP request
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set standard headers
	httpReq.Header.Set("User-Agent", c.userAgent)
	httpReq.Header.Set("Accept", "application/json")
	// Note: Don't set Accept-Encoding manually - let Go's HTTP client handle compression automatically
	// When DisableCompression=false, Go will request and decompress gzip/deflate transparently

	// Set custom headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Notify observer
	c.observer.OnAttempt(ctx, httpReq, attempt)

	// Execute request
	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		c.observer.OnError(ctx, httpReq, err, attempt, duration)
		// Detect timeout errors and convert them to proper AppError
		if isTimeoutError(err) || ctx.Err() == context.DeadlineExceeded {
			return nil, apperrors.ProviderTimeout(c.providerName, err)
		}
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }() //nolint:errcheck // Intentional: cannot handle Close() errors in defer

	// Read response body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.observer.OnError(ctx, httpReq, err, attempt, duration)
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	resp := &Response{
		StatusCode: httpResp.StatusCode,
		Headers:    httpResp.Header,
		Body:       body,
	}

	// Handle HTTP errors — return both the response and error so callers
	// can inspect status codes (e.g. 429) and headers (e.g. Retry-After).
	if httpResp.StatusCode >= 400 {
		err := c.handleHTTPError(httpResp.StatusCode, httpResp.Header, body)
		c.observer.OnError(ctx, httpReq, err, attempt, duration)
		return resp, err
	}

	// Success
	c.observer.OnSuccess(ctx, httpReq, httpResp.StatusCode, attempt, duration)
	return resp, nil
}

// sanitizeResponseBody cleans up response bodies for error messages
// It detects HTML and provides concise summaries suitable for logs
func sanitizeResponseBody(body []byte, maxLength int) string {
	if len(body) == 0 {
		return "empty response"
	}

	bodyStr := strings.TrimSpace(string(body))

	// Detect HTML responses
	isHTML := strings.HasPrefix(bodyStr, "<!DOCTYPE") ||
		strings.HasPrefix(bodyStr, "<html") ||
		strings.Contains(bodyStr[:min(100, len(bodyStr))], "<html")

	if isHTML {
		// Extract useful info from HTML
		summary := extractHTMLSummary(bodyStr)
		if summary != "" {
			return summary
		}
		return "HTML error page (see debug logs for details)"
	}

	// For non-HTML, truncate if too long
	if len(bodyStr) > maxLength {
		return bodyStr[:maxLength] + "... (truncated)"
	}

	return bodyStr
}

// extractHTMLSummary extracts meaningful info from HTML error pages
func extractHTMLSummary(html string) string {
	// Try to extract title
	if idx := strings.Index(html, "<title>"); idx != -1 {
		end := strings.Index(html[idx:], "</title>")
		if end != -1 {
			title := strings.TrimSpace(html[idx+7 : idx+end])
			// Clean up common patterns
			title = strings.ReplaceAll(title, "&amp;", "&")
			title = strings.ReplaceAll(title, "&lt;", "<")
			title = strings.ReplaceAll(title, "&gt;", ">")
			return title
		}
	}

	// Try to extract h1
	if idx := strings.Index(html, "<h1"); idx != -1 {
		// Find end of opening tag
		start := strings.Index(html[idx:], ">")
		if start != -1 {
			start = idx + start + 1
			end := strings.Index(html[start:], "</h1>")
			if end != -1 {
				h1 := strings.TrimSpace(html[start : start+end])
				return h1
			}
		}
	}

	return ""
}

// handleHTTPError converts HTTP status codes to appropriate errors
func (c *Client) handleHTTPError(statusCode int, headers http.Header, body []byte) error {
	// Sanitize body for error messages (max 200 chars for non-HTML)
	sanitized := sanitizeResponseBody(body, 200)

	switch statusCode {
	case 400:
		return apperrors.ProviderInvalidRequest(c.providerName, fmt.Errorf("HTTP 400: %s", sanitized))
	case 401, 403:
		return apperrors.ProviderAuthFailed(c.providerName, fmt.Errorf("HTTP %d: %s", statusCode, sanitized))
	case 404:
		// If body is empty or generic, provide a better message
		if sanitized == "empty response" || sanitized == "error code: 404" {
			sanitized = "endpoint or resource not found"
		}
		return apperrors.ProviderNotFound(c.providerName, sanitized)
	case 429:
		retryAfter := ""
		if headers != nil {
			retryAfter = headers.Get("Retry-After")
		}
		return apperrors.ProviderRateLimited(c.providerName, retryAfter)
	case 500, 502, 503, 504:
		return apperrors.ProviderUnavailable(c.providerName, fmt.Errorf("HTTP %d: %s", statusCode, sanitized))
	default:
		return fmt.Errorf("HTTP %d: %s", statusCode, sanitized)
	}
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, url string, headers map[string]string, timeout time.Duration) (*Response, error) {
	return c.Do(ctx, Request{
		Method:  http.MethodGet,
		URL:     url,
		Headers: headers,
		Timeout: timeout,
	})
}

// GetJSON performs a GET request and decodes the response as JSON
func (c *Client) GetJSON(ctx context.Context, url string, headers map[string]string, timeout time.Duration, dest interface{}) error {
	resp, err := c.Get(ctx, url, headers, timeout)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return fmt.Errorf("decoding JSON response: %w", err)
	}

	return nil
}

// Post performs a POST request
func (c *Client) Post(ctx context.Context, url string, headers map[string]string, body []byte, timeout time.Duration) (*Response, error) {
	if headers == nil {
		headers = make(map[string]string)
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}

	return c.Do(ctx, Request{
		Method:  http.MethodPost,
		URL:     url,
		Headers: headers,
		Body:    body,
		Timeout: timeout,
	})
}

// PostJSON performs a POST request with JSON body
func (c *Client) PostJSON(ctx context.Context, url string, headers map[string]string, body interface{}, timeout time.Duration, dest interface{}) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding JSON body: %w", err)
	}

	resp, err := c.Post(ctx, url, headers, bodyBytes, timeout)
	if err != nil {
		return err
	}

	if dest != nil {
		if err := json.Unmarshal(resp.Body, dest); err != nil {
			return fmt.Errorf("decoding JSON response: %w", err)
		}
	}

	return nil
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check for timeout in error message (handles http2 timeout and others)
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded")
}

// GetCircuitBreakerStats returns statistics about the circuit breaker
func (c *Client) GetCircuitBreakerStats() resilience.CircuitBreakerStats {
	return resilience.GetCircuitBreakerStats(c.breaker)
}
