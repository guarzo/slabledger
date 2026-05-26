# DH Error Surfacing — Design

**Date:** 2026-05-26
**Branch:** `dh-error-surface`
**Status:** draft, pending review

## Problem

The slabledger backend hides upstream DH errors behind generic 502 responses
and "check server logs" messages. Concrete instance from production
(2026-05-26):

- User clicks "List on DH" → response is `502 Bad Gateway` with body
  `{"error":"DH listing failed — check server logs for details"}`.
- DH actually returned `HTTP 422
  {"error":"No active channel configured for: shopify"}` from
  `POST /api/v1/enterprise/inventory/{id}/sync` — a config issue, not a
  network failure.
- The error message tells the user nothing actionable. Significant time was
  wasted hypothesizing about DH outages, Cloudflare, key rotation, circuit
  breakers — none of which were the cause.

Two layers conspire to hide the real error:

1. **Service layer**
   (`internal/domain/dhlisting/dh_listing_service.go:266–270`): channel-sync
   error is logged at WARN and the purchase is reverted to `in_stock`, but
   the error is **not propagated** back through `ListPurchases`. The result
   struct exposes `Listed/Synced/Skipped/Total` but no per-cert reason.
2. **Handler layer**
   (`internal/adapters/httpserver/handlers/campaigns_dh_listing.go:97–128`):
   when `result.Listed == 0`, fallthrough branches return one of two generic
   502 messages ("check server logs for details" / "will retry automatically
   on next sync") regardless of cause.

Same pattern exists in the other DH handlers (`dh_reconcile_handler.go`,
`dh_unmatch_handler.go`, `dh_fix_match_handler.go`, `dh_retry_match_handler.go`,
`dh_select_match_handler.go`) wherever an upstream DH error is collapsed into
`writeError(w, http.StatusBadGateway, "DH API error")` or similar.

## Goals

When an upstream DH call returns a specific, actionable error, the user sees
that specific error with a status code that matches the upstream's intent —
not a generic 502.

**In scope:** DH-call paths only. Specifically:

- `internal/adapters/clients/dh/` — DH HTTP client error surface
- `internal/domain/dhlisting/dh_listing_service.go` — service-level error
  plumbing
- Six DH handlers:
  - `campaigns_dh_listing.go`
  - `dh_reconcile_handler.go`
  - `dh_unmatch_handler.go`
  - `dh_fix_match_handler.go`
  - `dh_retry_match_handler.go`
  - `dh_select_match_handler.go`

**Out of scope** (noted as follow-up):

- ~100+ `InternalServerError` sites outside the DH paths
- Non-DH external services (CardLadder, MarketMovers, PSA-Exchange, Google
  Sheets) — same anti-pattern likely applies but is not part of this PR.
- Service boundary or type renames, file restructuring.

## Anti-patterns this fixes

1. **Status-code laundering.** Handler returns 502 (or 500) when the
   underlying call returned 4xx. A 422 from upstream is a logical rejection,
   not a bad gateway.
2. **Reason swallowing.** Service logs an error at WARN/ERROR but returns a
   result struct that doesn't carry the reason. Handler has nothing to
   surface.
3. **"Check server logs" messages.** The user can't see server logs. If we
   have a reason, return it. If we don't, say so honestly.
4. **Generic catch-all branches.** Fallthrough `else`/`default` clauses that
   return one error string regardless of cause.

## Design

### 1. `dh.UpstreamError` (new)

In `internal/adapters/clients/dh/` (new file `errors.go` or
`upstream_error.go`):

```go
package dh

// UpstreamError represents a non-2xx response from DH. Returned from the DH
// HTTP client wherever the request reaches DH and DH returns a status that
// isn't success. Network errors, timeouts, and circuit-breaker trips return
// a different (non-UpstreamError) error so handlers can distinguish
// "DH said no" from "we couldn't reach DH".
type UpstreamError struct {
    StatusCode int    // upstream HTTP status (e.g. 422)
    Body       string // upstream response body (trimmed)
    Message    string // best-effort extracted message (e.g. JSON {"error": "..."})
    RequestID  string // x-request-id header if present
    Op         string // logical operation, e.g. "sync_channels", "update_inventory", "psa_import"
}

func (e *UpstreamError) Error() string {
    if e.Message != "" {
        return fmt.Sprintf("dh %s: status %d: %s", e.Op, e.StatusCode, e.Message)
    }
    return fmt.Sprintf("dh %s: status %d", e.Op, e.StatusCode)
}

func (e *UpstreamError) IsClientError() bool {
    return e.StatusCode >= 400 && e.StatusCode < 500
}
```

Returned from the DH client wherever it currently does
`fmt.Errorf("dh: ... status %d ...", ...)`. The existing
`apperrors.HasErrorCode(err, ErrCodeProviderNotFound)` path keeps working —
the client still wraps 404→provider-not-found at the call site that already
does that mapping; `UpstreamError` is used for the cases that currently get
swallowed.

`Message` extraction: when content-type is JSON and body parses to a map with
an `error` or `message` string field, use that. Otherwise use the trimmed
body (capped, e.g. 500 chars) so we don't accidentally dump a Cloudflare HTML
page into a `writeError`.

### 2. `DHListingResult.FailedCerts`

Add a map to the existing result type:

```go
type DHListingResult struct {
    Listed      int
    Synced      int
    Skipped     int
    Total       int
    Error       error             // batch-fatal (e.g. PSA keys exhausted)
    FailedCerts map[string]error  // per-cert failure reasons; nil if nothing failed
}
```

Populated on every `continue` branch in `ListPurchases` that previously only
logged. Specifically:

- Inline match/push failure → `FailedCerts[cert] = err`
- `UpdateInventoryStatus` non-fatal failure (the `else` branch around
  `dh_listing_service.go:266`) → `FailedCerts[cert] = err`
- Channel-sync failure (the revert path at
  `dh_listing_service.go:278`) → `FailedCerts[cert] = err`
- Persistence failure post-sync → `FailedCerts[cert] = persistErr`
- "Not enrolled in push pipeline" → `FailedCerts[cert] =
  errors.New("not enrolled in DH push pipeline")` (or skip — see note below)

Existing `Error` field semantics unchanged (used for `ErrPSAKeysExhausted`
batch short-circuit).

Existing batch counts (`Listed`/`Synced`/`Skipped`/`Total`) and the revert-
to-`in_stock` side effect on channel-sync failure remain unchanged.

### 3. Handler routing helper

In the handlers package (small private helper, probably in a new
`dh_errors.go` or appended to an existing handlers util file):

```go
// dhErrorStatus inspects err for an *dh.UpstreamError and returns (status,
// message) suitable for writeError. Routing rules:
//
//   - DH 4xx (except 401/403) → pass status through (422, 409, 404, 400, 429, …).
//     These are logical rejections; the user can act on them.
//   - DH 401/403           → 502 (DH credentials problem, not the user's session).
//   - DH 5xx               → 502 (DH is broken / unreachable).
//   - Anything else        → 502 with err.Error() (network, timeout, parse failure).
func dhErrorStatus(err error) (int, string) {
    var ue *dh.UpstreamError
    if errors.As(err, &ue) {
        switch {
        case ue.StatusCode == 401 || ue.StatusCode == 403:
            return http.StatusBadGateway,
                fmt.Sprintf("DH auth failed (status %d): %s", ue.StatusCode, ue.Message)
        case ue.IsClientError():
            return ue.StatusCode, fmt.Sprintf("DH %s: %s", ue.Op, ue.Message)
        default:
            return http.StatusBadGateway,
                fmt.Sprintf("DH %s failed (status %d): %s", ue.Op, ue.StatusCode, ue.Message)
        }
    }
    return http.StatusBadGateway, err.Error()
}
```

### 4. `campaigns_dh_listing.go` HandleListPurchaseOnDH

Modified flow when `result.Listed == 0`:

1. If `result.Error != nil`, route through the existing `ErrPSAKeysExhausted`
   branch (unchanged) or `dhErrorStatus(result.Error)`.
2. Otherwise, look up `result.FailedCerts[p.CertNumber]`:
   - If present and `errors.As` to `*dh.UpstreamError`, call `dhErrorStatus`
     and return.
   - If present but not an UpstreamError, return `502` with the error's
     `Error()` string (no "check server logs").
3. Otherwise (no error recorded), re-read the purchase and fall through to
   the existing `DHPushStatus`-based 409 branches (Held/Unmatched/Dismissed).
4. Drop both "check server logs for details" and "will retry automatically
   on next sync" strings. The new `FailedCerts` plumbing should make the
   "no recorded reason" branch unreachable in practice. As a defensive
   fallback, return `502` with `"DH listing failed with no recorded
   upstream error"` so the message is honest about its own opacity rather
   than directing the user to logs they can't access.

### 5. Other DH handlers

For each of `dh_reconcile_handler.go`, `dh_unmatch_handler.go`,
`dh_fix_match_handler.go`, `dh_retry_match_handler.go`,
`dh_select_match_handler.go`: replace
`writeError(w, http.StatusBadGateway, "DH API error")` and equivalent calls
with `status, msg := dhErrorStatus(err); writeError(w, status, msg)`.

Existing structured logging at the handler entry points is preserved (still
log the full error context at the appropriate level).

## Error handling matrix

| Upstream | Old behavior | New behavior |
|---|---|---|
| 422 channel-sync ("No active channel for: shopify") | 502, "check server logs" | **422**, "DH sync_channels: No active channel configured for: shopify" |
| 409 from DH | 502 "DH API error" | **409**, "DH {op}: {body}" |
| 404 from DH (non-provider-not-found path) | 502 "DH API error" | **404**, "DH {op}: {body}" |
| 401/403 (DH creds bad) | 502 "DH API error" | **502**, "DH auth failed (status 401): …" |
| 500 from DH | 502 "DH API error" | **502**, "DH {op} failed (status 500): {body}" |
| Network timeout | 502 "DH API error" | **502**, "{err.Error()}" |
| Provider-not-found (existing path) | unchanged (inline reset + skip) | unchanged |
| PSA keys exhausted | 502 "deferred — please try again" | **unchanged** (already specific) |
| Held/Unmatched/Dismissed precondition | 409 with specific message | **unchanged** |

## Side effects preserved

- Channel-sync failure still reverts the purchase to `in_stock` and persists
  the revert locally.
- Existing event recording (`dhevents.TypeListed`, `TypeChannelSynced`,
  `TypeListDeferred`) unchanged.
- Provider-not-found auto-reset flow (`ResetDHFieldsForRepush`) unchanged.
- PSA keys exhausted batch short-circuit unchanged.

## Frontend

No response shape changes. The body is still `{error: string}`. No
`web/src/types/` changes needed.

## Testing

### New tests

`campaigns_dh_listing_test.go`:

- **422 channel sync passthrough**: mock `DHInventoryLister` such that
  `SyncChannels` returns `&dh.UpstreamError{StatusCode: 422, Body:
  '{"error":"No active channel configured for: shopify"}', Message: "No active
  channel configured for: shopify", Op: "sync_channels"}`. Assert response
  is **422** (not 502) and body contains the upstream message. Assert the
  purchase was reverted to `in_stock`.
- **502 on DH 500**: `SyncChannels` returns
  `&dh.UpstreamError{StatusCode: 500, ...}`. Assert response is **502** with
  upstream body in error message.
- **502 on network error**: `SyncChannels` returns
  `fmt.Errorf("dial tcp: timeout")`. Assert response is **502** with the
  raw error message.
- **502 on 401**: `SyncChannels` returns
  `&dh.UpstreamError{StatusCode: 401, ...}`. Assert response is **502**,
  message starts with "DH auth failed".

DH client tests (`internal/adapters/clients/dh/`):

- `UpstreamError.IsClientError` table-driven cases.
- Client returns `UpstreamError` for synthetic 4xx and 5xx upstream
  responses, with JSON `Message` extraction.

### Updated tests

Existing assertions on `"check server logs"` and
`"will retry automatically"` in `campaigns_dh_listing_test.go` updated to
assert the new specific behavior (either the upstream message or the
fallback path).

### Existing tests

Run `go test -race ./...` to confirm no regressions across the handlers and
domain packages.

## Memory note

Add `feedback_error_surfacing_pattern.md` to
`~/.claude/projects/-workspace/memory/MEMORY.md` capturing the anti-pattern
and the fix template (UpstreamError → handler helper) for future passes on
non-DH paths.

## Out-of-scope notes (for follow-up)

- ~100 `InternalServerError` sites in handlers — many likely share this
  pattern. A future audit can apply the same UpstreamError approach to other
  clients.
- CL, MM, PSA-Exchange, Google Sheets handler error paths.
- Possible consolidation of `apperrors.HasErrorCode` and `dh.UpstreamError`
  into one error model — defer.
