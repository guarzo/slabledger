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
