package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/resilience"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/sony/gobreaker"
)

var httpxLogger = telemetry.NewSlogLogger(slog.LevelWarn, "text")

// Observer provides hooks for monitoring HTTP client operations
type Observer interface {
	OnAttempt(ctx context.Context, req *http.Request, attempt int)
	OnSuccess(ctx context.Context, req *http.Request, statusCode int, attempt int, duration time.Duration)
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
	UserAgent            string
	DefaultTimeout       time.Duration
	RetryPolicy          resilience.RetryPolicy
	CircuitBreakerConfig resilience.CircuitBreakerConfig
	Observer             Observer
	Transport            *http.Transport
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
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,

		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0,
		ExpectContinueTimeout: 1 * time.Second,

		DisableCompression: false,
		ForceAttemptHTTP2:  true,
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
	httpClient := &http.Client{
		Transport: config.Transport,
		Timeout:   config.DefaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
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
		logger:       httpxLogger,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}

	c.breaker = resilience.NewCircuitBreaker(config.CircuitBreakerConfig, c.logger)

	return c
}

// Do executes an HTTP request with retry and circuit breaker
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()
	var resp *Response
	attempt := 0

	err := resilience.RetryWithBackoff(ctx, c.logger, c.retryPolicy, func() error {
		attempt++

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
		httpReq, _ := http.NewRequestWithContext(ctx, req.Method, req.URL, nil) //nolint:errcheck // Best-effort request creation for error logging
		c.observer.OnError(ctx, httpReq, err, attempt, duration)
		return resp, err
	}

	return resp, nil
}

// doRequest performs a single HTTP request attempt
func (c *Client) doRequest(ctx context.Context, req Request, attempt int) (*Response, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("User-Agent", c.userAgent)
	httpReq.Header.Set("Accept", "application/json")
	// Note: Don't set Accept-Encoding manually — Go's HTTP client handles compression automatically.
	// When DisableCompression=false, Go requests and decompresses gzip/deflate transparently.

	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	c.observer.OnAttempt(ctx, httpReq, attempt)

	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		c.observer.OnError(ctx, httpReq, err, attempt, duration)
		if isTimeoutError(err) || ctx.Err() == context.DeadlineExceeded {
			return nil, apperrors.ProviderTimeout(c.providerName, err)
		}
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }() //nolint:errcheck // Intentional: cannot handle Close() errors in defer

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

	c.observer.OnSuccess(ctx, httpReq, httpResp.StatusCode, attempt, duration)
	return resp, nil
}
