# Maintainability Round 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix documentation gaps, standardize social errors, migrate 3 clients to httpx, and split 3 files over 500 lines.

**Architecture:** Documentation edits, error pattern standardization, httpx client migrations (same pattern as round 1's image_client.go), and pure file reorganization within existing packages. No new interfaces, no behavioral changes (except httpx adding retry/circuit breaker).

**Tech Stack:** Go 1.26, httpx client library, bash

**Spec:** `docs/superpowers/specs/2026-03-26-maintainability-round2-design.md`

**Branch:** `guarzo/refactor`

**Cross-cutting rule:** In every code task (2–8), remove comments that merely restate what the code does. Keep comments that explain *why* something is non-obvious. Apply only to files touched in that task.

**User preferences:** Do NOT use worktree isolation. Always run `golangci-lint run` after code changes.

---

## Task 1: D1 — Documentation fixes (CLAUDE.md)

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Fix migration count**

Find `(19 pairs, 000001–000019)` on line 86 and replace with `(21 pairs, 000001–000021)`.
Find `19 migration pairs (\`000001\`–\`000019\`)` on line 228 and replace with `21 migration pairs (\`000001\`–\`000021\`)`.

- [ ] **Step 2: Add `make check` to Quick Commands**

In the Quick Commands bash block (after `npm test` line ~20), add:

```
# Quality
make check                                 # Full quality check (lint + architecture + file size)
```

- [ ] **Step 3: Add Quality Checks section**

After the `## Resilience Patterns` section (after line ~184), add:

```markdown
## Quality Checks

- `make check` — runs lint + architecture import check + file size check
- `scripts/check-imports.sh` — fails if domain packages import adapter packages (hexagonal invariant)
- `scripts/check-file-size.sh` — warns at 500 lines, fails at 600 lines (excludes test files and mocks)
```

- [ ] **Step 4: Replace Environment Variables section**

Replace the entire `## Environment Variables` section (lines 99–128, from the heading through the closing ` ``` `) with:

```markdown
## Environment Variables

See `.env.example` for the complete list with descriptions. Key groups:

- **Required**: `PRICECHARTING_TOKEN`
- **AI**: `AZURE_AI_ENDPOINT`, `AZURE_AI_API_KEY`, `AZURE_AI_DEPLOYMENT`
- **Auth**: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `ENCRYPTION_KEY`
- **Schedulers**: `PRICE_REFRESH_ENABLED`, `ADVISOR_REFRESH_HOUR`, `SOCIAL_CONTENT_HOUR`

Full reference: [.env.example](.env.example)
```

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: fix migration count, add make check, simplify env vars section"
```

---

## Task 2: D2 — Standardize `social/errors.go`

**Files:**
- Modify: `internal/domain/social/errors.go`

- [ ] **Step 1: Rewrite errors.go to use ErrorCode/NewAppError pattern**

Replace the entire file contents with:

```go
package social

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	ErrCodePostNotFound  errors.ErrorCode = "ERR_POST_NOT_FOUND"
	ErrCodeNotConfigured errors.ErrorCode = "ERR_NOT_CONFIGURED"
	ErrCodeNotPublishable errors.ErrorCode = "ERR_NOT_PUBLISHABLE"
)

var (
	ErrPostNotFound  = errors.NewAppError(ErrCodePostNotFound, "post not found")
	ErrNotConfigured = errors.NewAppError(ErrCodeNotConfigured, "instagram publishing not configured")
	ErrNotPublishable = errors.NewAppError(ErrCodeNotPublishable, "cannot publish: caption not ready")
)

func IsPostNotFound(err error) bool  { return errors.HasErrorCode(err, ErrCodePostNotFound) }
func IsNotConfigured(err error) bool { return errors.HasErrorCode(err, ErrCodeNotConfigured) }
func IsNotPublishable(err error) bool { return errors.HasErrorCode(err, ErrCodeNotPublishable) }
```

- [ ] **Step 2: Verify callers still work**

The callers in `internal/adapters/httpserver/handlers/instagram.go` already use `errors.Is()`:
```go
case errors.Is(err, social.ErrNotConfigured):
case errors.Is(err, social.ErrPostNotFound), errors.Is(err, social.ErrNotPublishable):
```

The `errors.NewAppError` type implements the `error` interface and supports `errors.Is()`, so callers need no changes.

- [ ] **Step 3: Verify compilation and tests**

Run:
```bash
go build ./internal/domain/social/... ./internal/adapters/httpserver/...
go test ./internal/domain/social/... ./internal/adapters/httpserver/...
golangci-lint run ./internal/domain/social/...
```
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add internal/domain/social/errors.go
git commit -m "refactor: standardize social/errors.go to ErrorCode/NewAppError pattern

Matches the error pattern used by all other domain packages (campaigns,
favorites, etc.). Adds error codes and predicate functions."
```

---

## Task 3: D3a — Migrate `psa/client.go` to httpx.Client

**Files:**
- Modify: `internal/adapters/clients/psa/client.go`

**Pattern reference:** See `internal/adapters/clients/azureai/image_client.go` for the completed migration pattern from round 1.

- [ ] **Step 1: Read the file and understand current usage**

Read `internal/adapters/clients/psa/client.go` fully. Identify:
- The `httpClient *http.Client` field
- The constructor where `&http.Client{Timeout: 15 * time.Second}` is created
- All places where `c.httpClient.Do(req)` is called
- All `http.NewRequestWithContext` calls
- Auth pattern (Bearer token)
- Response body handling (`json.NewDecoder(resp.Body).Decode()` or `io.ReadAll`)

- [ ] **Step 2: Change field type and constructor**

Change `httpClient *http.Client` to `httpClient *httpx.Client`.

Replace the raw `http.Client` construction with:
```go
httpCfg := httpx.DefaultConfig("PSA")
httpCfg.DefaultTimeout = 15 * time.Second
```
And `httpClient: httpx.NewClient(httpCfg)`.

- [ ] **Step 3: Replace HTTP calls with httpx equivalents**

For each `http.NewRequestWithContext` + `c.httpClient.Do(req)` call:
- Build a `headers` map with the auth header
- Replace with `c.httpClient.Get(ctx, url, headers, timeout)`
- Replace `resp.Body` reads with direct `resp.Body` (already `[]byte` in httpx)
- Remove `resp.Body.Close()` defers
- Remove manual status code checks (httpx returns errors for non-2xx)
- Replace `json.NewDecoder(resp.Body).Decode(&target)` with `json.Unmarshal(resp.Body, &target)`

- [ ] **Step 4: Clean up imports and remove redundant comments**

Remove unused imports (`"io"`, `"net/http"` if no longer needed). Keep `"net/http"` only if `http.StatusXxx` constants are still referenced. Remove comments that just restate function names.

- [ ] **Step 5: Verify**

```bash
go build ./internal/adapters/clients/psa/...
go test ./internal/adapters/clients/psa/...
golangci-lint run ./internal/adapters/clients/psa/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/psa/client.go
git commit -m "refactor: migrate psa/client.go to httpx.Client

Adds retry + circuit breaker resilience, consistent with other API clients."
```

---

## Task 4: D3b — Migrate `instagram/client.go` to httpx.Client

**Files:**
- Modify: `internal/adapters/clients/instagram/client.go`

- [ ] **Step 1: Read the file and understand current usage**

Read `internal/adapters/clients/instagram/client.go` fully. Identify:
- The `httpClient *http.Client` field
- Constructor creating `&http.Client{Timeout: 30 * time.Second}`
- All `c.httpClient.Do(req)` calls (both GET and POST)
- Auth pattern (query parameter tokens, not headers)
- Response body handling (`io.ReadAll`)
- The polling loop (not streaming — safe for httpx)

- [ ] **Step 2: Change field type and constructor**

Change `httpClient *http.Client` to `httpClient *httpx.Client`.

Replace with:
```go
httpCfg := httpx.DefaultConfig("Instagram")
httpCfg.DefaultTimeout = 30 * time.Second
```
And `httpClient: httpx.NewClient(httpCfg)`.

- [ ] **Step 3: Replace HTTP calls with httpx equivalents**

For GET calls: `c.httpClient.Get(ctx, url, headers, timeout)` — tokens are in URL query params, so headers may be nil or empty.

For POST calls: `c.httpClient.Post(ctx, url, headers, body, timeout)` — Content-Type and form data.

Replace `io.ReadAll(resp.Body)` with direct `resp.Body` access. Remove `resp.Body.Close()` defers. Remove manual status code checks.

- [ ] **Step 4: Clean up imports and remove redundant comments**

Remove unused imports. Remove comments that restate function names.

- [ ] **Step 5: Verify**

```bash
go build ./internal/adapters/clients/instagram/...
go test ./internal/adapters/clients/instagram/...
golangci-lint run ./internal/adapters/clients/instagram/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/instagram/client.go
git commit -m "refactor: migrate instagram/client.go to httpx.Client

Adds retry + circuit breaker resilience, consistent with other API clients."
```

---

## Task 5: D3c — Migrate `google/oauth.go` to httpx.Client

**Files:**
- Modify: `internal/adapters/clients/google/oauth.go`

- [ ] **Step 1: Read the file and understand current usage**

Read `internal/adapters/clients/google/oauth.go` fully. Identify:
- The `httpClient *http.Client` field
- Constructor creating `&http.Client{Timeout: 10 * time.Second}`
- POST call for token exchange (form-encoded body)
- GET call for user info (Bearer auth header)
- Response body handling (`io.ReadAll`)

- [ ] **Step 2: Change field type and constructor**

Change `httpClient *http.Client` to `httpClient *httpx.Client`.

Replace with:
```go
httpCfg := httpx.DefaultConfig("GoogleOAuth")
httpCfg.DefaultTimeout = 10 * time.Second
```
And `httpClient: httpx.NewClient(httpCfg)`.

- [ ] **Step 3: Replace HTTP calls with httpx equivalents**

For the token exchange POST: `c.httpClient.Post(ctx, url, headers, body, timeout)` with `"Content-Type": "application/x-www-form-urlencoded"` header.

For the user info GET: `c.httpClient.Get(ctx, url, headers, timeout)` with `"Authorization": "Bearer " + token` header.

Replace `io.ReadAll(resp.Body)` with direct `resp.Body` access. Remove `resp.Body.Close()` defers. Remove manual status code checks.

- [ ] **Step 4: Clean up imports and remove redundant comments**

Remove unused imports. Remove comments that restate function names.

- [ ] **Step 5: Verify**

```bash
go build ./internal/adapters/clients/google/...
go test ./internal/adapters/clients/google/...
golangci-lint run ./internal/adapters/clients/google/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/google/oauth.go
git commit -m "refactor: migrate google/oauth.go to httpx.Client

Adds retry + circuit breaker resilience, consistent with other API clients."
```

---

## Task 6: D4 — Split `purchases_repository_queries.go` into 2 files

**Files:**
- Modify: `internal/adapters/storage/sqlite/purchases_repository_queries.go` (keep lookups + field updates)
- Create: `internal/adapters/storage/sqlite/purchases_repository_pricing.go` (price overrides + eBay export)

- [ ] **Step 1: Create `purchases_repository_pricing.go`**

Read `purchases_repository_queries.go` fully. Create `purchases_repository_pricing.go` with these functions CUT from the original:
- `UpdatePurchasePriceOverride`
- `UpdateReviewedPrice`
- `UpdatePurchaseAISuggestion`
- `GetPriceOverrideStats`
- `ClearPurchaseAISuggestion`
- `AcceptAISuggestion`
- `SetEbayExportFlag`
- `ClearEbayExportFlags`
- `ListEbayFlaggedPurchases`

Package is `sqlite`. Add needed imports.

- [ ] **Step 2: Remove moved code from `purchases_repository_queries.go`**

Delete the moved functions. Clean up unused imports. Remove any redundant comments that restate function names.

- [ ] **Step 3: Verify**

```bash
go build ./internal/adapters/storage/sqlite/...
go test ./internal/adapters/storage/sqlite/...
golangci-lint run ./internal/adapters/storage/sqlite/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/purchases_repository_queries.go \
       internal/adapters/storage/sqlite/purchases_repository_pricing.go
git commit -m "refactor: extract pricing and eBay export queries from purchases_repository_queries.go

Move price override, AI suggestion, and eBay export flag operations into
purchases_repository_pricing.go. Lookups and field updates stay."
```

---

## Task 7: D5 — Split `suggestion_rules.go` into 2 files

**Files:**
- Modify: `internal/domain/campaigns/suggestion_rules.go` (keep rules 1–4)
- Create: `internal/domain/campaigns/suggestion_rules_optimization.go` (rules 5–7)

- [ ] **Step 1: Create `suggestion_rules_optimization.go`**

Read `suggestion_rules.go` fully. Create `suggestion_rules_optimization.go` with these functions CUT from the original:
- `suggestSpendCapRebalancing` (rule 5)
- `suggestCharacterAdjustments` (rule 6)
- `suggestPhaseTransitions` (rule 7)

Package is `campaigns`. Add needed imports. Helper functions (`confidenceLabel`, `confidenceLabelWithAge`, `gradeRangeFromLabel`, `SplitInclusionList`, `isMarketplaceChannel`) are defined in other files in the same package — they don't need to be moved.

- [ ] **Step 2: Remove moved code from `suggestion_rules.go`**

Delete the three moved functions. Clean up unused imports. Remove redundant comments.

- [ ] **Step 3: Verify**

```bash
go build ./internal/domain/campaigns/...
go test ./internal/domain/campaigns/...
golangci-lint run ./internal/domain/campaigns/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/campaigns/suggestion_rules.go \
       internal/domain/campaigns/suggestion_rules_optimization.go
git commit -m "refactor: split suggestion_rules.go into expansion and optimization files

Rules 1-4 (expansion/coverage) stay in suggestion_rules.go.
Rules 5-7 (optimization/lifecycle) move to suggestion_rules_optimization.go."
```

---

## Task 8: D6 — Split `service_import_psa.go` into 2 files

**Files:**
- Modify: `internal/domain/campaigns/service_import_psa.go` (keep main orchestration)
- Create: `internal/domain/campaigns/service_import_psa_enrich.go` (background enrichment)

- [ ] **Step 1: Create `service_import_psa_enrich.go`**

Read `service_import_psa.go` fully. Create `service_import_psa_enrich.go` with these functions CUT from the original:
- `batchResolveCardIDs` (method on `*service`)
- `certEnrichWorker` (method on `*service`)
- `enrichSingleCert` (method on `*service`)

Package is `campaigns`. Add needed imports.

- [ ] **Step 2: Remove moved code from `service_import_psa.go`**

Delete the three moved functions. Clean up unused imports. Remove redundant comments.

- [ ] **Step 3: Verify**

```bash
go build ./internal/domain/campaigns/...
go test ./internal/domain/campaigns/...
golangci-lint run ./internal/domain/campaigns/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/campaigns/service_import_psa.go \
       internal/domain/campaigns/service_import_psa_enrich.go
git commit -m "refactor: extract cert enrichment from service_import_psa.go

Move batchResolveCardIDs, certEnrichWorker, and enrichSingleCert into
service_import_psa_enrich.go. Main import orchestration stays."
```

---

## Task 9: Final verification

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: SUCCESS

- [ ] **Step 2: Full test suite**

Run: `go test -race -count=1 ./...`
Expected: All PASS

- [ ] **Step 3: Quality checks**

Run: `make check`
Expected: Architecture check passes. File size check passes (warnings expected for `main.go` 576L and `router.go` 508L only — the 3 newly split files should be under 500).

- [ ] **Step 4: Verify file sizes**

Run: `./scripts/check-file-size.sh`
Expected: No failures. The files we split should now show:
- `purchases_repository_queries.go` < 400L (was 582)
- `suggestion_rules.go` < 300L (was 552)
- `service_import_psa.go` < 400L (was 517)
