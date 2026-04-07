# DoubleHolo API Integration — Improvement Issues

Review date: 2026-04-07
Scope: `internal/adapters/clients/dh/`, `internal/adapters/clients/dhprice/`, `internal/adapters/clients/pricelookup/`

## Coverage Metrics

| Package | Coverage |
|---------|----------|
| `dh` | 64.8% |
| `dhprice` | 87.6% |
| `pricelookup` | 89.6% |
| **Overall** | **75.7%** |

No files exceed the 500-line threshold. All tests pass with race detector.

---

## #1: `getEnterprise` duplicates `doEnterprise` for GET requests

**Category**: Duplication | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/clients/dh/client.go:190`

`getEnterprise` (lines 190–219) is a specialized GET-only version of `doEnterprise` (lines 224–297). Both implement identical auth checking, rate limiting, header construction, JSON unmarshalling, and health recording. The only difference is `getEnterprise` uses `c.httpClient.Get()` while `doEnterprise` uses `c.httpClient.Do()` — and `doEnterprise` already handles GET semantics when body is nil. This is ~30 lines of duplicated logic.

**Suggested approach**: Replace `getEnterprise` with a call to `doEnterprise(ctx, "GET", fullURL, nil, dest)`. Reconcile the logging gap (see #6).

---

## #2: `ConvertToIntelligence` has 0% test coverage

**Category**: Tests | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/clients/dh/convert.go:11`

`ConvertToIntelligence` is a pure function that converts `MarketDataResponse` into domain `intelligence.MarketIntelligence`. It handles nil fields, time parsing, and cents conversion — all error-prone. Despite being straightforward to test (no external dependencies), it has zero coverage. It's only tested indirectly via the `dh_intelligence_refresh` scheduler.

**Suggested approach**: Add a table-driven test covering: nil response, response with all fields populated, response with partial fields (some nil pointers), and response with unparseable timestamps in sales.

---

## #3: `pricelookup.Adapter` references `pricing.Price` fields that `dhprice` never populates

**Category**: Quality | **Severity**: Medium | **Effort**: Medium (1–4hr)
**Location**: `internal/adapters/clients/pricelookup/adapter.go:141-206`

`GetMarketSnapshot` reads `price.Market`, `price.Velocity`, `price.Conservative`, `price.Distributions`, and `price.LastSoldByGrade` — none of which `dhprice.buildPrice` ever populates. Since DH is now the sole price source (previous sources removed 2026-04-06), these code paths are dead in practice. The `pricelookup.Adapter` was designed for a multi-source world that no longer exists.

**Suggested approach**: Either (a) prune the dead fields from `GetMarketSnapshot` to only handle what `dhprice` actually provides, or (b) enrich `dhprice.buildPrice` to populate `LastSoldByGrade`, `Market`, and/or `Conservative` from DH data. Option (a) is lower effort; option (b) would improve market signal quality.

---

## #4: `PSAKeyRotator` interface and `ResetPSAKeyRotation()` are never called outside package

**Category**: Dead Code | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/clients/dh/client.go:329-338`

`ResetPSAKeyRotation()` is defined and exported but never called by any consumer. The `PSAKeyRotator` interface (lines 334–338) defines both `RotatePSAKey()` and `ResetPSAKeyRotation()`, but callers only use `RotatePSAKey()` via a direct type assertion to `*dh.Client`, not through the interface. The interface itself appears unused.

**Suggested approach**: Remove `ResetPSAKeyRotation()` and the `PSAKeyRotator` interface unless there's a planned use.

---

## #5: `dh.Client` test coverage at 64.8% with key functions at 0%

**Category**: Tests | **Severity**: Medium | **Effort**: Medium (1–4hr)
**Location**: `internal/adapters/clients/dh/client.go`

Functions with 0% coverage: `WithLogger`, `WithRateLimitRPS`, `WithPSAKeys`, `Health()`, `RotatePSAKey`, `ResetPSAKeyRotation`, `IsPSARateLimitError`, `ResolveCertWithRotation`, `parsePSAKeys`. Several of these are actively used in production (`ResolveCertWithRotation`, `IsPSARateLimitError`, `parsePSAKeys`). The PSA key rotation logic handles rate limit recovery during bulk cert resolution but has no direct tests.

**Suggested approach**: Add unit tests for `parsePSAKeys` (pure function), `IsPSARateLimitError` (string matching), `ResolveCertWithRotation` (mock resolve + rotate functions), and `RotatePSAKey`/`ResetPSAKeyRotation` (state transitions). All testable without HTTP servers.

---

## #6: `getEnterprise` silently swallows logging on GET failures

**Category**: Quality | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/clients/dh/client.go:190-219`

`getEnterprise` records health failures but never logs errors — unlike `doEnterprise` which logs at both Debug (request/response) and Error (failure) levels. GET request failures (used by `CardLookup`, `RecentSales`, `Suggestions`, `ListInventory`, `GetOrders`, `GetCertResolutionJob`) are harder to diagnose than POST/PATCH/DELETE failures.

**Suggested approach**: Resolves automatically if #1 is implemented (consolidate into `doEnterprise`).

---

## #7: `dhprice.gradeKey` maps BGS 9.5 and CGC 9.5 to the same grade as PSA 9.5

**Category**: Quality | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/clients/dhprice/provider.go:36-46`

The grade mapping conflates BGS 9.5 and CGC 9.5 with PSA 9.5 into a single `pricing.GradePSA95` bucket. The `Grade95Cents` price is therefore a blended median of potentially different-value items. BGS 9.5 and CGC 9.5 carry different market premiums than PSA 9.5.

**Suggested approach**: If intentional, add a comment explaining the business rationale for the cross-grader conflation. If an oversight, consider separating these grades.

---

## #8: No test for `MarketDataEnterprise` when `CardLookup` fails

**Category**: Tests | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/clients/dh/client_test.go:256`

`MarketDataEnterprise` has tests for "full response" and "recent sales failure returns partial data", but no test for when `CardLookup` itself fails. The function treats `CardLookup` as a hard failure (returns error) vs `RecentSales` as a soft failure (returns partial data + logs warning). The hard failure path is untested.

**Suggested approach**: Add a test case where the server returns a 500 for `/api/v1/enterprise/cards/lookup` and verify the error propagates.

---

## #9: `SalesDistribution` has no PSA8 field but `pricelookup` references PSA8

**Category**: Quality | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `internal/domain/pricing/provider.go:68-72`, `internal/adapters/clients/pricelookup/adapter.go:189`

The `Distributions` struct only has `PSA10`, `PSA9`, and `Raw` fields. `GetMarketSnapshot` has a `case 8:` branch with a comment "No PSA8 distribution data available." Now that DH is the sole source and `dhprice` does compute PSA8 sales data, this gap may be fillable.

**Suggested approach**: Either add `PSA8 *SalesDistribution` to the `Distributions` struct, or leave as-is with a clearer comment. Low priority since fallback calculations handle the gap.

---

## #10: `MarshalChannels` error handling silently returns "[]"

**Category**: Quality | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/clients/dh/types_v2.go:124-133`

`MarshalChannels` swallows the `json.Marshal` error and returns `"[]"`. While `[]InventoryChannelStatus` is trivially serializable (only string fields), the pattern is fragile — if the struct ever gains a non-serializable field, the error would be invisible. Called from 6 locations.

**Suggested approach**: Return `(string, error)` or add a comment explaining why the error is safely swallowed.

---
---

# Frontend (React) — Improvement Issues

Review date: 2026-04-07
Scope: `web/src/`

## Coverage Metrics

| Metric | Value |
|--------|-------|
| ESLint warnings (strict) | 0 |
| TypeScript errors | 165 (all from missing `node_modules`) |
| Files over 500 LOC | 1 (`InventoryTab.tsx` at 506) |
| Test suites failing | 6 of 18 |
| Tests passing | 184 |
| Source files without tests | ~170 of ~185 (92%) |

---

## #1: Missing npm dependencies break typecheck and 6 test suites

**Category**: Quality | **Severity**: High | **Effort**: Small (< 1hr)
**Location**: `web/package.json` + `web/node_modules/`

Six production dependencies (`@tanstack/react-query`, `@tanstack/react-virtual`, `radix-ui`, `react-markdown`, `remark-gfm`, `html-to-image`) are declared in `package.json` and `package-lock.json` but missing from `node_modules/`. This causes all 165 TypeScript errors (46 TS2307 module-not-found + 117 TS7006 cascading `any` types + 2 others) and all 6 failing test suites. Zero errors are genuine code issues.

**Suggested approach**: Run `npm ci` in `web/`. All typecheck errors and test failures should resolve. Consider adding `npm ci` or `npm install --frozen-lockfile` to the CI pipeline if not already present.

---

## #2: No fetch-error handling on most pages

**Category**: UX | **Severity**: High | **Effort**: Medium (1–4hr)
**Location**: `web/src/react/pages/DashboardPage.tsx`, `CampaignsPage.tsx`, `CampaignDetailPage.tsx`, `ContentPage.tsx`

Most data-fetching pages never check `isError` from React Query. If the API is down or returns errors, these pages show indefinite loading spinners or render misleading empty states (e.g., `ContentPage` shows "No posts yet" on fetch failure; `CampaignDetailPage` shows "Campaign not found" on server errors). Only `GlobalInventoryPage` properly handles query errors with a retry button.

**Suggested approach**: Add `isError` / `error` checks to each page's main query, showing an inline error message with a retry button. Follow the pattern in `GlobalInventoryPage.tsx:12-24`.

---

## #3: Cache invalidation logic duplicated across 6+ files

**Category**: Duplication | **Severity**: High | **Effort**: Medium (1–4hr)
**Location**: `web/src/react/pages/campaign-detail/RecordSaleModal.tsx:143-162`, `pages/campaigns/OperationsTab.tsx:145-157`, `queries/useCampaignQueries.ts`, `pages/tools/ImportSalesTab.tsx:68-74`

Cache invalidation after mutations is copy-pasted across 6+ files with 80+ total lines of `queryClient.invalidateQueries()` calls. The sets overlap but are inconsistent — `RecordSaleModal` invalidates `channelVelocity` and `suggestions`, but `OperationsTab` does not. This creates bugs where stale data persists after certain operations.

**Suggested approach**: Create a shared `queries/invalidation.ts` module with composed functions like `invalidateAfterSale(queryClient, campaignId)`, `invalidateAfterImport(queryClient)`, and `invalidateAll(queryClient)`. Replace all inline invalidation calls.

---

## #4: InventoryTab.tsx exceeds 500-line limit with internal duplication

**Category**: Maintainability | **Severity**: Medium | **Effort**: Medium (1–4hr)
**Location**: `web/src/react/pages/campaign-detail/InventoryTab.tsx` (506 lines)

This file exceeds the project's 500-line guideline. It contains ~60 lines of near-identical code between print-path and virtual-path rendering for both desktop rows (lines 388–411 vs 414–451) and mobile cards (lines 319–334 vs 336–364). Additionally, mobile and desktop stat displays (lines 89–181) duplicate the same values/formatting in different layouts.

**Suggested approach**: Extract dialog renderings to an `InventoryDialogs` component, deduplicate print/virtual row rendering with a shared helper, and consider a `StatSummary` component for the mobile/desktop stats.

---

## #5: ~170 source files (92%) have no corresponding test

**Category**: Tests | **Severity**: Medium | **Effort**: Large (4hr+)
**Location**: `web/src/` (broad)

Of ~185 testable source files, only ~15 have corresponding tests (8% file-level coverage). Critical untested pure-logic files include: `inventoryCalcs.ts` (inventory math), `priceDecisionHelpers.ts` (price source selection), `priceCardUtils.ts` (price calculations), `useCampaignDerived.ts` (derived campaign logic), `api/client.ts` (HTTP client with retry), and `shopifyCSVParser.ts` (CSV parsing). These are all highly testable pure functions.

**Suggested approach**: Prioritize tests for pure business logic files first (no React rendering needed): `inventoryCalcs.ts`, `priceDecisionHelpers.ts`, `priceCardUtils.ts`, `shopifyCSVParser.ts`, `marketplaceUrls.ts`. Then add tests for the core hooks and API client.

---

## #6: PokeballLoader inaccessible to screen readers; no skip-to-content link

**Category**: UX | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: `web/src/react/PokeballLoader.tsx:22`, `web/src/react/App.tsx`

The primary loading indicator (`PokeballLoader`) lacks `role="status"` and `aria-live="polite"`, making loading states invisible to screen readers. Used on every page. Additionally, there is no skip-to-content link despite `<main id="main-content">` being ready for it. Keyboard users must Tab through the entire header/nav on every page load.

**Suggested approach**: Add `role="status"` and `aria-live="polite"` with a visually-hidden "Loading" text to `PokeballLoader`. Add a skip-to-content `<a>` as the first child in `App.tsx` that links to `#main-content` with CSS `sr-only focus:not-sr-only`.

---

## #7: Custom dialogs lack focus trapping

**Category**: UX | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: `web/src/react/PriceOverrideDialog.tsx`, `web/src/react/PriceHintDialog.tsx`

Two dialogs are implemented as custom overlay divs instead of using Radix UI Dialog (which the project already depends on). They have `role="dialog"` and Escape handling, but no focus trapping — users can Tab into background content while the dialog is open. All other dialogs in the project (`ConfirmDialog`, `RecordSaleModal`, `PriceLookupDrawer`) correctly use Radix Dialog.

**Suggested approach**: Migrate both to Radix `Dialog.Root`/`Dialog.Content`, matching the pattern in `ConfirmDialog.tsx`. This adds focus trapping, proper portal rendering, and consistent animation.

---

## #8: Error handling uses two competing patterns

**Category**: Duplication | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: `web/src/react/pages/tools/CardIntakeTab.tsx`, `pages/ShopifySyncPage.tsx`, `pages/tools/EbayExportTab.tsx`, plus 5 other files

A `getErrorMessage(err, fallback)` utility exists in `utils/formatters.ts` and is used in 7 files. However, 13 other call sites across 8 files use the raw pattern `err instanceof Error ? err.message : 'fallback'` instead. This creates inconsistency and fragility (e.g., the raw pattern doesn't handle API error objects).

**Suggested approach**: Replace all 13 raw `instanceof Error` patterns with `getErrorMessage()` from formatters.

---

## #9: API client has internal method duplication

**Category**: Duplication | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `web/src/js/api/client.ts`

`post()` (lines 227–241) and `put()` (lines 246–260) are identical except for the HTTP method string. `deleteResource()` partially duplicates `_expectNoContent` logic. `uploadFile()` implements its own timeout/error handling instead of reusing `fetchWithRetry`, creating an inconsistent resilience model.

**Suggested approach**: Extract a shared `_jsonRequest(method, endpoint, data, options)` helper for post/put. Route `uploadFile` through `fetchWithRetry` or document why it differs.

---

## #10: Test file organization is inconsistent

**Category**: Quality | **Severity**: Low | **Effort**: Small (< 1hr)
**Location**: `web/src/` and `web/tests/`

Tests are split between colocated files in `web/src/` (9 test files) and a separate `web/tests/` directory (6 test files) with no discernible pattern. For example, `useDebounce.test.ts` is in `tests/hooks/` but `useForm.test.ts` is colocated. `Button.test.tsx` is in `tests/ui/` but `CardShell.test.tsx` is colocated.

**Suggested approach**: Pick one convention (colocation is the React community standard) and consolidate. Move the 6 files from `web/tests/` to colocated positions next to their source files, or document the split convention if intentional.

---
---

# Backend (Go) — Improvement Issues

Review date: 2026-04-07
Scope: `internal/`, `cmd/`

## Coverage Metrics

| Metric | Value |
|--------|-------|
| Go source LOC (excl tests/mocks) | 51,710 |
| Test LOC | 41,717 |
| Test-to-code ratio | 0.77 |
| Architecture violations | 0 |
| Files over 500 LOC | 1 (`cmd/slabledger/main.go` at 556) |
| Packages with zero tests | 4 (all interface-only: intelligence, storage, observability, testutil) |

---

## #1: Campaign error-to-HTTP-status mapping duplicated 37+ times

**Category**: Duplication | **Severity**: High | **Effort**: Medium (1–4hr)
**Location**: `internal/adapters/httpserver/handlers/` (all campaign handler files)

Every campaign handler manually checks `IsCampaignNotFound` -> 404, `IsValidationError` -> 400, `IsPurchaseNotFound` -> 404, else log+500. This identical if-else cascade is copy-pasted 37+ times across handler files. Adding a new domain error type (e.g., 403 for authorization) requires editing dozens of handlers.

**Suggested approach**: Extract a `handleCampaignError(w http.ResponseWriter, logger Logger, msg string, err error)` helper that maps domain error types to HTTP status codes in one place. All handlers call it instead of inline cascades.

---

## #2: `docs/API.md` is missing ~30 endpoints

**Category**: Docs | **Severity**: High | **Effort**: Medium (1–4hr)
**Location**: `docs/API.md`

Entire route groups registered in `routes.go` are absent from `docs/API.md`: DH integration (9 endpoints), picks (5), cert scanning (2), order imports (2), opportunities (2), CardLadder admin (3), social metrics (2), image proxy, buy cost PATCH, selected sell sheet POST, and DH admin config endpoints.

**Suggested approach**: Cross-reference `internal/adapters/httpserver/routes.go` with `docs/API.md` systematically. Document each missing endpoint with request/response shapes by reading the corresponding handler.

---

## #3: Business logic embedded in `finance_repository.go` storage adapter

**Category**: Architecture | **Severity**: High | **Effort**: Medium (1–4hr)
**Location**: `internal/adapters/storage/sqlite/finance_repository.go:126-212`

`GetCapitalSummary` computes `weeksToCover`, `trend` (improving/declining/stable), and `alertLevel` (ok/warning/critical) using domain constants like `WeeksToCoverCriticalThreshold` and `TrendChangeThreshold`. The analytics repository similarly computes ROI, SellThroughPct, and TotalUnsold in Go code after SQL queries. This violates the hexagonal architecture rule that adapters must not contain business logic.

**Suggested approach**: Move derived metric calculations (trend, alertLevel, ROI, sell-through) into domain functions. The repository returns raw data; the domain service enriches it.

---

## #4: Double computation of `OverrideTotalUsd` in both repo and service layers

**Category**: Duplication | **Severity**: High | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/storage/sqlite/purchases_repository_pricing.go:121-122` and `internal/domain/campaigns/service_pricing.go:66-67`

The same `float64(cents) / 100` conversion for `OverrideTotalUsd` and `SuggestionTotalUsd` happens in both the repository layer and the service layer. The service silently overwrites the repository's value. If the formula ever diverges between layers, one will silently override the other — a latent bug.

**Suggested approach**: Remove the repository-layer computation; let the domain service be the single source of truth for USD conversion.

---

## #5: 845 lines of critical campaign analytics with zero tests

**Category**: Tests | **Severity**: High | **Effort**: Large (4hr+)
**Location**: `internal/domain/campaigns/tuning_analytics.go` (365 lines), `internal/domain/campaigns/service_analytics.go` (key functions)

Pure functions like `computePriceTierPerformance`, `computeMarketAlignment`, `computeBuyThresholdAnalysis`, `snapshotFromPurchase`, `applyCLSignal` (70/30 price blending with floor logic), and `enrichAgingItem` (market signal computation, price anomaly detection) power the main dashboard but have zero direct tests. The CL blending math and market signal computation are especially bug-prone without coverage.

**Suggested approach**: Start with `tuning_analytics.go` — all pure functions, perfect for table-driven tests with zero mocking needed. Then add focused tests for `applyCLSignal` and `enrichAgingItem`.

---

## #6: Repeated eBay fee fallback pattern in 6 locations

**Category**: Duplication | **Severity**: High | **Effort**: Small (< 1hr)
**Location**: `internal/domain/campaigns/channel_fees.go:18`, `service_sell_sheet.go:81`, `suggestion_rules.go:191`, `service_arbitrage.go:33`, `crack_arbitrage.go:33`, `acquisition_arbitrage.go:39`

The 3-line pattern `feePct := campaign.EbayFeePct; if feePct == 0 { feePct = DefaultMarketplaceFeePct }` is duplicated across 6 files. If default fee logic changes or per-channel defaults are introduced, every copy must be found and updated.

**Suggested approach**: Add a method `func (c *Campaign) EffectiveEbayFeePct() float64` that encapsulates the fallback. Replace all 6 call sites with a single method call.

---

## #7: Inconsistent `sql.ErrNoRows` check style — 8 files use `==` instead of `errors.Is`

**Category**: Quality | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: `internal/adapters/storage/sqlite/` — `api_tracker.go`, `advisor_cache.go`, `cardladder_store.go`, `social_repository.go`, `cl_sales_store.go`, `instagram_store.go` (8 total)

Eight storage files use `err == sql.ErrNoRows` while 15+ files correctly use `errors.Is(err, sql.ErrNoRows)`. Direct equality will break silently if errors are ever wrapped by middleware or instrumentation.

**Suggested approach**: Find-and-replace all `err == sql.ErrNoRows` with `errors.Is(err, sql.ErrNoRows)`. Mechanical fix.

---

## #8: Significant dead code — unused exports, constants, and interface methods

**Category**: Dead Code | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: Multiple packages

High-confidence dead code includes: 3 truly dead exported functions (`ExtractCardNumberFromPSATitle` in `import_parsing.go:31`, `ValidateVerdictAdjustment` in `scoring/safety.go:45`, `GetTestToken` in `testutil/config.go:26`), 7 unused cache constants (entire `constants/cache.go` is dead), 2 unused interface methods (`RecordCardAccess` on `AccessTracker`, `UpdateRateLimit` on `APITracker` in `pricing/repository.go`), 1 unused type (`ResettingCounter` in `platform/resilience/counter.go` with full implementation and tests but no consumers), 2 unused error functions (`NewErrUserNotFound`, `IsUserNotFound` in `auth/errors.go`), and 2 unused scheduler defaults (`DefaultAccessLogCleanupConfig`, `DefaultSessionCleanupConfig`). Additionally, 15 exported functions in `cardutil/normalize_sets.go` are only used within their own package and should be unexported.

**Suggested approach**: Delete the truly dead symbols. Unexport the `cardutil` functions that are package-internal. Remove unused interface methods and their implementations.

---

## #9: `cmd/slabledger/main.go` exceeds 500-line limit at 556 lines

**Category**: Maintainability | **Severity**: Medium | **Effort**: Small (< 1hr)
**Location**: `cmd/slabledger/main.go:1-556`

The `runServer()` function spans 376 lines — a god-function that creates 30+ dependencies, wires them together, and handles shutdown. The file already has partial splits (`init.go`, `server.go`) but the core wiring remains monolithic. This file is currently over the project's 500-line limit enforced by `scripts/check-file-size.sh`.

**Suggested approach**: Extract `buildServerDependencies(...)` into `server.go` and `gracefulShutdown(...)` into a new `shutdown.go`, reducing `main.go` below 500 lines.

---

## #10: 501 lines of campaign suggestion algorithms with zero tests

**Category**: Tests | **Severity**: Medium | **Effort**: Medium (1–4hr)
**Location**: `internal/domain/campaigns/suggestion_rules.go` (230 lines), `internal/domain/campaigns/suggestion_rules_optimization.go` (271 lines)

These files contain complex branching logic for generating campaign optimization suggestions (fee structures, sell-through analysis, price tier distributions, market alignment recommendations). Despite being critical business logic that directly influences user decisions, neither file has any test coverage.

**Suggested approach**: Add table-driven tests covering key suggestion scenarios: campaigns with different fee structures, various sell-through rates, price tier distributions, and edge cases like empty campaigns or campaigns with zero sales.

---

## Honorable Mentions

- **Inconsistent error wrapping** in `purchases_repository_pricing.go` — `UpdateReviewedPrice` wraps all errors but 6 adjacent functions doing the same pattern use bare `return err`
- **`http.Error` vs `writeError` inconsistency** — `social_proxy.go` (6 uses) and `pricing_diagnostics.go` (1 use) return plain text errors while 300+ other handler calls use JSON `writeError`
- **Campaigns package at 58 files / 11K lines** — spans ~20 concerns (CRUD, imports, exports, analytics, portfolio, arbitrage, suggestions, Monte Carlo). Candidate for splitting into sub-packages
- **Migration count stale** in `CLAUDE.md` and `docs/SCHEMA.md` — both say 43 migration pairs but there are actually 44 (missing `000044_dh_push_safety`)
- **`SCHEMA.md` missing table** — `dh_push_config` table and `campaign_purchases.dh_hold_reason` column from migration 000044 are undocumented
- **Duplicate `writeJSON`/`writeError` implementations** — `pricing_api.go` defines `writePricingJSON`/`writePricingError` identical to the shared helpers in `helpers.go`, plus `apikey.go` middleware has a third copy
- **40+ `ExecContext` + `RowsAffected` boilerplate blocks** in SQLite adapter — could be consolidated with an `execExpectOne(ctx, db, notFoundErr, query, args...)` helper
- **Duplicate inline mocks** — campaigns package has 787-line `mockRepo` in `mock_repo_test.go` parallel to the shared `mocks.MockCampaignRepository`
- **`centsToDollars` duplication** — `mathutil.ToDollars(int64)`, handler-local `centsToDollars(int)`, and 15+ inline `float64(cents)/100` conversions. The `int64` vs `int` signature mismatch discourages use of the shared function
