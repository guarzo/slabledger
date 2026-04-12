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
