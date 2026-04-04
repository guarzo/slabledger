package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

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

// handleHTTPError converts HTTP status codes to appropriate errors
func (c *Client) handleHTTPError(statusCode int, headers http.Header, body []byte) error {
	sanitized := sanitizeResponseBody(body, 200)

	switch statusCode {
	case 400:
		return apperrors.ProviderInvalidRequest(c.providerName, fmt.Errorf("HTTP 400: %s", sanitized))
	case 401, 403:
		return apperrors.ProviderAuthFailed(c.providerName, fmt.Errorf("HTTP %d: %s", statusCode, sanitized))
	case 404:
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

// sanitizeResponseBody cleans up response bodies for error messages.
// It detects HTML and provides concise summaries suitable for logs.
func sanitizeResponseBody(body []byte, maxLength int) string {
	if len(body) == 0 {
		return "empty response"
	}

	bodyStr := strings.TrimSpace(string(body))

	isHTML := strings.HasPrefix(bodyStr, "<!DOCTYPE") ||
		strings.HasPrefix(bodyStr, "<html") ||
		strings.Contains(bodyStr[:min(100, len(bodyStr))], "<html")

	if isHTML {
		summary := extractHTMLSummary(bodyStr)
		if summary != "" {
			return summary
		}
		return "HTML error page (see debug logs for details)"
	}

	if len(bodyStr) > maxLength {
		return bodyStr[:maxLength] + "... (truncated)"
	}

	return bodyStr
}

// extractHTMLSummary extracts meaningful info from HTML error pages
func extractHTMLSummary(html string) string {
	if idx := strings.Index(html, "<title>"); idx != -1 {
		end := strings.Index(html[idx:], "</title>")
		if end != -1 {
			title := strings.TrimSpace(html[idx+7 : idx+end])
			title = strings.ReplaceAll(title, "&amp;", "&")
			title = strings.ReplaceAll(title, "&lt;", "<")
			title = strings.ReplaceAll(title, "&gt;", ">")
			return title
		}
	}

	if idx := strings.Index(html, "<h1"); idx != -1 {
		start := strings.Index(html[idx:], ">")
		if start != -1 {
			start = idx + start + 1
			end := strings.Index(html[start:], "</h1>")
			if end != -1 {
				return strings.TrimSpace(html[start : start+end])
			}
		}
	}

	return ""
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Handles http2 timeout and other cases not covered by net.Error
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded")
}

// GetCircuitBreakerStats returns statistics about the circuit breaker
func (c *Client) GetCircuitBreakerStats() resilience.CircuitBreakerStats {
	return resilience.GetCircuitBreakerStats(c.breaker)
}
