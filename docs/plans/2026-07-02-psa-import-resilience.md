# PSA Import Resilience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop bulk cert import from hammering an open PSA circuit breaker (A), and preserve PSA's `Retry-After` value end-to-end in the returned error (B Part 1).

**Architecture:** Two isolated backend changes. (A) adds a batch-scoped "PSA is down" latch to the `ImportCerts` loop so remaining certs are queued for retry without issuing doomed lookups. (B1) captures the `Retry-After`-carrying error from the httpx layer inside the PSA client's `doRequest` and returns it instead of a fresh empty rate-limit error. No frontend change (the intake UI already partitions on `retryable`), no migration, no new interface.

**Tech Stack:** Go 1.26, hexagonal architecture, table-driven tests, `httptest` for client tests.

**Spec:** `docs/specs/2026-07-02-psa-import-resilience-design.md`
**Worktree:** `.worktrees/psa-import-resilience` on branch `fix/psa-import-resilience` (baseline `origin/main` @ `b177514d`). All commits land here.

## Global Constraints

- TDD — write the failing test first, verify red → green.
- Hexagonal: domain depends only on interfaces; never import adapter packages from domain.
- Mocks from `internal/testutil/mocks/` (Fn-field pattern) — but this package already has a local `mockCertLookup` in `service_cert_entry_test.go`; reuse it, do not create inline mocks.
- Table-driven tests. `ctx` is the first arg. Money in cents. Source files < 500 lines.
- Do NOT change the `CertImportResult` / `CertImportError` JSON shape — the frontend consumes `retryable`.
- Run `go test -race ./...` and `make check` green before claiming done.
- Commit messages end with: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`

---

### Task 1: Task A — stop bulk import after the breaker opens

**Files:**
- Modify: `internal/domain/inventory/service_cert_entry.go` (add helper ~after line 50; add latch + early-stop in `ImportCerts` loop ~lines 90–158)
- Test: `internal/domain/inventory/service_cert_entry_test.go` (add one test function)

**Interfaces:**
- Consumes: `apperrors.HasErrorCode(err, code)`, `apperrors.ErrCodeProviderCircuitOpen`, `apperrors.ErrCodeProviderRateLimit`, `apperrors.ProviderCircuitOpen(provider)` — all already exist.
- Produces: `isPSAUnavailableError(err error) bool` (unexported, package `inventory`). No exported surface change.

**Behavior to implement:** Once `LookupCert` returns a circuit-open or rate-limit error, set a batch-scoped `psaUnavailable` latch. For every remaining not-yet-attempted cert, skip the lookup and append `CertImportError{Retryable: true, Error: "PSA temporarily unavailable — queued for retry"}`. The already-exists branch (top of loop, no PSA call) is unaffected. A per-cert timeout/not-found does NOT set the latch.

- [ ] **Step 1: Write the failing test**

Add to `internal/domain/inventory/service_cert_entry_test.go` (reuses the existing local `mockCertLookup` and `newMockRepo` helpers in that file):

```go
// TestImportCerts_StopsCallingPSAAfterBreakerOpens guards the bulk-import
// incident: once PSA's circuit breaker opens mid-batch, every remaining cert
// would otherwise issue a doomed lookup that fails instantly with
// ERR_PROV_CIRCUIT_OPEN. The loop must latch "PSA unavailable" on the first
// such error and queue the rest for retry without calling PSA again.
func TestImportCerts_StopsCallingPSAAfterBreakerOpens(t *testing.T) {
	repo := newMockRepo()
	repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}

	var calls int
	certLookup := &mockCertLookup{
		lookupFn: func(_ context.Context, _ string) (*CertInfo, error) {
			calls++
			return nil, apperrors.ProviderCircuitOpen("PSA")
		},
	}
	svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

	certs := []string{"1", "2", "3", "4", "5"}
	result, err := svc.ImportCerts(context.Background(), certs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("LookupCert called %d times, want 1 (stop after breaker opens)", calls)
	}
	if result.Failed != len(certs) {
		t.Errorf("Failed = %d, want %d", result.Failed, len(certs))
	}
	if len(result.Errors) != len(certs) {
		t.Fatalf("Errors length = %d, want %d", len(result.Errors), len(certs))
	}
	for i, e := range result.Errors {
		if !e.Retryable {
			t.Errorf("Errors[%d] (cert %q) Retryable = false, want true", i, e.CertNumber)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace/.worktrees/psa-import-resilience && go test ./internal/domain/inventory/ -run TestImportCerts_StopsCallingPSAAfterBreakerOpens -v`
Expected: FAIL — `LookupCert called 5 times, want 1` (the latch does not exist yet; every cert still calls PSA).

- [ ] **Step 3: Add the `isPSAUnavailableError` helper**

In `internal/domain/inventory/service_cert_entry.go`, immediately after the `isRetryableImportError` function (after line 50, before `func (s *service) ImportCerts`):

```go
// isPSAUnavailableError reports whether an error indicates PSA itself is
// unreachable batch-wide (circuit breaker open or rate-limited), as opposed to
// a failure specific to one cert (not-found, a single timeout). When true, the
// import loop stops issuing lookups for the remaining certs and queues them for
// retry instead — hammering an open breaker only produces instant
// ERR_PROV_CIRCUIT_OPEN noise and delays recovery.
func isPSAUnavailableError(err error) bool {
	return apperrors.HasErrorCode(err, apperrors.ErrCodeProviderCircuitOpen) ||
		apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit)
}
```

- [ ] **Step 4: Declare the latch before the loop**

In `ImportCerts`, immediately before the `for _, certNum := range cleaned {` line (currently line 91), insert:

```go
	// Once PSA signals it is unavailable batch-wide (breaker open or rate
	// limited), stop issuing lookups for the remaining certs — they would only
	// produce instant circuit-open failures. Queue them for retry instead.
	psaUnavailable := false

```

- [ ] **Step 5: Add the early-stop check and the latch-set**

In the same file, locate this block (currently lines 143–158):

```go
		if s.certLookup == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: "cert lookup not configured"})
			continue
		}

		info, certErr := s.certLookup.LookupCert(ctx, certNum)
		if certErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      certErr.Error(),
				Retryable:  isRetryableImportError(certErr),
			})
			continue
		}
```

Replace it with (adds the `psaUnavailable` early-stop before the lookup, and the latch-set after a batch-wide failure):

```go
		if s.certLookup == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: "cert lookup not configured"})
			continue
		}

		// PSA already signalled batch-wide unavailability earlier in this
		// import — do not issue another doomed lookup; queue this cert.
		if psaUnavailable {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      "PSA temporarily unavailable — queued for retry",
				Retryable:  true,
			})
			continue
		}

		info, certErr := s.certLookup.LookupCert(ctx, certNum)
		if certErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      certErr.Error(),
				Retryable:  isRetryableImportError(certErr),
			})
			// A breaker-open / rate-limit error means PSA is down for the whole
			// batch, not just this cert — latch it so remaining certs are
			// queued instead of hammering the open breaker.
			if isPSAUnavailableError(certErr) {
				psaUnavailable = true
			}
			continue
		}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd /workspace/.worktrees/psa-import-resilience && go test ./internal/domain/inventory/ -run TestImportCerts_StopsCallingPSAAfterBreakerOpens -v`
Expected: PASS.

- [ ] **Step 7: Run the full inventory package to check for regressions**

Run: `cd /workspace/.worktrees/psa-import-resilience && go test -race ./internal/domain/inventory/`
Expected: PASS (existing `TestImportCerts_ClassifiesTransientFailures` and other `ImportCerts` tests still green — single-cert and non-breaker paths are unchanged).

- [ ] **Step 8: Commit**

```bash
cd /workspace/.worktrees/psa-import-resilience
git add internal/domain/inventory/service_cert_entry.go internal/domain/inventory/service_cert_entry_test.go
git commit -m "$(cat <<'EOF'
fix(inventory): stop bulk cert import after PSA breaker opens

Once LookupCert returns a circuit-open or rate-limit error, latch
"PSA unavailable" for the rest of the batch and queue remaining certs as
Retryable instead of issuing lookups that fail instantly with
ERR_PROV_CIRCUIT_OPEN. Response shape unchanged; the intake UI already
renders retryable certs as amber "will retry" rows.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Task B Part 1 — preserve `Retry-After` end-to-end in the PSA client

**Files:**
- Modify: `internal/adapters/clients/psa/client.go` (`doRequest`, lines ~148–208)
- Test: `internal/adapters/clients/psa/client_test.go` (add one test function)

**Interfaces:**
- Consumes: `c.httpClient.Get(...)` already returns a `429` error wrapping `apperrors.ProviderRateLimited(provider, retryAfter)` whose `Context["reset_time"]` holds the header value (see `internal/adapters/clients/httpx/client_helpers.go` case `429`).
- Produces: no signature change. `doRequest` now returns the httpx-origin rate-limit error (carrying `reset_time`) instead of a fresh empty `apperrors.ProviderRateLimited("PSA", "")` on the two real-429 return paths.

**Why two return sites:** with N comma-separated tokens, each 429 calls `rotateToken()` and `continue`s, so the production bulk path exhausts the loop and returns at **line ~207**. The single-token path returns at **line ~196**. Both must propagate. The daily-limit guard at **line ~160** is left as-is (no upstream call, no header).

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/clients/psa/client_test.go` (uses the existing `newTestClient` helper; `errors` and `apperrors` are already imported):

```go
// TestDoRequest_PreservesRetryAfter proves a 429's Retry-After header survives
// from the httpx layer through doRequest into the returned AppError's
// reset_time context, rather than being flattened into an empty
// ProviderRateLimited. Covers both real-429 return paths: single-token (no
// rotation) and multi-token (loop exhausted after all tokens 429).
func TestDoRequest_PreservesRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
	}{
		{name: "single token, no rotation", tokens: nil},
		{name: "multi token, all 429", tokens: []string{"token-a", "token-b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "86115")
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer server.Close()

			c := newTestClient(t, server.URL, tc.tokens...)
			_, err := c.GetCert(context.Background(), "12345678")
			if err == nil {
				t.Fatal("expected rate-limit error")
			}
			if !apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit) {
				t.Fatalf("expected ErrCodeProviderRateLimit in chain, got %v", err)
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected AppError in chain, got %T: %v", err, err)
			}
			got, _ := appErr.Context["reset_time"].(string)
			if got != "86115" {
				t.Errorf("reset_time = %q, want %q", got, "86115")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace/.worktrees/psa-import-resilience && go test ./internal/adapters/clients/psa/ -run TestDoRequest_PreservesRetryAfter -v`
Expected: FAIL — both subtests fail on `reset_time = "" , want "86115"` (doRequest currently returns a fresh empty `ProviderRateLimited("PSA", "")` whose `Context` is nil).

- [ ] **Step 3: Declare the capture variable**

In `internal/adapters/clients/psa/client.go`, in `doRequest`, immediately after `maxAttempts := len(c.tokens)` (line 148), insert:

```go
	// Capture the most recent 429 error from httpx — it carries the parsed
	// Retry-After header in its reset_time context. On a multi-token client
	// every attempt 429s and the loop exhausts, so we return this at the end
	// rather than a fresh empty ProviderRateLimited.
	var lastRateLimitErr error
```

- [ ] **Step 4: Capture and propagate in the real-429 branch**

Locate this block (currently lines 188–197):

```go
			// 429: rotate to the next token and retry rather than giving up immediately.
			if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
				if c.rotateToken() {
					c.logger.Info(ctx, "PSA "+opName+": rate limited, retrying with backup key",
						observability.String("cert", certNumber))
					continue
				}
				c.logger.Warn(ctx, "PSA "+opName+": rate limited, no backup keys available",
					observability.String("cert", certNumber))
				return nil, apperrors.ProviderRateLimited("PSA", "")
			}
```

Replace it with:

```go
			// 429: rotate to the next token and retry rather than giving up immediately.
			if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
				// httpx's error carries the Retry-After header in its
				// reset_time context; keep it so callers (and a future paced
				// retry) can read the reset window.
				lastRateLimitErr = err
				if c.rotateToken() {
					c.logger.Info(ctx, "PSA "+opName+": rate limited, retrying with backup key",
						observability.String("cert", certNumber))
					continue
				}
				c.logger.Warn(ctx, "PSA "+opName+": rate limited, no backup keys available",
					observability.String("cert", certNumber))
				return nil, err
			}
```

- [ ] **Step 5: Propagate at the loop-exhausted return**

Locate the final return of `doRequest` (currently line 207):

```go
	return nil, apperrors.ProviderRateLimited("PSA", "")
}
```

Replace it with:

```go
	if lastRateLimitErr != nil {
		return nil, lastRateLimitErr
	}
	return nil, apperrors.ProviderRateLimited("PSA", "")
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd /workspace/.worktrees/psa-import-resilience && go test ./internal/adapters/clients/psa/ -run TestDoRequest_PreservesRetryAfter -v`
Expected: PASS (both subtests).

- [ ] **Step 7: Run the full PSA package to check for regressions**

Run: `cd /workspace/.worktrees/psa-import-resilience && go test -race ./internal/adapters/clients/psa/`
Expected: PASS. In particular `TestClient_GetCert_ErrorTypes/rate_limited_returns_ProviderRateLimited` still passes — with no `Retry-After` header the wrapped error still satisfies `Code == ErrCodeProviderRateLimit` (only `reset_time` context differs, which that test does not assert).

- [ ] **Step 8: Commit**

```bash
cd /workspace/.worktrees/psa-import-resilience
git add internal/adapters/clients/psa/client.go internal/adapters/clients/psa/client_test.go
git commit -m "$(cat <<'EOF'
fix(psa): preserve Retry-After in rate-limit error instead of dropping it

doRequest discarded httpx's 429 error (which carries the Retry-After header
in reset_time) and returned a fresh empty ProviderRateLimited. Capture and
return the httpx error on both real-429 paths: single-token (line ~196) and
loop-exhausted after multi-token rotation (line ~207). The daily-limit guard
is unchanged — no upstream call means no header. Typed code stays
ERR_PROV_RATE_LIMIT so import classification is unaffected.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Full race test suite**

Run: `cd /workspace/.worktrees/psa-import-resilience && go test -race ./...`
Expected: PASS across all packages.

- [ ] **Step 2: Quality checks**

Run: `cd /workspace/.worktrees/psa-import-resilience && make check`
Expected: lint clean, architecture import check passes (no domain→adapter imports introduced), file-size check passes (`service_cert_entry.go` gains ~20 lines, well under the 500 warn / 600 fail thresholds).

- [ ] **Step 3: Confirm no frontend or API-shape drift**

Run: `cd /workspace/.worktrees/psa-import-resilience && git diff --stat origin/main`
Expected: only `internal/domain/inventory/service_cert_entry.go`, `internal/domain/inventory/service_cert_entry_test.go`, `internal/adapters/clients/psa/client.go`, `internal/adapters/clients/psa/client_test.go`, and the two docs files (spec + this plan). No `web/` changes, no migration files.

---

## Self-Review Notes

- **Spec coverage:** Task A → spec §"Task A" (latch + `isPSAUnavailableError` + queued error, response shape preserved). Task B1 → spec §"Task B Part 1" (capture + propagate at lines 196 & 207, line 160 left as-is). Descoped items (D, B Part 2) have no tasks — correct.
- **Accepted limitation** (spec §"Accepted limitation"): a daily-quota exhaustion also carries `ErrCodeProviderRateLimit`, so Task A's latch will stop the batch on a daily-quota wall too, and those certs get the same amber "click Import again" path as a burst. This is intended for A+B scope; distinguishing burst vs day-scale was Task B Part 2 (descoped). No task needed.
- **Type consistency:** `isPSAUnavailableError` and `psaUnavailable` are the only new identifiers; both used exactly as defined. `lastRateLimitErr` is local to `doRequest`.
- **No placeholders:** every step shows the exact code or exact command + expected output.
