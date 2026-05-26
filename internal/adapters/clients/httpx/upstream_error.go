package httpx

import (
	"encoding/json"
	"fmt"
	"strings"
)

// UpstreamError represents a non-2xx HTTP response from an upstream provider.
// It is returned (wrapped, alongside the existing apperrors semantic wrapping)
// by handleHTTPError so callers can extract the raw status code and body when
// they want to surface them directly. Use errors.As to extract it:
//
//	var ue *httpx.UpstreamError
//	if errors.As(err, &ue) {
//	    // route on ue.StatusCode, surface ue.Message
//	}
//
// Network failures, timeouts, and circuit-breaker trips are NOT UpstreamErrors —
// they return only the existing apperrors. UpstreamError means "we reached
// the provider and the provider said no".
type UpstreamError struct {
	Provider   string // e.g. "dh"
	Op         string // logical operation, e.g. "POST /v1/enterprise/inventory/123/sync"
	StatusCode int    // upstream HTTP status (e.g. 422)
	Body       string // upstream response body (sanitized, length-capped)
	Message    string // best-effort extracted human message (e.g. JSON "error" field)
	RequestID  string // upstream x-request-id header if present
}

// Error implements error.
func (e *UpstreamError) Error() string {
	detail := e.Message
	if detail == "" {
		detail = e.Body
	}
	if detail != "" {
		return fmt.Sprintf("%s %s: status %d: %s", e.Provider, e.Op, e.StatusCode, detail)
	}
	return fmt.Sprintf("%s %s: status %d", e.Provider, e.Op, e.StatusCode)
}

// IsClientError reports whether the upstream returned a 4xx status.
func (e *UpstreamError) IsClientError() bool {
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// extractUpstreamMessage attempts to pull a human-readable error message out
// of an upstream response body. For JSON bodies with an "error" or "message"
// string field (the two common conventions), returns that value. Otherwise
// returns the body as-is (already sanitized by sanitizeResponseBody).
func extractUpstreamMessage(body []byte, contentType string) string {
	bodyStr := strings.TrimSpace(string(body))
	if bodyStr == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return sanitizeResponseBody([]byte(bodyStr), 200)
	}
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		return bodyStr
	}
	if v, ok := probe["error"].(string); ok && v != "" {
		return v
	}
	if v, ok := probe["message"].(string); ok && v != "" {
		return v
	}
	return bodyStr
}
