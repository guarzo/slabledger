# DH Error Surfacing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface upstream DH HTTP errors faithfully to the user. Upstream 4xx (e.g. DH 422 "No active channel configured for: shopify") must reach the response body with a matching status code, not a generic 502 "check server logs".

**Architecture:** Introduce `httpx.UpstreamError{StatusCode, Body, Message, RequestID, Op}` returned from the unified HTTP client for all non-2xx responses (in addition to the existing apperrors wrapping for known categories). Plumb per-cert failure reasons through `dhlisting.DHListingResult.FailedCerts`. Add handler helper `dhErrorStatus` that uses `errors.As` to pull status/body out of the upstream error and route appropriately (4xx passthrough except 401/403→502; 5xx→502).

**Tech Stack:** Go 1.26, hexagonal architecture, `internal/adapters/clients/httpx`, `internal/adapters/clients/dh`, `internal/domain/dhlisting`, `internal/adapters/httpserver/handlers`.

**Design discovery (affects spec):** The spec proposed `dh.UpstreamError` in the `dh` package, but `httpx.handleHTTPError` already consumes non-2xx responses at the httpx layer before the DH client sees them. The type belongs in `httpx` so it can be populated where the raw response is available. This is a strictly smaller, cleaner change than the alternative (unwinding httpx's auto-error). All other design decisions in the spec stand.

---

## File Structure

**New files:**
- `internal/adapters/clients/httpx/upstream_error.go` — the new error type + helpers
- `internal/adapters/clients/httpx/upstream_error_test.go` — unit tests
- `internal/adapters/httpserver/handlers/dh_errors.go` — handler helper `dhErrorStatus`
- `internal/adapters/httpserver/handlers/dh_errors_test.go` — unit tests for the helper

**Modified files:**
- `internal/adapters/clients/httpx/client_helpers.go` — `handleHTTPError` returns errors that *also* carry an `*UpstreamError` via `errors.As`
- `internal/adapters/clients/httpx/client.go` — capture `Op` (request method+path) in the error
- `internal/domain/dhlisting/types.go` — add `FailedCerts map[string]error` to `DHListingResult`
- `internal/domain/dhlisting/dh_listing_service.go` — populate `FailedCerts` on every skip/revert branch
- `internal/adapters/httpserver/handlers/campaigns_dh_listing.go` — use `result.FailedCerts` and `dhErrorStatus`
- `internal/adapters/httpserver/handlers/dh_reconcile_handler.go` — use `dhErrorStatus`
- `internal/adapters/httpserver/handlers/dh_unmatch_handler.go` — use `dhErrorStatus`
- `internal/adapters/httpserver/handlers/dh_fix_match_handler.go` — use `dhErrorStatus`
- `internal/adapters/httpserver/handlers/dh_retry_match_handler.go` — use `dhErrorStatus`
- `internal/adapters/httpserver/handlers/dh_select_match_handler.go` — use `dhErrorStatus`
- `internal/adapters/httpserver/handlers/campaigns_dh_listing_test.go` — update assertions, add new cases

**Out of scope:** non-DH handlers, non-DH clients, CL/MM/PSA-Exchange paths. Captured in `MEMORY.md` for follow-up.

---

## Task 1: Introduce `httpx.UpstreamError` type with extraction helper

**Files:**
- Create: `internal/adapters/clients/httpx/upstream_error.go`
- Test: `internal/adapters/clients/httpx/upstream_error_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/adapters/clients/httpx/upstream_error_test.go`:

```go
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestUpstreamError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  UpstreamError
		want string
	}{
		{
			name: "with message",
			err:  UpstreamError{Provider: "dh", Op: "POST /v1/foo", StatusCode: 422, Message: "No active channel"},
			want: `dh POST /v1/foo: status 422: No active channel`,
		},
		{
			name: "without message uses body",
			err:  UpstreamError{Provider: "dh", Op: "PATCH /v1/bar", StatusCode: 500, Body: "internal error"},
			want: `dh PATCH /v1/bar: status 500: internal error`,
		},
		{
			name: "no message or body",
			err:  UpstreamError{Provider: "dh", Op: "GET /v1/baz", StatusCode: 404},
			want: `dh GET /v1/baz: status 404`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpstreamError_IsClientError(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{200, false}, {399, false}, {400, true}, {422, true}, {499, true}, {500, false}, {503, false},
	}
	for _, tt := range tests {
		ue := UpstreamError{StatusCode: tt.status}
		if got := ue.IsClientError(); got != tt.want {
			t.Errorf("IsClientError(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestUpstreamError_ExtractMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		ct   string
		want string
	}{
		{
			name: "json error field",
			body: `{"error":"No active channel configured for: shopify"}`,
			ct:   "application/json",
			want: "No active channel configured for: shopify",
		},
		{
			name: "json message field",
			body: `{"message":"bad request"}`,
			ct:   "application/json",
			want: "bad request",
		},
		{
			name: "json without error or message",
			body: `{"foo":"bar"}`,
			ct:   "application/json",
			want: `{"foo":"bar"}`,
		},
		{
			name: "non-json plain",
			body: "internal error",
			ct:   "text/plain",
			want: "internal error",
		},
		{
			name: "empty body",
			body: "",
			ct:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUpstreamMessage([]byte(tt.body), tt.ct)
			if got != tt.want {
				t.Errorf("extractUpstreamMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpstreamError_ErrorsAs(t *testing.T) {
	original := &UpstreamError{Provider: "dh", Op: "POST /v1/sync", StatusCode: 422, Message: "x"}
	wrapped := fmt.Errorf("operation failed: %w", original)
	var got *UpstreamError
	if !errors.As(wrapped, &got) {
		t.Fatal("errors.As did not find UpstreamError through wrapping")
	}
	if got.StatusCode != 422 {
		t.Errorf("StatusCode = %d, want 422", got.StatusCode)
	}
}

// Sanity check: extractUpstreamMessage trims and ignores nulls.
func TestUpstreamError_ExtractMessage_IgnoresEmptyJSONField(t *testing.T) {
	if got := extractUpstreamMessage([]byte(`{"error":""}`), "application/json"); got != `{"error":""}` {
		t.Errorf("empty error field should fall through to raw body, got %q", got)
	}
}

// Ensure JSON detection works even when content-type has charset suffix.
func TestUpstreamError_ExtractMessage_JSONWithCharset(t *testing.T) {
	body := `{"error":"foo"}`
	if got := extractUpstreamMessage([]byte(body), "application/json; charset=utf-8"); got != "foo" {
		t.Errorf("got %q, want %q", got, "foo")
	}
}

var _ = json.Marshal // keep json import if unused above
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/httpx/ -run TestUpstreamError -v`
Expected: FAIL with "undefined: UpstreamError" / "undefined: extractUpstreamMessage"

- [ ] **Step 3: Write minimal implementation**

Create `internal/adapters/clients/httpx/upstream_error.go`:

```go
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
		return bodyStr
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/clients/httpx/ -run TestUpstreamError -v`
Expected: PASS (all sub-tests)

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/httpx/upstream_error.go internal/adapters/clients/httpx/upstream_error_test.go
git commit -m "httpx: introduce UpstreamError carrying raw status + body

Returned (wrapped, alongside existing apperrors) from handleHTTPError so
handlers can route on upstream status and surface the upstream message
verbatim instead of collapsing every 4xx into a generic 502.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 2: Wrap `handleHTTPError` results with `UpstreamError`

**Files:**
- Modify: `internal/adapters/clients/httpx/client_helpers.go:98-138`
- Modify: `internal/adapters/clients/httpx/client.go:246-250` (pass request method+url through)
- Test: `internal/adapters/clients/httpx/client_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/adapters/clients/httpx/client_test.go`:

```go
func TestClient_UpstreamErrorAttached(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		ct         string
		wantStatus int
		wantMsg    string
	}{
		{name: "422 json error", status: 422, body: `{"error":"No active channel"}`, ct: "application/json", wantStatus: 422, wantMsg: "No active channel"},
		{name: "500 plain", status: 500, body: "boom", ct: "text/plain", wantStatus: 500, wantMsg: "boom"},
		{name: "404 json message", status: 404, body: `{"message":"gone"}`, ct: "application/json", wantStatus: 404, wantMsg: "gone"},
		{name: "401 auth", status: 401, body: "no", ct: "text/plain", wantStatus: 401, wantMsg: "no"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.ct != "" {
					w.Header().Set("Content-Type", tt.ct)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := NewClient("testprov", nil, nil)
			_, err := c.Get(context.Background(), srv.URL+"/v1/thing", nil, 5*time.Second)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ue *UpstreamError
			if !errors.As(err, &ue) {
				t.Fatalf("expected UpstreamError in chain, got %v", err)
			}
			if ue.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", ue.StatusCode, tt.wantStatus)
			}
			if ue.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", ue.Message, tt.wantMsg)
			}
			if ue.Provider != "testprov" {
				t.Errorf("Provider = %q, want %q", ue.Provider, "testprov")
			}
			if !strings.Contains(ue.Op, "/v1/thing") {
				t.Errorf("Op = %q, want it to contain %q", ue.Op, "/v1/thing")
			}
		})
	}
}
```

Add imports if not already present at the top of `client_test.go`: `"errors"`, `"strings"`, `"time"`, `"context"`, `"net/http"`, `"net/http/httptest"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/httpx/ -run TestClient_UpstreamErrorAttached -v`
Expected: FAIL — `errors.As` does not find `UpstreamError` in the chain.

- [ ] **Step 3: Modify `handleHTTPError` to wrap each return with UpstreamError**

Edit `internal/adapters/clients/httpx/client_helpers.go`. Change signature to accept the request URL+method and build/wrap an UpstreamError around each return:

Replace the existing `handleHTTPError` function with:

```go
// handleHTTPError converts HTTP status codes to appropriate errors. Each
// returned error has an *UpstreamError in its error chain (via fmt.Errorf
// "%w"), so callers can errors.As to extract raw status + body alongside
// the existing semantic apperrors wrapping.
func (c *Client) handleHTTPError(ctx context.Context, method, url string, statusCode int, headers http.Header, body []byte) error {
	sanitized := sanitizeResponseBody(body, 200)
	contentType := ""
	if headers != nil {
		contentType = headers.Get("Content-Type")
	}
	requestID := ""
	if headers != nil {
		requestID = headers.Get("X-Request-Id")
	}
	ue := &UpstreamError{
		Provider:   c.providerName,
		Op:         fmt.Sprintf("%s %s", method, url),
		StatusCode: statusCode,
		Body:       sanitized,
		Message:    extractUpstreamMessage(body, contentType),
		RequestID:  requestID,
	}
	switch statusCode {
	case 400:
		return apperrors.ProviderInvalidRequest(c.providerName, fmt.Errorf("HTTP 400: %s: %w", sanitized, ue))
	case 401, 403:
		return apperrors.ProviderAuthFailed(c.providerName, fmt.Errorf("HTTP %d: %s: %w", statusCode, sanitized, ue))
	case 404:
		if sanitized == "empty response" || sanitized == "error code: 404" {
			sanitized = "endpoint or resource not found"
		}
		// ProviderNotFound takes a string, so attach UpstreamError via wrapping
		// at the call site by returning a fmt.Errorf chain.
		return fmt.Errorf("%w: %w", apperrors.ProviderNotFound(c.providerName, sanitized), ue)
	case 429:
		retryAfter := ""
		if headers != nil {
			retryAfter = headers.Get("Retry-After")
		}
		if c.logger != nil {
			fields := []observability.Field{
				observability.String("provider", c.providerName),
				observability.String("body", sanitized),
			}
			if headers != nil {
				for _, h := range []string{"Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"} {
					if v := headers.Get(h); v != "" {
						fields = append(fields, observability.String(h, v))
					}
				}
			}
			c.logger.Info(ctx, "HTTP 429 rate limit response", fields...)
		}
		return fmt.Errorf("%w: %w", apperrors.ProviderRateLimited(c.providerName, retryAfter), ue)
	case 500, 502, 503, 504:
		return apperrors.ProviderUnavailable(c.providerName, fmt.Errorf("HTTP %d: %s: %w", statusCode, sanitized, ue))
	default:
		return fmt.Errorf("HTTP %d: %s: %w", statusCode, sanitized, ue)
	}
}
```

- [ ] **Step 4: Update the caller in `client.go` to pass method+url**

Edit `internal/adapters/clients/httpx/client.go`. Find the call site at line ~247:

```go
	if httpResp.StatusCode >= 400 {
		err := c.handleHTTPError(ctx, httpResp.StatusCode, httpResp.Header, body)
```

Replace with:

```go
	if httpResp.StatusCode >= 400 {
		err := c.handleHTTPError(ctx, httpReq.Method, httpReq.URL.String(), httpResp.StatusCode, httpResp.Header, body)
```

- [ ] **Step 5: Run httpx tests to verify**

Run: `go test ./internal/adapters/clients/httpx/ -v`
Expected: All tests PASS, including the new `TestClient_UpstreamErrorAttached`.

- [ ] **Step 6: Run full test suite to confirm no regression**

Run: `go test ./internal/adapters/clients/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/clients/httpx/client_helpers.go internal/adapters/clients/httpx/client.go internal/adapters/clients/httpx/client_test.go
git commit -m "httpx: attach UpstreamError to every non-2xx response

handleHTTPError now wraps each returned error with *UpstreamError so callers
can errors.As to extract the raw status code, sanitized body, and request
id. Existing apperrors (ProviderAuthFailed, ProviderUnavailable, etc.)
semantic wrapping is preserved on the outer error — callers that only
check error codes are unaffected.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 3: Add `FailedCerts` map to `DHListingResult`

**Files:**
- Modify: `internal/domain/dhlisting/types.go` (or wherever DHListingResult is defined)
- Modify: `internal/domain/dhlisting/dh_listing_service.go`
- Test: `internal/domain/dhlisting/dh_listing_service_test.go`

- [ ] **Step 1: Find the DHListingResult definition**

Run: `grep -rn "type DHListingResult" internal/domain/dhlisting/`
Read the file. It is in `types.go`.

- [ ] **Step 2: Write the failing test**

Append to `internal/domain/dhlisting/dh_listing_service_test.go` (use the existing test setup patterns in that file — read it first if you haven't):

```go
func TestListPurchases_RecordsChannelSyncFailureInFailedCerts(t *testing.T) {
	// Mock lister: UpdateInventoryStatus succeeds, SyncChannels returns a
	// simulated upstream 422. The service must revert to in_stock AND record
	// the upstream error against the cert in FailedCerts.
	upstream := &httpx.UpstreamError{
		Provider:   "dh",
		Op:         "POST /v1/enterprise/inventory/42/sync",
		StatusCode: 422,
		Body:       `{"error":"No active channel configured for: shopify"}`,
		Message:    "No active channel configured for: shopify",
	}

	// ... use the test scaffolding already present in this file to build
	// a service with a mock lister whose SyncChannels returns the upstream
	// error and whose UpdateInventoryStatus returns success on the first
	// call and success on the revert call. Use the existing
	// `newTestService` / `fakeLister` helpers (or whatever the file uses).

	result := svc.ListPurchases(ctx, []string{"CERT-1"})

	if result.Listed != 0 {
		t.Errorf("Listed = %d, want 0 (channel sync failed)", result.Listed)
	}
	gotErr, ok := result.FailedCerts["CERT-1"]
	if !ok {
		t.Fatalf("FailedCerts missing CERT-1; got %v", result.FailedCerts)
	}
	var ue *httpx.UpstreamError
	if !errors.As(gotErr, &ue) {
		t.Fatalf("FailedCerts[CERT-1] = %v; expected to wrap *httpx.UpstreamError", gotErr)
	}
	if ue.StatusCode != 422 {
		t.Errorf("ue.StatusCode = %d, want 422", ue.StatusCode)
	}
}
```

Note: the exact scaffolding (mock types, helper names) depends on what's already in `dh_listing_service_test.go`. Read the file first; reuse the same fakes the existing channel-sync tests use. If a fake lister doesn't exist, add one minimally — do NOT introduce a new mocks package.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/domain/dhlisting/ -run TestListPurchases_RecordsChannelSyncFailureInFailedCerts -v`
Expected: FAIL with "FailedCerts" undefined or nil.

- [ ] **Step 4: Add `FailedCerts` to `DHListingResult`**

Edit the file containing `type DHListingResult struct`:

```go
type DHListingResult struct {
	Listed      int
	Synced      int
	Skipped     int
	Total       int
	Error       error            // batch-fatal error (e.g. PSA keys exhausted)
	FailedCerts map[string]error // per-cert failure reasons; nil if no skip/revert recorded an error
}
```

- [ ] **Step 5: Implement population in `dh_listing_service.go`**

Edit `internal/domain/dhlisting/dh_listing_service.go`. In `ListPurchases`:

1. Near the top of the function (after the empty-check / before the loop), declare a `failedCerts` local:

```go
	failedCerts := map[string]error{}
```

2. At every `skipped++; continue` branch in the per-cert loop, ALSO populate `failedCerts[cn] = <the err that caused the skip>`. Specifically:

   - Inline match failure (where `invID == 0` from `s.inlineMatchAndPush`): set `failedCerts[cn] = errors.New("inline match/push failed")`. (`inlineMatchAndPush` already returns 0 on failure without surfacing an error; if you can change it to return an error too without expanding scope, prefer that — otherwise the static string is OK as a documented fallback.)
   - "Not enrolled in push pipeline" branch (lines ~182–191): set `failedCerts[cn] = fmt.Errorf("not enrolled in DH push pipeline (status %s)", p.DHPushStatus)`.
   - "No committed price" branch (lines ~199–205): set `failedCerts[cn] = errors.New("no committed price; skipped list transition")`.
   - `UpdateInventoryStatus` failure final `else` branch (lines ~266–270): set `failedCerts[cn] = err` (the actual upstream error).
   - `UpdateInventoryStatus` `ErrCodeProviderNotFound` branch: set `failedCerts[cn] = err`.
   - Channel-sync failure branch (lines ~278–304): set `failedCerts[cn] = err` (the channel-sync error) BEFORE the revert (so the upstream error is the one recorded, not a revert error).
   - JSON marshal failure (lines ~310–317): set `failedCerts[cn] = marshalErr`.
   - Persist-listed-status failure (lines ~319–331): set `failedCerts[cn] = persistErr`.

3. Return the map in the final result. Change the two return statements at the end of `ListPurchases`:

```go
		return DHListingResult{
			Listed:      listed,
			Synced:      synced,
			Skipped:     len(purchases) - listed - synced,
			Total:       len(purchases),
			Error:       err,
			FailedCerts: failedCerts,
		}
```

And:

```go
	if len(failedCerts) == 0 {
		failedCerts = nil
	}
	return DHListingResult{
		Listed:      listed,
		Synced:      synced,
		Skipped:     skipped,
		Total:       len(purchases),
		FailedCerts: failedCerts,
	}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/domain/dhlisting/ -run TestListPurchases_RecordsChannelSyncFailureInFailedCerts -v`
Expected: PASS.

- [ ] **Step 7: Run full dhlisting tests to confirm no regression**

Run: `go test ./internal/domain/dhlisting/ -v`
Expected: PASS (existing tests).

- [ ] **Step 8: Commit**

```bash
git add internal/domain/dhlisting/types.go internal/domain/dhlisting/dh_listing_service.go internal/domain/dhlisting/dh_listing_service_test.go
git commit -m "dhlisting: plumb per-cert failure reasons via FailedCerts map

ListPurchases now records the actual error against the cert on every
skip/revert branch instead of just logging at WARN. Existing
Listed/Synced/Skipped/Total counts and revert-to-in_stock side-effect
on channel-sync failure are unchanged.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 4: Handler helper `dhErrorStatus`

**Files:**
- Create: `internal/adapters/httpserver/handlers/dh_errors.go`
- Test: `internal/adapters/httpserver/handlers/dh_errors_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/adapters/httpserver/handlers/dh_errors_test.go`:

```go
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

func TestDHErrorStatus(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMsgPart string
	}{
		{
			name:        "422 client passthrough",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /sync", StatusCode: 422, Message: "No active channel"},
			wantStatus:  http.StatusUnprocessableEntity,
			wantMsgPart: "No active channel",
		},
		{
			name:        "409 client passthrough",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /sync", StatusCode: 409, Message: "conflict"},
			wantStatus:  http.StatusConflict,
			wantMsgPart: "conflict",
		},
		{
			name:        "404 client passthrough",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "GET /x", StatusCode: 404, Message: "not found"},
			wantStatus:  http.StatusNotFound,
			wantMsgPart: "not found",
		},
		{
			name:        "401 remapped to 502",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /x", StatusCode: 401, Message: "bad creds"},
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "DH auth failed",
		},
		{
			name:        "403 remapped to 502",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /x", StatusCode: 403, Message: "forbidden"},
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "DH auth failed",
		},
		{
			name:        "500 mapped to 502",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /x", StatusCode: 500, Message: "boom"},
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "boom",
		},
		{
			name:        "non-upstream error becomes 502",
			err:         errors.New("dial tcp: timeout"),
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "dial tcp: timeout",
		},
		{
			name:        "wrapped upstream error is unwrapped",
			err:         fmt.Errorf("op failed: %w", &httpx.UpstreamError{Provider: "dh", Op: "POST /sync", StatusCode: 422, Message: "No active channel"}),
			wantStatus:  http.StatusUnprocessableEntity,
			wantMsgPart: "No active channel",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMsg := dhErrorStatus(tt.err)
			if gotStatus != tt.wantStatus {
				t.Errorf("status = %d, want %d", gotStatus, tt.wantStatus)
			}
			if !strings.Contains(gotMsg, tt.wantMsgPart) {
				t.Errorf("msg = %q, want it to contain %q", gotMsg, tt.wantMsgPart)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/httpserver/handlers/ -run TestDHErrorStatus -v`
Expected: FAIL — `undefined: dhErrorStatus`.

- [ ] **Step 3: Write the helper**

Create `internal/adapters/httpserver/handlers/dh_errors.go`:

```go
package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

// dhErrorStatus inspects err for an *httpx.UpstreamError and maps it to an
// HTTP status + user-facing message suitable for writeError.
//
// Routing rules:
//   - DH 4xx (except 401/403) → pass the status through (422, 409, 404, 400,
//     429, …). These are logical rejections the user can act on.
//   - DH 401/403 → 502. These are OUR credentials being bad, not the user's
//     session; surfacing 401/403 would let auth middleware/UI mistake it for
//     a session problem.
//   - DH 5xx → 502. DH is broken or unreachable.
//   - Anything else (no UpstreamError in the chain — network, timeout, parse
//     failure) → 502 with the raw error message.
func dhErrorStatus(err error) (int, string) {
	var ue *httpx.UpstreamError
	if errors.As(err, &ue) {
		switch {
		case ue.StatusCode == http.StatusUnauthorized || ue.StatusCode == http.StatusForbidden:
			return http.StatusBadGateway, fmt.Sprintf("DH auth failed (status %d): %s", ue.StatusCode, ue.Message)
		case ue.IsClientError():
			return ue.StatusCode, fmt.Sprintf("DH %s: %s", ue.Op, ue.Message)
		default:
			return http.StatusBadGateway, fmt.Sprintf("DH %s failed (status %d): %s", ue.Op, ue.StatusCode, ue.Message)
		}
	}
	return http.StatusBadGateway, err.Error()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/httpserver/handlers/ -run TestDHErrorStatus -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_errors.go internal/adapters/httpserver/handlers/dh_errors_test.go
git commit -m "handlers: add dhErrorStatus helper for upstream-aware error routing

4xx passes through (422→422, 409→409, 404→404, etc.),
401/403 remap to 502 to avoid auth-middleware confusion, 5xx and
non-upstream errors map to 502.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 5: Wire `dhErrorStatus` + `FailedCerts` into `HandleListPurchaseOnDH`

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_dh_listing.go:97-128`
- Modify: `internal/adapters/httpserver/handlers/campaigns_dh_listing_test.go`

- [ ] **Step 1: Update existing test that asserts "check server logs"**

Open `internal/adapters/httpserver/handlers/campaigns_dh_listing_test.go`. Find the cases at lines ~173 and ~193 that assert `"check server logs"` and `"will retry automatically"`.

Replace those `wantErrSubstr` strings with the new specific behavior. For the existing case that simulated a channel-sync failure (the test that currently exercises the "check server logs" branch), change it to drive an `*httpx.UpstreamError{StatusCode: 422, Message: "No active channel configured for: shopify", Op: "POST /v1/enterprise/inventory/X/sync"}` from the mock lister's `SyncChannels` and assert:

```go
wantStatus:    http.StatusUnprocessableEntity, // 422 passthrough
wantErrSubstr: "No active channel configured for: shopify",
```

For the case that asserted `"will retry automatically"`, change it to drive a 5xx upstream error and assert:

```go
wantStatus:    http.StatusBadGateway,
wantErrSubstr: "status 500",
```

(Adapt exact field names to whatever the existing test struct uses — read the file first.)

- [ ] **Step 2: Add a new explicit table-driven test case**

In the same test file, add the canonical case for the bug we diagnosed:

```go
{
	name: "DH channel sync returns 422 — handler surfaces it as 422",
	setup: func(svc *mockDHListingSvc) {
		svc.result = dhlisting.DHListingResult{
			Listed: 0, Synced: 0, Skipped: 1, Total: 1,
			FailedCerts: map[string]error{
				"CERT-1": &httpx.UpstreamError{
					Provider:   "dh",
					Op:         "POST /v1/enterprise/inventory/42/sync",
					StatusCode: 422,
					Body:       `{"error":"No active channel configured for: shopify"}`,
					Message:    "No active channel configured for: shopify",
				},
			},
		}
	},
	wantStatus:    http.StatusUnprocessableEntity,
	wantErrSubstr: "No active channel configured for: shopify",
},
```

(The exact mock setup depends on what `campaigns_dh_listing_test.go` already uses. Read the file and follow its pattern.)

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/adapters/httpserver/handlers/ -run TestHandleListPurchaseOnDH -v`
Expected: FAIL — new and updated cases fail because the handler still returns 502 "check server logs".

- [ ] **Step 4: Update the handler**

Edit `internal/adapters/httpserver/handlers/campaigns_dh_listing.go`. Replace the block from line 97 (`if result.Listed == 0 {`) through line 128 (`}`) with:

```go
	if result.Listed == 0 {
		// Prefer the upstream reason captured per-cert by the service.
		if certErr, ok := result.FailedCerts[p.CertNumber]; ok && certErr != nil {
			h.logger.Warn(r.Context(), "dh listing: per-cert failure",
				observability.Err(certErr), observability.String("purchaseId", purchaseID))
			status, msg := dhErrorStatus(certErr)
			writeError(w, status, msg)
			return
		}
		// No per-cert error captured. Re-read the purchase to give a specific
		// reason based on push status.
		updated, readErr := h.service.GetPurchase(r.Context(), purchaseID)
		if readErr != nil {
			writeError(w, http.StatusBadGateway, "DH listing failed with no recorded upstream error")
			return
		}
		if updated.DHStatus == inventory.DHStatusListed {
			writeJSON(w, http.StatusOK, result)
			return
		}
		if updated.DHInventoryID == 0 {
			switch updated.DHPushStatus {
			case inventory.DHPushStatusUnmatched:
				writeError(w, http.StatusConflict, "Cert could not be matched to a DH card")
			case inventory.DHPushStatusHeld:
				writeError(w, http.StatusConflict, "DH push is held for review — approve it first")
			case inventory.DHPushStatusDismissed:
				writeError(w, http.StatusConflict, "DH push was dismissed for this purchase")
			default:
				writeError(w, http.StatusBadGateway, "DH listing failed with no recorded upstream error")
			}
			return
		}
		writeError(w, http.StatusBadGateway, "DH listing failed with no recorded upstream error")
		return
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapters/httpserver/handlers/ -run TestHandleListPurchaseOnDH -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_dh_listing.go internal/adapters/httpserver/handlers/campaigns_dh_listing_test.go
git commit -m "handlers: surface DH listing per-cert upstream errors verbatim

When a single-cert ListPurchases returns Listed=0, route the per-cert
failure through dhErrorStatus so upstream 4xx pass through (422→422)
and the upstream message reaches the response body. Drops both
'check server logs for details' and 'will retry automatically on next
sync' strings; the new 'no recorded upstream error' fallback is honest
about its own opacity rather than directing the user to inaccessible
logs.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: Apply `dhErrorStatus` to remaining DH handlers

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_reconcile_handler.go:39,82`
- Modify: `internal/adapters/httpserver/handlers/dh_unmatch_handler.go:74`
- Modify: `internal/adapters/httpserver/handlers/dh_fix_match_handler.go:116-119`
- Modify: `internal/adapters/httpserver/handlers/dh_retry_match_handler.go:83`
- Modify: `internal/adapters/httpserver/handlers/dh_select_match_handler.go:130-133`

- [ ] **Step 1: Read each file to confirm current behavior**

Run: `for f in dh_reconcile_handler.go dh_unmatch_handler.go dh_fix_match_handler.go dh_retry_match_handler.go dh_select_match_handler.go; do echo "=== $f ==="; grep -n "StatusBadGateway" internal/adapters/httpserver/handlers/$f; done`

- [ ] **Step 2: Replace each generic 502 with dhErrorStatus**

For each location, replace the pattern:

```go
writeError(w, http.StatusBadGateway, "DH API error")
```

(or `pushErr.Error()`, or `"failed to delete DH inventory item"`, etc.) with:

```go
status, msg := dhErrorStatus(err) // or pushErr / reconcileErr — use the actual variable name
writeError(w, status, msg)
```

Preserve all existing structured logging at the same call site. Only the `writeError` line changes.

Specifically:

**`dh_reconcile_handler.go:39`** — `writeError(w, http.StatusBadGateway, "reconcile failed: "+err.Error())` → replace with `status, msg := dhErrorStatus(err); writeError(w, status, "reconcile failed: "+msg)` (keep the "reconcile failed:" prefix so the existing context isn't lost).

**`dh_reconcile_handler.go:82`** — `writeError(w, http.StatusBadGateway, "DH reconcile failed")` → if there's an error variable in scope at that point, route it through `dhErrorStatus`; if not (truly no error info), keep as-is and add a comment noting why.

**`dh_unmatch_handler.go:74`** — Same pattern: route the actual error through `dhErrorStatus`.

**`dh_fix_match_handler.go:116-119`** — Currently has TWO `writeError` calls (pushErr.Error and "DH API error"). Replace both with `status, msg := dhErrorStatus(pushErr); writeError(w, status, msg)`.

**`dh_retry_match_handler.go:83`** — Same pattern.

**`dh_select_match_handler.go:130-133`** — Same pattern as `dh_fix_match_handler.go`.

- [ ] **Step 3: Verify no existing test asserts on these specific 502 strings**

Run: `grep -rn "DH API error\|reconcile failed\|failed to delete DH inventory" internal/adapters/httpserver/handlers/*_test.go`

If any test asserts on these exact strings as expected output, update it to assert on the new format produced by `dhErrorStatus` (e.g. `"dh POST /v1/...: status XXX: ..."`).

- [ ] **Step 4: Run the full handlers test suite**

Run: `go test ./internal/adapters/httpserver/handlers/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_reconcile_handler.go internal/adapters/httpserver/handlers/dh_unmatch_handler.go internal/adapters/httpserver/handlers/dh_fix_match_handler.go internal/adapters/httpserver/handlers/dh_retry_match_handler.go internal/adapters/httpserver/handlers/dh_select_match_handler.go
git commit -m "handlers: route remaining DH handler errors through dhErrorStatus

dh_reconcile, dh_unmatch, dh_fix_match, dh_retry_match, dh_select_match
now use the same upstream-aware error mapping as HandleListPurchaseOnDH.
Upstream 4xx pass through; 401/403/5xx/network become 502 with the
upstream message attached.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 7: Full repo verification

- [ ] **Step 1: Run the full test suite with race detection**

Run: `go test -race -timeout 10m ./...`
Expected: All packages PASS.

- [ ] **Step 2: Run quality checks**

Run: `make check`
Expected: PASS (lint + architecture + file size).

- [ ] **Step 3: Build the binary**

Run: `go build -o /tmp/slabledger ./cmd/slabledger`
Expected: clean build, no errors.

- [ ] **Step 4: Spot-verify the canonical bug locally**

Smoke test by reading the new code path end-to-end:

```bash
grep -n "FailedCerts\[p.CertNumber\]\|dhErrorStatus" internal/adapters/httpserver/handlers/campaigns_dh_listing.go
grep -n "FailedCerts" internal/domain/dhlisting/dh_listing_service.go
```

Confirm:
1. `campaigns_dh_listing.go` looks up `FailedCerts[p.CertNumber]` before any fallback.
2. `dh_listing_service.go` sets `failedCerts[cn] = err` on the channel-sync failure path.

- [ ] **Step 5: No commit (verification only)**

---

## Task 8: Update memory + done

- [ ] **Step 1: Append to `~/.claude/projects/-workspace/memory/MEMORY.md`**

Add this line under "Feedback":

```
- [feedback_error_surfacing_pattern.md](feedback_error_surfacing_pattern.md) — When an upstream client returns 4xx with a structured body, surface the upstream status + message through the handler. Don't collapse to a generic 502 "check server logs". Pattern: httpx.UpstreamError + dhErrorStatus helper. DH paths fixed 2026-05-26; ~100 other 500 sites remain for follow-up.
```

Create `~/.claude/projects/-workspace/memory/feedback_error_surfacing_pattern.md` with the same content expanded with an example.

- [ ] **Step 2: Push the branch and open PR**

```bash
git push -u origin dh-error-surface
gh pr create --title "fix: surface upstream DH error status + body to user" --body "$(cat <<'EOF'
## Summary

- Adds `httpx.UpstreamError{StatusCode, Body, Message, RequestID, Op}` attached to every non-2xx httpx response via `errors.As`-able wrapping.
- Plumbs per-cert failure reasons through `dhlisting.DHListingResult.FailedCerts`.
- Adds handler helper `dhErrorStatus` that passes upstream 4xx through (422→422, 409→409, 404→404, …) except 401/403 which remap to 502.
- Applies the helper to 6 DH handlers (`campaigns_dh_listing`, `dh_reconcile`, `dh_unmatch`, `dh_fix_match`, `dh_retry_match`, `dh_select_match`).
- Drops `"check server logs for details"` and `"will retry automatically on next sync"` strings.

## Test plan

- [x] `go test -race -timeout 10m ./...`
- [x] `make check`
- [x] New unit tests cover: `UpstreamError` extraction, httpx attachment for 4xx/5xx, `dhErrorStatus` routing matrix, and the canonical DH 422 channel-sync passthrough through `HandleListPurchaseOnDH`.
- [x] Existing channel-sync revert behavior (purchase reverts to `in_stock`) preserved.

## Out of scope

- ~100 `InternalServerError` sites in handlers outside the DH paths
- Non-DH external services (CL, MM, PSA-Exchange, Google Sheets) — same pattern likely applies; tracked in MEMORY.md.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Done**

---

## Self-Review

**Spec coverage:**
- `dh.UpstreamError` → relocated to `httpx.UpstreamError` with design-discovery note in plan header. ✅
- `FailedCerts` on `DHListingResult` → Task 3. ✅
- Handler helper with 4xx passthrough + 401/403→502 carve-out → Task 4. ✅
- HandleListPurchaseOnDH refactor → Task 5. ✅
- Other 5 DH handlers → Task 6. ✅
- New 422 test case proving upstream body reaches response → Task 5 Step 2. ✅
- Revert-to-`in_stock` preserved → Task 3 Step 5 ordering (set `failedCerts` BEFORE revert). ✅
- Memory note → Task 8. ✅
- `go test -race ./...` → Task 7 Step 1. ✅

**Placeholder scan:** Task 6 has some "actual variable name" / "if there's an error variable in scope" language — that's necessary because the file contents at line 82 of `dh_reconcile_handler.go` and line 74 of `dh_unmatch_handler.go` haven't been read yet in the plan. Steps direct the engineer to read them first. Not a placeholder, an instruction to inspect.

**Type consistency:** `UpstreamError` fields (`Provider`, `Op`, `StatusCode`, `Body`, `Message`, `RequestID`) used consistently across Tasks 1, 2, 3, 4, 5. `FailedCerts map[string]error` consistent across Tasks 3, 5. `dhErrorStatus(err error) (int, string)` signature consistent across Tasks 4, 5, 6.
