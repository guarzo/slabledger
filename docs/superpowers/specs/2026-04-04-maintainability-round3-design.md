# Maintainability Round 3 Design

**Goal:** Comprehensive maintainability pass — standardize remaining error patterns, split 4 files approaching/exceeding 500 lines, add test coverage for the two most critical undertested packages, and remove vestigial code.

**Branch:** `guarzo/refactor`

**Builds on:** Rounds 1 and 2 (`2026-03-26-maintainability-design.md`, `2026-03-26-maintainability-round2-design.md`)

---

## Scope

| Category | Items | Tasks |
|----------|-------|-------|
| Error standardization | advisor, auth, ai | 3 |
| File splits | tools_portfolio, import_parsing, httpx/client, service_advanced | 4 |
| Test coverage | HTTP handlers, SQLite repositories | 2 |
| Cleanup | pricecharting wrapper, CLAUDE.md | 2 |
| **Total** | | **11** |

### Explicitly out of scope

- `main.go` split (568L — reads well top-to-bottom, scoped out in round 2)
- `azureai/client.go` httpx migration (uses raw `*http.Client` intentionally for SSE streaming with custom retry; already uses `httpx.DefaultTransport()`)
- `advisor.go` handler tests (SSE streaming tests deserve their own task)
- `social_repository.go` tests (18 methods — too large for this round)
- campaigns.Service interface segregation (62 methods, stable)

---

## E1: Standardize `advisor/errors.go`

**Problem:** The advisor package uses `fmt.Errorf` for domain errors. Three genuine domain errors lack structured codes.

**Files:** Create `internal/domain/advisor/errors.go`

**Error codes:**

| Code | Sentinel | Message |
|------|----------|---------|
| `ERR_ADVISOR_MAX_ROUNDS` | `ErrMaxRoundsExceeded` | `exceeded maximum tool call rounds` |
| `ERR_ADVISOR_TOOL_PANIC` | `ErrToolPanic` | `tool executor panicked` |
| `ERR_ADVISOR_UNSUPPORTED_TYPE` | `ErrUnsupportedDataType` | `unsupported factor data type` |

**Predicate functions:** `IsMaxRoundsExceeded`, `IsToolPanic`, `IsUnsupportedDataType`

**Caller updates:**
- `service_impl.go:408` — replace `fmt.Errorf("exceeded maximum tool rounds (%d)...")` with `ErrMaxRoundsExceeded` (add round count and tool names via `WithContext`)
- `service_impl.go:371` — wrap panic recovery error with `ErrToolPanic`
- `scoring.go:77` — replace `fmt.Errorf("unsupported factor data type: %T"...)` with `ErrUnsupportedDataType` (add type via `WithContext`)

**Pattern:** Follow `internal/domain/campaigns/errors.go` exactly.

---

## E2: Standardize `auth/errors.go`

**Problem:** `auth/types.go` defines `ErrUserNotFound = errors.New("user not found")` using stdlib `errors.New` instead of the project's `NewAppError` pattern.

**Files:**
- Create `internal/domain/auth/errors.go`
- Modify `internal/domain/auth/types.go` (remove old sentinel)

**Error codes:**

| Code | Sentinel | Message |
|------|----------|---------|
| `ERR_AUTH_USER_NOT_FOUND` | `ErrUserNotFound` | `user not found` |

**Predicate functions:** `IsUserNotFound`

**Caller impact:** Callers already use `errors.Is(err, auth.ErrUserNotFound)`. Since `NewAppError` supports `errors.Is` via `Unwrap`, callers need no changes.

**Note:** OAuth errors (token exchange, provider failures) are adapter-level concerns — they don't need domain error codes.

---

## E3: Type-safe `ClassifyAIError` in `ai/tracking.go`

**Problem:** `ClassifyAIError` uses string matching (`strings.Contains(errMsg, "rate limit")`) to classify errors. With the `ErrorCode` pattern available, this can be type-safe.

**Files:** Modify `internal/domain/ai/tracking.go`

**Approach:** Add error code awareness to `ClassifyAIError`:

1. First check if the error chain contains known `ErrorCode` values (e.g., `ErrCodeProviderRateLimit` from `domain/errors`) using `errors.HasErrorCode`
2. Fall back to string matching for errors from providers that don't use `AppError` yet

```go
func ClassifyAIError(err error) (AIStatus, string) {
    if err == nil {
        return AIStatusSuccess, ""
    }
    errMsg := err.Error()
    if domainerrors.HasErrorCode(err, domainerrors.ErrCodeProviderRateLimit) {
        return AIStatusRateLimited, errMsg
    }
    // Fallback: string matching for non-AppError providers
    if strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "too_many_requests") ||
        strings.Contains(errMsg, "429") || strings.Contains(errMsg, "capacity exceeded") {
        return AIStatusRateLimited, errMsg
    }
    return AIStatusError, errMsg
}
```

**No new error codes created** — this package classifies errors, it doesn't define them.

---

## D1: Split `tools_portfolio.go` (543L) into 2 files

**Problem:** Largest non-test file in the codebase at 543 lines.

**Files:**
- Modify: `internal/adapters/advisortool/tools_portfolio.go`
- Create: `internal/adapters/advisortool/tools_portfolio_analysis.go`

**Split:**

`tools_portfolio.go` — Health, monitoring, and dashboard tools (~250L):
- `registerGetPortfolioHealth`
- `registerGetPortfolioInsights`
- `registerGetCreditSummary`
- `registerGetWeeklyReview`
- `registerGetCapitalTimeline`
- `registerGetDashboardSummary` (+ `dashboardSummary` struct)

`tools_portfolio_analysis.go` — Analysis, pricing, and batch tools (~290L):
- `registerGetExpectedValues`
- `registerGetCrackCandidates`
- `registerGetCampaignSuggestions`
- `registerRunProjection`
- `registerGetChannelVelocity`
- `registerGetCertLookup`
- `registerEvaluatePurchase`
- `registerSuggestPrice`
- `registerGetSuggestionStats`
- `registerGetAcquisitionTargets`
- `registerGetCrackOpportunities`
- `registerSuggestPriceBatch` (+ `jsonSchema` struct)
- `registerGetExpectedValuesBatch`

**Rationale:** Health/monitoring is "how is my portfolio doing?" vs analysis is "what should I do next?" — clean conceptual boundary.

---

## D2: Split `import_parsing.go` (489L) into 2 files

**Problem:** Mix of low-level title parsing and higher-level metadata extraction in one file.

**Files:**
- Modify: `internal/domain/campaigns/import_parsing.go`
- Create: `internal/domain/campaigns/import_parsing_metadata.go`

**Split:**

`import_parsing.go` — Core title parsing and card number extraction (~260L):
- Regex pattern definitions (`hashCardNumberRegex`, `bareCardNumberRegex`, `gradeRegex`, `cardNumberTokenRegex`)
- `isInvalidCardNumber`, `isGenericSetName`
- `ExtractCardNumberFromPSATitle`
- `ExtractGrade`
- `ParsePSAListingTitle`, `parsePSAListingTitleWithIndex`
- `isLetter`, `isLetterOrDigit`, `looksLikeCollectorNumber`
- `parseNoNumberTitle` (+ `noNumberCardPatterns`, `noNumberStopWords`)

`import_parsing_metadata.go` — Metadata extraction, set resolution, variants (~230L):
- `resolvePSACategory` (+ `psaCategoryToSetName` map)
- `variantPattern` struct, `variantPatterns` list
- `stripVariantTokens`
- `extractCardNameFromPSATitle`
- `parseCardMetadataFromTitle`
- `resolveSetName`
- `stripCollectionSuffix`
- `extractYearFromTitle`
- `extractVariantFromTitle`
- `ExportParseCardMetadataFromTitle`, `ExportIsGenericSetName` (test wrappers)

**Rationale:** Title parsing (extracting tokens from raw strings) vs metadata assembly (combining parsed tokens into structured data) are separate concerns.

---

## D3: Split `httpx/client.go` (483L) into 2 files

**Problem:** Core retry/circuit-breaker execution mixed with convenience methods and error handling utilities.

**Files:**
- Modify: `internal/adapters/clients/httpx/client.go`
- Create: `internal/adapters/clients/httpx/client_helpers.go`

**Split:**

`client.go` — Core types and execution (~300L):
- `Observer` interface, `NoopObserver`
- `Config` struct, `DefaultConfig`
- `DefaultTransport`
- `Option` type, `WithLogger`
- `Client` struct, `NewClient`
- `Do` (retry + circuit breaker orchestration)
- `doRequest` (single HTTP execution)

`client_helpers.go` — Convenience API, error handling, response sanitization (~180L):
- `Request`, `Response` structs
- `Get`, `GetJSON`, `Post`, `PostJSON`
- `handleHTTPError`
- `sanitizeResponseBody`, `extractHTMLSummary`
- `isTimeoutError`
- `GetCircuitBreakerStats`

**Rationale:** Consumers of httpx primarily use the convenience methods. Separating them from the retry/circuit-breaker internals makes both files easier to navigate.

---

## D4: Split `service_advanced.go` (475L) into 2 files

**Problem:** Mixes cert/value operations with arbitrage/activation analysis.

**Files:**
- Modify: `internal/domain/campaigns/service_advanced.go`
- Create: `internal/domain/campaigns/service_arbitrage.go`

**Split:**

`service_advanced.go` — Cert operations, value analysis, simulation (~260L):
- `LookupCert`
- `QuickAddPurchase`
- `GetExpectedValues`
- `EvaluatePurchase`
- `RunProjection`

`service_arbitrage.go` — Arbitrage analysis and activation validation (~215L):
- `GetCrackCandidates`
- `GetActivationChecklist`
- `GetCrackOpportunities`
- `GetAcquisitionTargets`

**Rationale:** "What is this card/purchase worth?" (value analysis) vs "Where are the cross-campaign opportunities?" (arbitrage) are distinct decision domains.

---

## T1: Handler tests — campaigns_finance, picks, social

**Problem:** HTTP handlers at 25.9% coverage. Three high-value handler files have zero tests.

**Files:**
- Create: `internal/adapters/httpserver/handlers/campaigns_finance_test.go`
- Create: `internal/adapters/httpserver/handlers/picks_handler_test.go`
- Create: `internal/adapters/httpserver/handlers/social_test.go`

**Pattern:** Follow existing handler test conventions (reference: `handlers/auth_test.go`, `handlers/campaigns_test.go`):
- `httptest.NewRequest` + `httptest.NewRecorder`
- Fn-field mock structs implementing domain service interfaces
- Table-driven tests with `[]struct`
- Import mocks from `internal/testutil/mocks/` where available

**Coverage targets per file:**

`campaigns_finance_test.go`:
- Happy path for each handler method (record sale, get finance summary, credit operations)
- Error paths: campaign not found, invalid input, service error

`picks_handler_test.go`:
- Happy path for each CRUD operation (create, list, get, update, delete picks)
- Error paths: not found, validation errors

`social_test.go`:
- Happy path for post management (list posts, get post, update status)
- Error paths: post not found, invalid status transition, service error

**Scope:** Happy paths + key error paths. Not exhaustive edge-case coverage.

---

## T2: SQLite tests — purchases_repository, sales_repository

**Problem:** SQLite storage at 28.6% coverage. The core purchases repository (17 methods, 13K) has zero tests.

**Files:**
- Create: `internal/adapters/storage/sqlite/purchases_repository_test.go`
- Create: `internal/adapters/storage/sqlite/sales_repository_test.go`

**Pattern:** Follow existing SQLite test conventions (reference: `sqlite/auth_repository_test.go`, `sqlite/campaigns_repository_test.go`):
- Temporary file-based SQLite database per test
- `setupTestDB` helper that creates schema + returns cleanup function
- Table-driven tests with `[]struct`
- Direct SQL verification of side effects where needed

**Coverage targets per file:**

`purchases_repository_test.go`:
- Core CRUD: CreatePurchase, GetPurchase, ListPurchasesByCampaign, CountPurchasesByCampaign
- Key updates: UpdatePurchaseBuyCost, UpdatePurchaseCampaign, UpdatePurchaseGrade, UpdatePurchaseCardMetadata
- Lookups: GetPurchaseIDByCertNumber, ListUnsoldPurchases, ListAllUnsoldPurchases
- Error paths: duplicate cert numbers, not found, invalid campaign ID

`sales_repository_test.go`:
- All methods (small file): create sale, list sales, get sale
- Error paths: duplicate sale, sale not found, referential integrity (purchase must exist)

**Scope:** Core CRUD + constraint violations. Batch queries and less-used lookups are out of scope for this round.

---

## C1: Remove vestigial `lookupByQueryWithRetry` wrapper

**Problem:** `pricecharting/pc_client.go:27-29` is a pure passthrough wrapper:

```go
func (p *PriceCharting) lookupByQueryWithRetry(ctx context.Context, query string) (*PCMatch, error) {
    return p.lookupByQueryInternal(ctx, query)
}
```

The name implies retry logic, but httpx handles retries internally. The comment acknowledges it's vestigial.

**Files:** Modify:
- `internal/adapters/clients/pricecharting/pc_client.go` (remove function + comments)
- `internal/adapters/clients/pricecharting/lookup_strategies.go` (2 call sites → call `lookupByQueryInternal` directly)
- `internal/adapters/clients/pricecharting/circuit_breaker_integration_test.go` (update test name + call sites)

---

## C2: CLAUDE.md verification

**Problem:** CLAUDE.md needs to stay accurate as the codebase evolves.

**Files:** Modify `CLAUDE.md`

**Checks:**
- Migration count: verify "30 migration pairs" matches actual count (currently correct)
- Architecture tree: verify file paths still accurate after splits
- Any new patterns or conventions introduced in this round

**Note:** This is a verification task, not a rewrite. Only update what's actually stale.

---

## Cross-cutting rules

1. **Comment hygiene:** In every code task, remove comments that merely restate what the code does. Keep comments that explain *why* something is non-obvious. Apply only to files touched in that task.
2. **Lint check:** Run `golangci-lint run` after every code task.
3. **No behavioral changes:** All changes are structural (file splits, error pattern migration, test additions). No logic changes except the `ClassifyAIError` improvement in E3 and removing the pricecharting wrapper in C1.
