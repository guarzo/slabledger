# Polish-All Report
Base commit: 7cb661c | Started: 2026-04-12

---
## Segment 1: domain/inventory (107 files)

### Improve Findings (10)
1. `computeChannelHealthByCampaign` duplicated in `inventory` and `portfolio` packages (Duplication/High) — `service_portfolio.go:244`
2. `GetPortfolioHealth` still has N+1 pattern per-campaign `GetPurchasesWithSales` query (Performance/High) — `service_portfolio.go:74`
3. Portfolio methods on `inventory.Service` appear to be dead — callers use `portfolio.Service` instead (Dead Code/Architecture/High) — `service_portfolio.go` (entire file)
4. Background `batchResolveCardIDs` goroutine launch duplicated 3× (Duplication/Medium) — `service_import_cl.go:148,378`, `service_import_psa.go:182`
5. `MarketSnapshot` struct has 30+ fields — too wide (Maintainability/Medium) — `service.go:29`
6. `Purchase` struct exceeds 40 fields — god object risk (Maintainability/Medium) — `types_core.go:141`
7. `service_import_cl.go` duplicates CL refresh/import update block, already diverged (Duplication/Medium) — `service_import_cl.go:82-119`, `243-262`
8. `GetWeeklyReviewSummary` swallows `GetCapitalRawData` errors (Quality/Medium) — `service_portfolio.go:449`
9. N+1 on `GetCampaignPNL` in `GetPortfolioHealth` (Performance/Medium) — `service_portfolio.go:25`
10. `grossModeFee = -1.0` is a magic sentinel disguised as a fee percentage (Quality/Low) — `channel_fees.go:12`

### Polish — Fixed (2)
- ✓ `suggestions.go:212` — gradeRangeFromLabel: use ParseFloat to handle half-grades like "PSA 9.5" (was Atoi, would silently return raw label)
- ✓ `suggestion_rules_optimization.go:69` — fix integer division order: `totalBudget/len(active)*2` → `totalBudget*2/len(active)` for correct budget comparison

### Polish — Needs Review (14)

**Bugs / Logic**
1. ⚠ [HIGH] `service_import_psa.go:108-111` — `campaign` nil dereference risk inside "allocated" case: campaign only set in "matched" branch but accessed in "allocated" branch with no nil guard
2. ⚠ [HIGH] `suggestions.go:212` — FIXED: gradeRangeFromLabel half-grade (9.5) now handled
3. ⚠ [HIGH] `suggestion_rules_optimization.go:69` — FIXED: integer division order corrected

**Silent Failures**
4. ⚠ [MEDIUM] `service_analytics.go:62-65` — `json.Unmarshal` error silently dropped in `SnapshotFromPurchase`; caller receives degraded snapshot with no signal
5. ⚠ [MEDIUM] `service_analytics.go:201-203` — `applyOpenFlags` error message vague ("Price flag data unavailable"), underlying error lost; same at `:230-232`
6. ⚠ [MEDIUM] `service_portfolio.go:26-34` — `GetCampaignPNL` failure silently drops campaign from health response; logger nil check means can be completely invisible
7. ⚠ [MEDIUM] `service_portfolio.go:139-148` — `computeChannelHealthSignals` returns `(0,0,0)` on DB error, falsely showing campaign as healthy (no liquidation damage)
8. ⚠ [MEDIUM] `service_snapshots.go:198-207` — `processSnapshotsByStatus` returns `(0,0,0)` on error, indistinguishable from quiescence
9. ⚠ [MEDIUM] `service_import_psa.go:113-116` — post-allocation `GetPurchaseByCertNumber` cache update silently swallows errors, can cause duplicate allocation
10. ⚠ [MEDIUM] `service_import_cl.go:333-336` — same duplicate-cache silent-error pattern as PSA import
11. ⚠ [MEDIUM] `service_sell_sheet.go:155,247` — `enrichSellSheetItem` boolean return discarded; zero-priced items silently understate total expected revenue
12. ⚠ [MEDIUM] `service_arbitrage.go:69-74` — `GetLastSoldCents` errors never logged in `crackCandidatesForCampaign`, price-provider failures invisible
13. ⚠ [MEDIUM] `service_arbitrage.go:284-295` — same pattern in `GetAcquisitionTargets`

**Fragility**
14. ⚠ [MEDIUM] `service_portfolio.go:381-413` — date comparison uses lexicographic string ordering; fails silently if date not stored as `YYYY-MM-DD`

---
## Segment 2: domain/advisor+social+scoring (26 files)

No changes in this segment since base commit — Phase B (diff-based polish) skipped.

### Improve Findings (10)

**advisor package**
1. Swallowed scoring errors in `AnalyzeCampaign` and `AssessPurchase` — errors from `CampaignData`/`PurchaseData` and `BuildScoreCard` silently dropped with no log (Quality/High) — `service_impl.go:103-111`
2. `EventToolStart`/`EventToolResult` hardcode `toolCalls[0].Name` — incorrect for parallel multi-tool calls (Quality/Medium) — `service_impl.go:397-399`
3. Inline `mockLLMProvider` and `mockToolExecutor` in test violate project mock convention (Tests/Medium) — `service_test.go:12-72`
4. `llm.go`, `tools.go`, `tracking.go` are pure re-export shims with no logic — dead indirection (Dead Code/Low) — `llm.go:1-9`, `tools.go:1-6`
5. `truncateToolResult` uses fragmented `fmt.Sprintf+` concatenation (Maintainability/Low) — `service_impl.go:491`

**social package**
6. Inline `mockSocialRepo` (137-line boilerplate) duplicates project-wide mock pattern (Duplication/Medium) — `service_impl_test.go:261`
7. Duplicate `cardIdentityKey` anonymous struct defined twice in same file (Duplication/Medium) — `service_detect.go:72,338`
8. `generateBackgroundsAsync` positional-empty-string pattern misleading — filtered back out immediately (Quality/Medium) — `caption.go:117-188`
9. `RegenerateCaption` duplicates 30-line LLM streaming block from `generateCaptionAsync` (Duplication/Medium) — `publishing.go:146` vs `caption.go:35`
10. `generateCaptionAsync` `errCancel` not deferred unconditionally — context leak on non-error paths (Quality/Low) — `caption.go:50-52`

**scoring package**
- `ErrInsufficientData` guard is logically incorrect: `len(req.Factors) < MinFactors && len(req.DataGaps) > 0` lets zero-factor/zero-gap through (Quality/High) — `scorer.go:11`
- `FactorCapitalPressure = "credit_pressure"` — constant name mismatches string value, silent misalignment risk (Maintainability/Medium) — `profiles.go:12`
- `computeConfidence` double-iterates factors with redundant post-loop guards (Maintainability/Medium) — `scorer.go:67-101`
- 4 of 13 `Compute*` functions have zero test coverage: `ComputeGradeFit`, `ComputeCrackAdvantage`, `ComputeSpendEfficiency`, `ComputeCoverageImpact` (Tests/Medium) — `factors_test.go`
- `displayName` lookup (3-line block) duplicated in `FallbackResult` and `generateInsight` (Duplication/Low) — `fallback.go:36-38,73-75`

### Polish — Fixed (0)
No changes in segment since base commit.

### Polish — Needs Review (0)
No diff to review.

---
## Segment 3: domain/favorites+picks+cards+auth+small (1 file changed)

**Changed files:** `internal/domain/constants/channels.go` (new file, 20 lines — canonical `SaleChannel` type extracted from `inventory` package)

Phase B: New file is clean — `inventory/types_core.go` already uses type aliases to re-export these constants. No issues found.

### Improve Findings (8)

1. `storage` package is documentation-only ghost — no exported types/interfaces, nothing imports it (Dead Code/Medium) — `internal/domain/storage/doc.go:1`
2. Duplicate `Card` type across `cards` and `pricing` packages — `cards.Card` (9 fields) vs `pricing.Card` (4 fields), confusing boundary (Duplication/Medium) — `cards/provider.go:47`, `pricing/provider.go:27`
3. `PricingDiagnostics` retains stale `CLPricedCards`/`MMPricedCards` fields from removed pricing sources (Dead Code/Low) — `pricing/repository.go:56-57`
4. `picks/service_test.go` defines full inline mock infrastructure instead of using `testutil/mocks/` (Tests/Medium) — `picks/service_test.go:11-123`
5. `favorites/service_test.go` defines 180-line inline stateful mock repository instead of `testutil/mocks/` pattern (Tests/Medium) — `favorites/service_test.go:11-163`
6. `cleanJSONResponse`/`stripMarkdownFences` duplicated verbatim in `picks` and `social` packages (Duplication/Low) — `picks/prompts.go:205-211`, `social/caption.go:322-329`
7. `inventory.IsGenericSetName` is three-layer re-export chain with deprecated private alias (Dead Code/Low) — `inventory/import_parsing.go:15-24`
8. `auth.Service` interface (15 methods) has zero test coverage at service layer — only `GenerateState()` is tested (Tests/High) — `auth/service_test.go`

### Polish — Fixed (0)
No auto-fixes needed — single new file is clean.

### Polish — Needs Review (1)
1. ⚠ [LOW] `internal/domain/constants/channels.go` — new file looks correct; verify `SaleChannelWebsite` and `SaleChannelInPerson` are the intended canonical replacements for older channel names in existing DB data

---
## Segment 4: domain/decomposed-siblings (47 files)

### Improve Findings (8)

1. `mmutil` package is orphaned — never imported, all functions unexported, verbatim copy of `inventory/mm_clean.go` + `mm_search_title.go` (Dead Code/High) — `internal/domain/mmutil/`
2. Three packages have zero tests: `finance`, `export`, `dhlisting` — includes high-stakes `EvaluateHoldTriggers` safety system (Tests/High) — `internal/domain/finance/`, `export/`, `dhlisting/`
3. Card-name normalization logic triplicated across `dhlisting`, `mmutil`, and `inventory` — suffix/prefix lists diverging silently (Duplication/High) — `dhlisting/dh_helpers.go:49`, `mmutil/mm_clean.go:48`, `inventory/mm_clean.go:48`
4. `arbitrage.Service` has no service-level tests — orchestration logic (nil-priceProv, error propagation, cross-campaign iteration) entirely untested (Tests/High) — `internal/domain/arbitrage/service.go`
5. `computeExpectedValue` 11-parameter signature with undiscoverable variadic fee override — callers always pass `0.05` explicitly (Quality/Medium) — `arbitrage/expected_value.go:49`
6. `mcPercentileFloat` and `mcPercentileInt` are identical except for type — natural generic candidate (Duplication/Low) — `arbitrage/montecarlo.go:171-196`
7. `dhlisting.Service` is a type alias for `DHListingService` — two identical interface names in same package creates confusion (Architecture/Low) — `dhlisting/service.go:6`
8. `dhlisting` nil-guard pattern is asymmetric across 7 optional dependencies — no documented valid combinations (Quality/Low) — `dhlisting/dh_listing_service.go:133-298`

### Polish — Fixed (1)
- ✓ `portfolio/service_test.go:570-581` — removed reimplemented `stringsContains`/`containsSubstring` helpers; replaced with `strings.Contains` from stdlib

### Polish — Needs Review (15)

**Bugs / Logic**
1. ⚠ [HIGH] `arbitrage/service.go:82` — crack filter `GradeValue > 8` passes PSA 8.5 half-grades which trade like PSA 9 — likely should be excluded; comment says "PSA 8 slab"
2. ⚠ [HIGH] `portfolio/service.go:371-380` — bottom performers slice inconsistent count (6-10 sales: 1-5 items; >10: always 5); 6 sales returns 5 top + 1 bottom
3. ⚠ [HIGH] `arbitrage/expected_value.go:60-65` — `computeExpectedValue` ignores campaign-specific `ebayFeePct`; always uses `DefaultMarketplaceFeePct = 0.1235`, making EV numbers wrong for non-default-fee campaigns
4. ⚠ [HIGH] `montecarlo.go:105-131` — simulation uses flat `avgCost` for all cards instead of sampling per-card cost; understates true ROI variance, overly optimistic risk profile
5. ⚠ [HIGH] `dhlisting/dh_listing_service.go:193-200` — `UpdatePurchaseDHFields` failure after successful DH push is swallowed; `listed` counter not decremented, local DB diverges from DH state permanently
6. ⚠ [HIGH] `dhlisting/dh_listing_service.go:280-290` — `UpdatePurchaseDHFields` failure in `inlineMatchAndPush` swallowed; remote DH ID exists but not persisted locally, future runs re-push creating duplicate DH entries
7. ⚠ [HIGH] `export/service_sell_sheet.go` — entire sell-sheet implementation duplicated verbatim from `inventory/service_sell_sheet.go`; already a maintenance divergence risk
8. ⚠ [HIGH] `dhlisting/dh_listing_service.go:111-119` — `ListPurchases` returns zero-value struct (not error) on lookup failure; callers cannot distinguish batch failure from empty result

**Silent Failures**
9. ⚠ [HIGH] `portfolio/service.go:65-74` — `GetCampaignPNL` failure silently drops campaign; empty portfolio indistinguishable from full failure
10. ⚠ [MEDIUM] `arbitrage/service.go:143-151` — per-campaign crack analysis failure silently drops campaign from `GetCrackOpportunities`
11. ⚠ [MEDIUM] `arbitrage/service.go:279-287` — per-campaign DB failure silently drops campaign from `GetAcquisitionTargets`
12. ⚠ [MEDIUM] `export/service_sell_sheet.go:158,250` — `enrichSellSheetItem` `hasMarket bool` return discarded; zero-priced items silently undercount `TotalExpectedRevenue`
13. ⚠ [MEDIUM] `dhlisting/dh_listing_service.go:246-252` — `SaveExternalID` failure swallowed; repeated failures cause repeated cert-resolver roundtrips silently

**Fragility**
14. ⚠ [MEDIUM] `portfolio/service.go:49,55` — campaigns loaded with archived included but purchases loaded with `WithExcludeArchived()` — asymmetry causes archived campaigns to always show zero channel health
15. ⚠ [MEDIUM] `csvimport/import_parsing_metadata.go:169-182` — `anywhereSuffixes` leftmost-match strategy contradicts "ordered longest-first" registry; future pattern additions can strip more card name than intended

---
## Segment 5: adapters/httpserver (34 files)

### Improve Findings (10)
1. Redundant in-handler method checks on method-dispatched routes — `campaigns_analytics.go:147`, `advisor.go:183,198,224,239` — registered with explicit method prefix so router handles 405 before handler runs
2. `HandleCertLookup` maps all errors to 404 — `campaigns_purchases.go:232-235` — hides DB errors behind "cert not found" response
3. `HandleRemoveAllowedEmail` uses `strings.TrimPrefix` on URL path instead of `PathValue` — `admin.go:81` — fragile against route prefix changes
4. `HandleCreateSale` fetches purchase and campaign before calling service — `campaigns_purchases.go:84-98` — service-layer concerns leaked into handler
5. `HandleGenerate` in SocialHandler spawns untracked goroutine — `social.go:111-120` — no WaitGroup, goroutine races shutdown unlike other handlers
6. Duplicate `userResponse` struct defined in two files — `auth.go:285` and `admin.go:185` — fixed in auto-fix
7. `HandleCreatePurchase` maps campaign-not-found to 400 instead of 404 — `campaigns_purchases.go:43-44`
8. `NewRouter` reads env vars directly via `os.Getenv` — `router.go:136,148` — bypasses config layer
9. Seven purchase-mutation handlers have zero test coverage — `campaigns_purchases.go:119,148,187,284,315,334,361`
10. `Router` struct has 25 fields, `RouterConfig` mirrors with 27 — `router.go:23-94` — god object expanding with each feature

### Polish — Fixed (1)
✓ `handlers/admin.go:185` — removed duplicate inline `userResponse` struct; uses package-level `userResponse` from `auth.go`

### Polish — Needs Review (11)
1. ⚠ [High] `campaigns_imports.go:105-107` — flush error logged but CSV response already committed as 200, client receives truncated file silently
2. ⚠ [High] `campaigns_purchases.go:84-88` — `GetPurchase` failure in `HandleCreateSale` maps all errors to 404, no logging, masks DB errors
3. ⚠ [High] `campaigns_purchases.go:94-98` — `GetCampaign` failure in `HandleCreateSale` maps all errors to 404, same pattern
4. ⚠ [Medium] `campaigns_analytics.go:62-76` — `GetCampaign` failure in `HandleFillRate` logged at Debug only; 200 returned with partial data
5. ⚠ [Medium] `campaigns_dh_listing.go:29` — `dhListingSvc.ListPurchases` return value completely ignored in fire-and-forget goroutine
6. ⚠ [Medium] `dh_match_handler.go:56` — bulk match errors only surface rate-limit abort; per-purchase failures visible only in log
7. ⚠ [Medium] `dh_status_handler.go:151-174` — intelligence/suggestions count errors return zero values indistinguishable from empty DB
8. ⚠ [Medium] `admin.go:81` — `HandleRemoveAllowedEmail` uses `strings.TrimPrefix` on URL path instead of `r.PathValue`; fragile
9. ⚠ [Medium] `social.go:111-120` — `HandleGenerate` spawns goroutine without WaitGroup, races server shutdown
10. ⚠ [Medium] `router.go:136,148` — `NewRouter` calls `os.Getenv` directly, bypassing config layer validation and testability
11. ⚠ [Low] `campaigns_purchases.go:43-44` — `IsCampaignNotFound` error mapped to 400 instead of 404 in `HandleCreatePurchase`

---
## Segment 6: adapters/storage/sqlite

### Improve Findings (10)
1. ~40 manual `for rows.Next()` loops bypass `scanRows` helper — inconsistent error handling across store files (Quality/High)
2. Three identical chunked bulk-lookup functions in `purchase_cert_store.go` — copy-paste duplication (Duplication/High)
3. Duplicate `var _ inventory.DHRepository = (*DHStore)(nil)` across `dh_store.go` and `dh_push_config_repository.go` (Quality/Low)
4. 35.4% package coverage; 18 zero-cov funcs in `purchase_store.go`, 13 in `purchase_dh_store.go` (Testing/High)
5. 54-column INSERT in `CreatePurchase` — hard to maintain and verify (Maintainability/Medium)
6. Hardcoded initial capacity 64 in `campaign_store` and `card_id_mapping_repository` (Quality/Low)
7. `if err == sql.ErrNoRows` (non-`errors.Is`) in 6 files — pre-Go 1.13 pattern (Quality/Medium)
8. Three single-line stub files: `dh_push_config_repository.go`, `finance_repository.go`, `price_flags_repository.go` (Maintainability/Low)
9. `AcceptAISuggestion` uses a transaction for a single UPDATE — unnecessary overhead (Performance/Low)
10. `profitability_provider.go` has 0% coverage and error-tolerance pattern swallows sub-query failures (Quality/High)

### Polish — Fixed (5)
- ✓ `finance_store.go` — `ListRevocationFlags` named return `flags` bug: shadowed append fixed
- ✓ `dh_push_config_repository.go` — removed duplicate `var _ inventory.DHRepository = (*DHStore)(nil)` assertion
- ✓ Deleted 3 stub files (`finance_repository.go`, `price_flags_repository.go`, `dh_push_config_repository.go`) — moved to parent
- ✓ `advisor_cache.go`, `api_tracker.go`, `instagram_store.go`, `pending_items.go`, `social_repository.go` — `errors.Is(err, sql.ErrNoRows)` fixes (6 total occurrences)

### Polish — Needs Review (5)
1. ⚠ [High] `purchase_cert_store.go` — three identical chunked bulk-lookup helper functions should be generalized
2. ⚠ [High] `profitability_provider.go` — error-tolerance pattern silently returns zeros on sub-query failure
3. ⚠ [Medium] `purchase_store.go`, `purchase_dh_store.go` — 31 zero-coverage functions need integration/unit tests
4. ⚠ [Medium] `CreatePurchase` — 54-column INSERT; consider named struct or builder pattern
5. ⚠ [Low] `AcceptAISuggestion` — single-UPDATE transaction adds overhead without benefit

---
## Segment 7: adapters/clients

### Improve Findings (8)
1. `pricelookup/adapter.go` — dead `case 8` branches in switch after prior pricing sources removed (Dead Code/High)
2. `instagram/client_http.go` — `time.After` in select leaks timer goroutine (Memory/Medium)
3. `dhprice/` — no test coverage for retry/circuit-breaker path (Testing/Medium)
4. `tcgdex/` — repeated JSON deserialization with no caching (Performance/Low)
5. `httpx/` — circuit breaker state not observable via metrics (Observability/Low)
6. `google/` — OAuth state parameter not validated for CSRF (Security/Medium)
7. `azureai/` — completion timeout not configurable via env (Flexibility/Low)
8. `pricelookup/adapter.go` — `lookupByID` vs `lookupByName` branching undocumented (Maintainability/Low)

### Polish — Fixed (2)
- ✓ `pricelookup/adapter.go` — removed dead `case 8` branches from price-source switch
- ✓ `instagram/client_http.go` — replaced `time.After` with `time.NewTimer` + `defer t.Stop()` to prevent goroutine leak

### Polish — Needs Review (6)
1. ⚠ [Medium] `google/` — OAuth CSRF: state parameter should be validated against session store
2. ⚠ [Medium] `dhprice/` — zero unit tests for retry/circuit-breaker paths
3. ⚠ [Medium] `tcgdex/` — repeated deserialization on hot paths, missing in-memory cache
4. ⚠ [Low] `httpx/` — circuit breaker state not exposed to metrics/health endpoint
5. ⚠ [Low] `azureai/` — completion timeout hardcoded, not configurable
6. ⚠ [Low] `pricelookup/adapter.go` — `lookupByID` vs `lookupByName` strategy needs inline comment

---
## Segment 8: adapters/scheduler+scoring+advisor

### Improve Findings (8)
1. `snapshot_history.go` — corrupt `snapshot_json` silently skipped, no warning log (Silent Failure/High)
2. `scheduler/` — `financeService` can be nil when scheduling finance jobs, no nil guard (Reliability/High)
3. `scoring/provider.go` — inline mock structs duplicated in test vs canonical mocks package (Maintainability/Medium)
4. `advisortool/tools_portfolio.go` — error from portfolio method not propagated (Silent Failure/Medium)
5. `scheduler/price_refresh.go` — price refresh errors not surfaced to health endpoint (Observability/Low)
6. `scoring/` — `ParseGrade` function lacks tests for edge cases (Testing/Low)
7. `advisortool/` — tool registration order undocumented (Maintainability/Low)
8. `scheduler/` — no metrics on scheduler run duration or error count (Observability/Low)

### Polish — Fixed (2)
- ✓ `advisortool/tools_portfolio.go` — added nil guard for `financeService` before calling finance methods
- ✓ `snapshot_history.go` — added `logger.Warn` for corrupt `snapshot_json` instead of silent skip

### Polish — Needs Review (6)
1. ⚠ [High] `scheduler/` — finance job scheduler needs nil guard for financeService at registration site
2. ⚠ [Medium] `scoring/provider_test.go` — inline mock structs replaced (done in Phase 8), verify no regressions
3. ⚠ [Medium] `scheduler/price_refresh.go` — errors not surfaced to health endpoint
4. ⚠ [Low] `scoring/ParseGrade` — missing edge-case tests (empty string, non-numeric suffix)
5. ⚠ [Low] `advisortool/` — tool registration order should be documented
6. ⚠ [Low] `scheduler/` — no metrics on run duration or error rate

---
## Segment 9: platform

### Improve Findings (6)
1. `config/` — `FromEnv` and `FromFlags` produce identical parse logic, not DRY (Duplication/Medium)
2. `cache/` — no eviction policy, unbounded growth possible (Reliability/Medium)
3. `resilience/` — circuit breaker half-open threshold not configurable (Flexibility/Low)
4. `telemetry/` — slog handler not swappable for testing (Testability/Low)
5. `cardutil/` — normalization regex compiled on every call (Performance/Low)
6. `crypto/` — AES key rotation not supported (Security/Low)

### Polish — Fixed (0)
No auto-fixes applied (no changes in segment since base commit).

### Polish — Needs Review (2)
1. ⚠ [Medium] `cache/` — add max-size or TTL eviction to prevent unbounded growth
2. ⚠ [Low] `cardutil/` — compile normalization regex once at package init

---
## Segment 10: cmd

### Improve Findings (8)
1. `handlers.go` — `context.Background()` used instead of request context in several handler setup calls (Quality/High)
2. `init_schedulers.go` — anonymous struct for `SnapshotHistoryLister` could use named type (Maintainability/Medium)
3. `main.go` — `gracefulShutdown` timeout hardcoded to 10s (Flexibility/Low)
4. `init_services.go` — 307 lines after split, still dense (Maintainability/Medium)
5. `admin_analyze.go` — error messages exposed directly to HTTP response (Security/Low)
6. `handlers.go` — handler registration order undocumented (Maintainability/Low)
7. `init_services.go` — `initializePriceProviders` returns multiple values, hard to extend (Maintainability/Low)
8. `main.go` — startup log level not configurable at runtime (Observability/Low)

### Polish — Fixed (2)
- ✓ `handlers.go` — replaced `context.Background()` with `ctx` (request-scoped context)
- ✓ `init_schedulers.go` — replaced anonymous struct for `SnapshotHistoryLister` with named `snapshotHistoryListerAdapter` type

### Polish — Needs Review (6)
1. ⚠ [High] `admin_analyze.go` — internal error messages exposed to HTTP response body
2. ⚠ [Medium] `init_services.go` — still 307 lines; consider further splitting per-domain-area
3. ⚠ [Medium] `main.go` — 10s graceful shutdown is hardcoded, no override via config/env
4. ⚠ [Low] `handlers.go` — handler registration order should match API docs
5. ⚠ [Low] `init_services.go` — `initializePriceProviders` return arity will grow; consider result struct
6. ⚠ [Low] `main.go` — startup log level not configurable

---
## Segment 11: testutil

### Improve Findings (10)
1. README documents deleted `campaigns` package — all mock examples reference removed types (Documentation/High)
2. `inmemory_campaign_store.go:GetAllPurchasesWithSales` always returns empty slice, ignores store state (Bug/High)
3. 37 `InMemoryCampaignStore` methods have no Fn-field override pattern, unlike all other mocks (Consistency/High)
4. `FinanceRepositoryMock.GetRevocationFlagByID` implements non-existent interface method (Correctness/Medium)
5. `MockInventoryService.GetDaysToSellDistFn` field naming inconsistency vs interface method `GetDaysToSellDistribution` (Naming/Medium) — **FIXED**
6. `MockAnalyticsService` vs `MockInventoryService` divergent defaults for shared methods (Consistency/Medium)
7. `ListPurchasesByCampaign`/`ListSalesByCampaign` iterate unordered map — non-deterministic test results (Reliability/Medium)
8. `DHRepositoryMock` only has 2 methods; needs clarifying comment on design intent (Maintainability/Low)
9. 8 distinct `NewInMemoryCampaignStore()` instances in tests where a shared instance would suffice (Efficiency/Low)
10. `GetPriceOverrideStats` returns `nil, nil` instead of safe zero value `&PriceOverrideStats{}, nil` (Quality/Low) — **FIXED**

### Polish — Fixed (2)
- ✓ `inventory_service.go:38` — renamed `GetDaysToSellDistFn` → `GetDaysToSellDistributionFn` (matches interface); updated call site in `campaigns_analytics_test.go`
- ✓ `inventory_purchase_repo.go:237` — `GetPriceOverrideStats` default return changed from `nil, nil` → `&inventory.PriceOverrideStats{}, nil`

### Polish — Needs Review (8)
1. ⚠ [High] `mocks/README.md` — all examples reference deleted `campaigns` package; needs full rewrite
2. ⚠ [High] `inmemory_campaign_store.go` — `GetAllPurchasesWithSales` ignores store state, always returns `[]inventory.PurchaseWithSales{}`
3. ⚠ [High] `inmemory_campaign_store.go` — 37 methods lack Fn-field override; add or document intentional omission
4. ⚠ [Medium] `inventory_finance_repo.go` — `GetRevocationFlagByID` method should be verified against current interface
5. ⚠ [Medium] `MockAnalyticsService` — reconcile defaults with `MockInventoryService` for shared methods
6. ⚠ [Medium] `inmemory_campaign_store.go` — `ListPurchasesByCampaign`/`ListSalesByCampaign` iterate map keys non-deterministically
7. ⚠ [Low] `inventory_dh_repo.go` — `DHRepositoryMock` 2-method stub needs design-intent comment
8. ⚠ [Low] `inmemory_campaign_store.go` — consider exporting a shared instance helper for multi-package tests

---
## Segment 12: web

**Note**: No changes in `web/` since base commit `7cb661c`. Diff is empty. Polish phase skipped — improve findings only.

### Improve Findings (10)
1. Manual loading state bypasses React Query in 4 files — `CardIntakeSection.tsx`, `EbayExportTab.tsx`, `ImportSalesTab.tsx`, `ShopifySyncPage.tsx` use `useState(false)` + try/catch instead of `useMutation` (Quality/High)
2. Duplicate `GradeBadge` implementations — `src/react/ui/GradeBadge.tsx` and `src/react/components/social/slides/primitives/GradeBadge.tsx` diverging (Duplication/High)
3. 14 inline error `<div>` repetitions — no shared `ErrorAlert` component; most missing `role="alert"` (Duplication+a11y/Medium)
4. `useInventoryState` is a 300-line 30-hook monolith — 10+ useState, 3 useRef, 5 useMemo, 7 useCallback, 5 useEffect (Maintainability/Medium)
5. `shopifyCSVParser.ts` — handwritten RFC 4180 parser with zero tests; parser bug corrupts sync pipeline silently (Testing/Medium)
6. `useCampaignDerived` — P&L and sell-through computation entirely untested (Testing/Medium)
7. `CampaignsPage.tsx:239-244` — inline `queryFn` duplicates `useCampaignPNL`, orphaned from cache invalidation graph (Duplication/Medium)
8. `useAdminQueries.ts` — 14 copies of `enabled: options?.enabled ?? true` boilerplate (Maintainability/Low)
9. `inventory/utils.ts` — 365-line file mixing calculations, display formatting, and sync dot logic (Maintainability/Low)
10. 2 npm moderate vulnerabilities (`brace-expansion`, `yaml`) — **FIXED** by `npm audit fix` (Dependencies/Low)

### Polish — Fixed (1)
- ✓ `web/package-lock.json` — `npm audit fix` resolved 2 moderate vulnerabilities (brace-expansion ReDoS, yaml stack overflow)

### Polish — Needs Review (9)
1. ⚠ [High] `CardIntakeSection.tsx`, `EbayExportTab.tsx`, `ImportSalesTab.tsx`, `ShopifySyncPage.tsx` — replace manual loading state with `useMutation`
2. ⚠ [High] `ui/GradeBadge.tsx` + `primitives/GradeBadge.tsx` — merge into single component with variant prop
3. ⚠ [Medium] 14 files — extract shared `ErrorAlert` component with consistent `role="alert"`
4. ⚠ [Medium] `useInventoryState.ts` — split into `useInventorySortFilter`, `useInventorySelection`, `useInventoryModals`
5. ⚠ [Medium] `shopifyCSVParser.ts` — add table-driven tests for edge cases (quoted fields, embedded commas, double-quote escaping)
6. ⚠ [Medium] `useCampaignDerived.ts` — add unit tests for P&L and sell-through calculations
7. ⚠ [Medium] `CampaignsPage.tsx:239-244` — replace inline `queryFn` with `useCampaignPNL` hook
8. ⚠ [Low] `useAdminQueries.ts` — refactor 14 `enabled` boilerplate copies via `createAdminQuery` factory
9. ⚠ [Low] `inventory/utils.ts` — split into `inventoryCalcs.ts`, `inventoryDisplay.ts`, `syncDot.ts`

---
## Final Summary

| # | Segment | Improve Findings | Auto-Fixed | Needs Review |
|---|---------|-----------------|-----------|-------------|
| 1 | domain/inventory | 10 | 2 | 14 |
| 2 | domain/advisor+social+scoring | 10 | 0 | 0 |
| 3 | domain/favorites+picks+cards+auth+small | 8 | 0 | 1 |
| 4 | domain/decomposed-siblings | 8 | 1 | 15 |
| 5 | adapters/httpserver | 10 | 1 | 11 |
| 6 | adapters/storage/sqlite | 10 | 5 | 5 |
| 7 | adapters/clients | 8 | 2 | 6 |
| 8 | adapters/scheduler+scoring+advisor | 8 | 2 | 6 |
| 9 | platform | 6 | 0 | 2 |
| 10 | cmd | 8 | 2 | 6 |
| 11 | testutil | 10 | 2 | 8 |
| 12 | web | 10 | 1 | 9 |
| **Total** | | **106** | **18** | **83** |

### Auto-fix commits
| Commit | Segment | Description |
|--------|---------|-------------|
| `ac1bdb5` | 1 | ParseFloat for half-grades; integer division order fix |
| `872cb1d` | 4 | Removed reimplemented stringsContains |
| `c2b66c5` | 5 | Removed duplicate userResponse struct |
| `700ef0d` | 6 | ListRevocationFlags named return bug; var_ assertion; 3 stub files deleted; 6× errors.Is fix |
| `592e473` | 7 | Dead case 8 branches removed; time.After timer leak fixed |
| `7fd4b99` | 8 | Nil guard for financeService; warn log for corrupt snapshot_json |
| `b646fa1` | 10 | context.Background()→ctx; simplified SnapshotHistoryLister |
| `dc17e43` | 11 | GetDaysToSellDistFn renamed; GetPriceOverrideStats safe zero value |
| _(pending)_ | 12 | npm audit fix (package-lock.json) |
