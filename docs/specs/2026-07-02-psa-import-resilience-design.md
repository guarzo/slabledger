# PSA Import Resilience — Design (A + B Part 1)

**Date:** 2026-07-02
**Branch:** `fix/psa-import-resilience` (worktree `.worktrees/psa-import-resilience`)
**Baseline:** `origin/main` @ `b177514d`
**Origin brief:** `docs/private/2026-07-01-psa-import-resilience-prompt.md`
**Scope decision:** A + B Part 1 only. Tasks D (durable retry queue) and B Part 2
(magnitude classifier) are **out of scope** for this cycle.

---

## Background

Bulk cert import (94 certs) on the scan-intake screen repeatedly tripped PSA's
per-IP rate limit and the shared circuit breaker. PR #442 (merged, present at
`origin/main`) already classifies transient PSA failures as `Retryable:true` and
the intake UI stages them as amber "will retry" rows the operator re-imports with
one click. This design closes two remaining gaps on top of #442.

### Why the brief's line numbers differ from the code

The brief was written against a tree slightly ahead of a stale local checkout.
This design is reconciled against `origin/main` @ `b177514d`. Notably, the brief
points Task B at `doRequest` "lines 184–192" (a single-token return); with PSA's
9-token rotation that return does not fire on the real bulk path. See Task B below.

---

## Task A — Stop bulk import from hammering an open breaker

### Problem

`ImportCerts` (`internal/domain/inventory/service_cert_entry.go`) calls
`s.certLookup.LookupCert` per cert. Once the breaker opens, every *remaining*
cert still issues a lookup that fails instantly with `ERR_PROV_CIRCUIT_OPEN` and
is appended to `result.Errors`. This is wasteful and clutters the result with
breaker noise instead of a clean "PSA is down, these are queued" signal.

### Change

In the `ImportCerts` per-cert loop:

1. Add a `psaUnavailable bool` flag, initially `false`.
2. Add a small predicate:

   ```go
   // isPSAUnavailableError reports whether an error indicates PSA itself is
   // unreachable batch-wide (breaker open or rate-limited), as opposed to a
   // per-cert failure. When true, remaining certs in the batch should be
   // queued for retry without issuing their lookups.
   func isPSAUnavailableError(err error) bool {
       return apperrors.HasErrorCode(err, apperrors.ErrCodeProviderCircuitOpen) ||
           apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit)
   }
   ```

3. Before issuing `LookupCert`, if `psaUnavailable` is already `true`, skip the
   call and append a queued error:

   ```go
   if psaUnavailable {
       result.Failed++
       result.Errors = append(result.Errors, CertImportError{
           CertNumber: certNum,
           Error:      "PSA temporarily unavailable — queued for retry",
           Retryable:  true,
       })
       continue
   }
   ```

4. In the existing `certErr != nil` branch (currently sets
   `Retryable: isRetryableImportError(certErr)`), after appending the error,
   flip the flag when the failure is batch-wide:

   ```go
   if isPSAUnavailableError(certErr) {
       psaUnavailable = true
   }
   ```

### Preserved behavior

- The **already-exists** branch runs at the top of the loop, before any PSA
  lookup, and touches no PSA. Certs that already exist in the DB continue to
  import correctly even after `psaUnavailable` flips — only PSA-requiring
  lookups are short-circuited.
- Certs imported earlier in the loop stay imported (no rollback).
- **Response shape is unchanged.** `web/src/react/pages/tools/CardIntakeTab.tsx`
  already partitions on `retryable` (via `importErrorStatus`), so queued certs
  render as amber "will retry" rows with no frontend change.
- Only `ErrCodeProviderCircuitOpen` and `ErrCodeProviderRateLimit` flip the
  flag. A one-off timeout on a single cert does **not** stop the batch.

### Accepted limitation (consequence of B Part 2 being descoped)

The client's own `dailyCallLimit` guard also returns `ErrCodeProviderRateLimit`,
so a **daily-quota exhaustion** trips `isPSAUnavailableError` and stops the batch
just like a 6-minute burst does — and both mark remaining certs `Retryable: true`,
producing the same amber "click Import again" banner. On a genuine daily-quota
wall, re-clicking is futile until the quota resets (~24h). Distinguishing burst
from day-scale for operator messaging was **Task B Part 2**, which is descoped
this cycle. This is a known, accepted behavior, not an oversight.

### Test

Table-driven in `internal/domain/inventory/service_cert_entry_test.go`:
`MockCertLookup.LookupCertFn` with an invocation counter returns a circuit-open
`AppError` on the Nth call. Assert:
- the counter stops incrementing at N (remaining certs issue no lookup), and
- remaining certs appear in `result.Errors` with `Retryable == true`.

Mock: `internal/testutil/mocks/cert_lookup.go` (Fn-field pattern; do not inline).

---

## Task B Part 1 — Preserve `Retry-After` end-to-end

### Problem

`internal/adapters/clients/httpx/client_helpers.go` already parses the
`Retry-After` header on a 429 into `apperrors.ProviderRateLimited(provider,
retryAfter)`, which stores it in `err.Context["reset_time"]`. But
`doRequest` (`internal/adapters/clients/psa/client.go`) **discards** that error
and returns a **fresh** `apperrors.ProviderRateLimited("PSA", "")` with an empty
reset time, so the parsed `Retry-After` never reaches the caller.

### The real return site (correction to the brief)

`doRequest` has three `ProviderRateLimited("PSA", "")` sites:

- **Line ~160** — our own `dailyCallLimit` budget guard. No PSA call was made,
  so there is **no header to propagate**. Leave as-is; out of scope.
- **Line ~196** — single-token case: a 429 arrives and `rotateToken()` returns
  `false` (no backup keys).
- **Line ~207** — loop exhausted after `maxAttempts`. **This is the production
  path**: with 9 comma-separated tokens, each 429 calls `rotateToken()` (returns
  `true` until all tried) and `continue`s, so the loop falls through to line 207.

The brief's cited "lines 184–192" corresponds to the single-token return; the
9-token bulk-import scenario that caused the incident exits at line 207. **Both
196 and 207 must propagate** or the actual failure mode gets an empty reset time.

### Change

In `doRequest`:

1. Declare `var lastRateLimitErr error` before the attempt loop.
2. In the real-429 branch (where `resp.StatusCode == http.StatusTooManyRequests`),
   set `lastRateLimitErr = err` before rotating. The httpx client was constructed
   with provider name `"PSA"`, so its error already reads
   `ProviderRateLimited("PSA", retryAfter)` with `reset_time` populated — no
   re-wrapping needed.
3. Line ~196 (no rotate left): `return nil, err` (the httpx error) instead of a
   fresh empty one.
4. Line ~207 (loop exhausted): `return nil, lastRateLimitErr` when non-nil;
   otherwise fall back to the current `apperrors.ProviderRateLimited("PSA", "")`
   (covers the theoretical case where the loop exits without a captured 429).

The typed code stays `ErrCodeProviderRateLimit`, so `isRetryableImportError`
continues to classify it and Task A's `isPSAUnavailableError` still trips.

### Honest scope note

`reset_time` currently has **no non-test consumer** (`ProviderRateLimited` is its
only producer). In an A+B world, Part 1 improves the logged/returned error and is
forward-compatible with a future Task D scheduler that would read it — it is
**not** a behavior change on its own. No capping, no classifier, no UI change
(those were Task D / B Part 2, both descoped).

### Test

In `internal/adapters/clients/psa/client_test.go`, an `httptest` server returns
`429` with `Retry-After: 86115`. Assert the error returned by the cert lookup:
- satisfies `apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit)`, and
- carries `Context["reset_time"] == "86115"`.

Cover both single-token and multi-token client construction so both the line-196
and line-207 return paths are exercised.

---

## Explicitly out of scope

- **Task D** — durable server-side paced retry queue (migration `000015`,
  Postgres store, scheduler job). Deferred to a separate spec/cycle. Client-side
  retry from #442 remains the source of truth for transient failures.
- **Task B Part 2** — magnitude classifier (burst vs day-scale) and any
  capped-backoff logic. Its only live consumer would have been D's scheduler.

---

## Conventions

- TDD — failing test first, verify red → green.
- Hexagonal: domain depends only on interfaces.
- Mocks from `internal/testutil/mocks/` (Fn-field pattern), never inline.
- Table-driven tests, `ctx` first arg, cents internally, files < 500 lines.
- `go test -race ./...` and `make check` green before claiming done.

## Files touched

| File | Task | Change |
|---|---|---|
| `internal/domain/inventory/service_cert_entry.go` | A | flag + predicate + early-stop |
| `internal/domain/inventory/service_cert_entry_test.go` | A | early-stop table test |
| `internal/adapters/clients/psa/client.go` | B1 | capture + propagate 429 error |
| `internal/adapters/clients/psa/client_test.go` | B1 | `reset_time` survives end-to-end |

No migration. No frontend change. No new domain interface.
