# Design: Polish Report — 9 Parallel Implementation Plans

**Date:** 2026-04-12  
**Source:** `docs/polish-report.md` (base commit `7cb661c`, 83 "needs review" items across 12 segments)  
**Goal:** Split all 83 items into independent, parallel-executable implementation plans with no cross-plan file conflicts.

---

## Decisions

- **Priority:** Correctness and safety first (HIGH severity items lead each plan), then MEDIUM, then LOW.
- **Grouping:** By architectural layer — each plan is scoped to a specific set of directories, minimizing cross-plan file conflicts.
- **Backend/Frontend split:** Go plans (P1–P8) are independent from the TypeScript/React plan (P9).
- **Parallelism:** All 9 plans can run simultaneously in separate git worktrees. No two plans touch the same files.

---

## Plan Inventory

| Plan | Name | Files | Items | High | Medium | Low |
|------|------|-------|-------|------|--------|-----|
| P1 | domain/inventory | `internal/domain/inventory/` | 14 | 1 | 11 | 2 |
| P2 | domain/decomposed-siblings | `internal/domain/arbitrage/`, `portfolio/`, `tuning/`, `finance/`, `export/`, `dhlisting/`, `csvimport/`, `mmutil/` | 15 | 9 | 6 | 0 |
| P3 | domain/advisor+social+scoring | `internal/domain/advisor/`, `social/`, `scoring/` | 10 | 2 | 6 | 2 |
| P4 | domain/small+testutil | `internal/domain/favorites/`, `picks/`, `cards/`, `auth/`, `pricing/`, `constants/`, `internal/testutil/` | 9 | 3 | 4 | 2 |
| P5 | adapters/httpserver | `internal/adapters/httpserver/` | 10 | 3 | 6 | 1 |
| P6 | adapters/storage/sqlite | `internal/adapters/storage/sqlite/` | 5 | 2 | 2 | 1 |
| P7 | adapters/clients+scheduler | `internal/adapters/clients/`, `internal/adapters/scheduler/`, `internal/adapters/advisortool/` | 12 | 1 | 5 | 6 |
| P8 | platform+cmd | `internal/platform/`, `cmd/` | 8 | 1 | 3 | 4 |
| P9 | web (TypeScript/React) | `web/src/` | 9 | 2 | 5 | 2 |
| **Total** | | | **83** | **25** | **48** | **20** |

---

## Detailed Item Lists

### P1 — domain/inventory

**Scope:** `internal/domain/inventory/`  
**Run in worktree:** `.worktrees/plan-p1-domain-inventory`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `service_import_psa.go:108-111` | nil dereference: `campaign` accessed in "allocated" branch without nil guard |
| 2 | MEDIUM | `service_analytics.go:62-65` | `json.Unmarshal` error silently dropped in `SnapshotFromPurchase`; caller receives degraded snapshot |
| 3 | MEDIUM | `service_analytics.go:201-203,230-232` | `applyOpenFlags` loses underlying error; vague "Price flag data unavailable" message |
| 4 | MEDIUM | `service_portfolio.go:26-34` | `GetCampaignPNL` failure silently drops campaign from health response |
| 5 | MEDIUM | `service_portfolio.go:139-148` | `computeChannelHealthSignals` returns `(0,0,0)` on DB error — false healthy signal |
| 6 | MEDIUM | `service_snapshots.go:198-207` | `processSnapshotsByStatus` returns `(0,0,0)` on error, indistinguishable from quiescence |
| 7 | MEDIUM | `service_import_psa.go:113-116` | post-allocation cache update swallows errors — duplicate allocation risk |
| 8 | MEDIUM | `service_import_cl.go:333-336` | same duplicate-cache silent-error pattern as PSA import |
| 9 | MEDIUM | `service_sell_sheet.go:155,247` | `enrichSellSheetItem` bool return discarded; zero-priced items understate revenue |
| 10 | MEDIUM | `service_arbitrage.go:69-74` | price-provider errors never logged in `crackCandidatesForCampaign` |
| 11 | MEDIUM | `service_arbitrage.go:284-295` | same pattern in `GetAcquisitionTargets` |
| 12 | MEDIUM | `service_portfolio.go:381-413` | lexicographic date comparison fragility (fails if not `YYYY-MM-DD`) |
| 13 | LOW | `channel_fees.go:12` | `grossModeFee = -1.0` magic sentinel → replace with named constant `GrossModeFeeDisabled` or similar |
| 14 | LOW | `service_import_cl.go:82-119,243-262` | duplicated CL refresh/import block — document divergence or extract shared helper |

**Constraints:**
- Items 10 and 11 overlap with P2's arbitrage items. P1 handles the `inventory/service_arbitrage.go` file; P2 handles `internal/domain/arbitrage/service.go`. These are different files.
- Do not touch `internal/domain/arbitrage/` (P2 scope).

---

### P2 — domain/decomposed-siblings

**Scope:** `internal/domain/arbitrage/`, `portfolio/`, `tuning/`, `finance/`, `export/`, `dhlisting/`, `csvimport/`, `mmutil/`  
**Run in worktree:** `.worktrees/plan-p2-domain-siblings`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `dhlisting/dh_listing_service.go:193-200` | `UpdatePurchaseDHFields` failure after DH push swallowed; listed counter not decremented → DB diverges from DH |
| 2 | HIGH | `dhlisting/dh_listing_service.go:280-290` | same in `inlineMatchAndPush` — remote DH ID not persisted, future runs create duplicate DH entries |
| 3 | HIGH | `dhlisting/dh_listing_service.go:111-119` | `ListPurchases` returns zero-value struct on lookup failure (not error) — callers can't distinguish failure from empty |
| 4 | HIGH | `portfolio/service.go:65-74` | `GetCampaignPNL` failure silently drops campaign from health response |
| 5 | HIGH | `export/service_sell_sheet.go` | verbatim duplicate of `inventory/service_sell_sheet.go` — `export` package should delegate to `inventory` package's sell-sheet logic or the duplicated functions should be removed from `export/` |
| 6 | HIGH | `arbitrage/service.go:82` | `GradeValue > 8` passes PSA 8.5 half-grades (they trade like PSA 9) — fix filter |
| 7 | HIGH | `portfolio/service.go:371-380` | bottom-performers slice has inconsistent count logic (6-10 sales: variable; >10: always 5) |
| 8 | HIGH | `arbitrage/expected_value.go:60-65` | EV ignores campaign-specific `ebayFeePct`; hardcodes `DefaultMarketplaceFeePct = 0.1235` |
| 9 | HIGH | `arbitrage/montecarlo.go:105-131` | simulation uses flat `avgCost` for all cards instead of per-card cost sampling |
| 10 | MEDIUM | `arbitrage/service.go:143-151` | per-campaign crack failure silently drops campaign from `GetCrackOpportunities` |
| 11 | MEDIUM | `arbitrage/service.go:279-287` | per-campaign DB failure silently drops campaign from `GetAcquisitionTargets` |
| 12 | MEDIUM | `export/service_sell_sheet.go:158,250` | `enrichSellSheetItem` bool return discarded |
| 13 | MEDIUM | `dhlisting/dh_listing_service.go:246-252` | `SaveExternalID` failure swallowed — repeated failures cause repeated cert-resolver roundtrips |
| 14 | MEDIUM | `portfolio/service.go:49,55` | archived campaigns loaded but purchases filtered with `WithExcludeArchived()` — asymmetry causes zero channel health for archived |
| 15 | MEDIUM | `csvimport/import_parsing_metadata.go:169-182` | leftmost-match strategy contradicts "ordered longest-first" registry intent |

**Constraints:**
- Item 5 (sell-sheet duplication): preferred resolution is to have `export/service_sell_sheet.go` delegate to the `inventory` package's implementation, or extract shared logic to a new file in `inventory/`. Do not change `inventory/service_sell_sheet.go` (P1 scope).
- Items 1-3 are data-integrity bugs — implement with tests that verify the error is now returned/logged.

---

### P3 — domain/advisor+social+scoring

**Scope:** `internal/domain/advisor/`, `internal/domain/social/`, `internal/domain/scoring/`  
**Run in worktree:** `.worktrees/plan-p3-domain-advisor`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `advisor/service_impl.go:103-111` | errors from `CampaignData`/`PurchaseData` and `BuildScoreCard` silently dropped in `AnalyzeCampaign`/`AssessPurchase` |
| 2 | HIGH | `scoring/scorer.go:11` | `ErrInsufficientData` guard is logically wrong: `&&` should be `\|\|` (zero-factor/zero-gap passes through) |
| 3 | MEDIUM | `advisor/service_impl.go:397-399` | hardcoded `toolCalls[0].Name` incorrect for parallel multi-tool calls |
| 4 | MEDIUM | `social/service_detect.go:72,338` | `cardIdentityKey` anonymous struct defined twice in same file — deduplicate |
| 5 | MEDIUM | `social/caption.go:50-52` | `errCancel` not deferred unconditionally — context leak on non-error paths |
| 6 | MEDIUM | `social/publishing.go:146` | `RegenerateCaption` duplicates 30-line LLM streaming block from `generateCaptionAsync` |
| 7 | MEDIUM | `scoring/scorer.go:67-101` | `computeConfidence` double-iterates factors with redundant post-loop guards |
| 8 | MEDIUM | `advisor/service_test.go:12-72` | inline `mockLLMProvider` and `mockToolExecutor` — replace with canonical mocks from `testutil/mocks/` |
| 9 | MEDIUM | `social/service_impl_test.go:261` | inline `mockSocialRepo` (137 lines) — replace with canonical mock |
| 10 | LOW | `advisor/llm.go`, `tools.go`, `tracking.go` | pure re-export shims with no logic — delete or consolidate |

**Constraints:**
- Item 8 and 9 require adding canonical mock types to `internal/testutil/mocks/` if they don't exist. Coordinate with P4 to avoid duplicate additions. P4 adds testutil mocks for `favorites` and `picks`; P3 adds for `advisor` and `social`.
- Item 2 (ErrInsufficientData): change `&&` to `||` and add a test that explicitly covers the zero-factor/zero-gap case.

---

### P4 — domain/small+testutil

**Scope:** `internal/domain/favorites/`, `picks/`, `cards/`, `auth/`, `pricing/`, `constants/`, `storage/`, `internal/testutil/`  
**Run in worktree:** `.worktrees/plan-p4-domain-small`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `testutil/mocks/README.md` | full rewrite — all examples reference deleted `campaigns` package; update to current architecture with working examples |
| 2 | HIGH | `testutil/inmemory_campaign_store.go` | `GetAllPurchasesWithSales` ignores store state, always returns `[]inventory.PurchaseWithSales{}` — fix to return actual data |
| 3 | HIGH | `testutil/inmemory_campaign_store.go` | 37 methods lack Fn-field override pattern — add overrides for each, or document deliberate omission with rationale |
| 4 | MEDIUM | `testutil/inventory_finance_repo.go` | verify `GetRevocationFlagByID` method matches current `FinanceRepository` interface; fix or remove |
| 5 | MEDIUM | `testutil/inmemory_campaign_store.go` | `ListPurchasesByCampaign`/`ListSalesByCampaign` iterate map keys non-deterministically — fix with sorted keys |
| 6 | MEDIUM | `domain/picks/service_test.go:11-123` | inline mock infrastructure (113 lines) → replace with canonical mocks |
| 7 | MEDIUM | `domain/favorites/service_test.go:11-163` | inline stateful mock repository (180 lines) → replace with canonical mock |
| 8 | LOW | `domain/storage/doc.go` | ghost package — no exports, nothing imports it; delete the file |
| 9 | LOW | `domain/pricing/repository.go:56-57` | stale `CLPricedCards`/`MMPricedCards` fields from removed pricing sources; delete |

**Constraints:**
- Item 3: if adding 37 Fn-field overrides is excessive, acceptable alternative is a `//nolint:override` style comment block documenting that InMemoryStore uses direct state mutation instead of Fn-fields, and why. Decision should be recorded in the README rewrite (item 1).
- When replacing inline mocks in items 6 and 7, check `testutil/mocks/` for existing `FavoritesRepositoryMock` / `PicksRepositoryMock` before creating new ones.

---

### P5 — adapters/httpserver

**Scope:** `internal/adapters/httpserver/`  
**Run in worktree:** `.worktrees/plan-p5-httpserver`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `campaigns_imports.go:105-107` | CSV flush error after 200 already committed — restructure to check all errors before writing response |
| 2 | HIGH | `campaigns_purchases.go:84-88` | `GetPurchase` failure in `HandleCreateSale` maps all errors to 404 with no logging — add error logging and use 500 for DB errors |
| 3 | HIGH | `campaigns_purchases.go:94-98` | `GetCampaign` failure in `HandleCreateSale` same pattern — same fix |
| 4 | MEDIUM | `campaigns_analytics.go:62-76` | `GetCampaign` failure logged at Debug only, returns partial 200 — upgrade to Error log, return 500 or 404 |
| 5 | MEDIUM | `campaigns_dh_listing.go:29` | `ListPurchases` return value ignored in fire-and-forget goroutine — log errors |
| 6 | MEDIUM | `dh_match_handler.go:56` | bulk match per-purchase failures only visible in log — surface summary error count in response |
| 7 | MEDIUM | `dh_status_handler.go:151-174` | zero counts indistinguishable from DB error — return partial error signal or distinct error field |
| 8 | MEDIUM | `admin.go:81` | `HandleRemoveAllowedEmail` uses `strings.TrimPrefix` on URL path — replace with `r.PathValue` |
| 9 | MEDIUM | `social.go:111-120` | `HandleGenerate` goroutine without WaitGroup races server shutdown — add proper lifecycle management |
| 10 | LOW | `campaigns_purchases.go:43-44` | `IsCampaignNotFound` mapped to 400 — change to 404 |

**Note:** `admin_analyze.go` error-message exposure belongs to P8 (the file is `cmd/slabledger/admin_analyze.go`, not in httpserver). This plan covers 10 items.

**Constraints:**
- Item 1: CSV response cannot be trivially restructured (streaming). Recommended fix is to buffer the CSV in memory first, only writing to response on success.
- Item 4: check all handlers in `admin_analyze.go` for the pattern; fix all occurrences.
- Item 10: the goroutine management pattern should match the existing shutdown pattern in other handlers (check for existing WaitGroup or context usage in the codebase).

---

### P6 — adapters/storage/sqlite

**Scope:** `internal/adapters/storage/sqlite/`  
**Run in worktree:** `.worktrees/plan-p6-sqlite`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `purchase_cert_store.go` | three identical chunked bulk-lookup helpers — generalize to a single typed helper function (use Go generics if appropriate) |
| 2 | HIGH | `profitability_provider.go` | error-tolerance pattern silently returns zeros on sub-query failure — return error or log prominently |
| 3 | MEDIUM | `purchase_store.go`, `purchase_dh_store.go` | 31 zero-coverage functions — add table-driven unit tests for the most critical paths |
| 4 | MEDIUM | `CreatePurchase` | 54-column INSERT — add positional grouping comments (not full builder; comments suffice for maintainability) |
| 5 | LOW | `AcceptAISuggestion` | single-UPDATE transaction wrapper adds overhead without benefit — remove transaction |

**Constraints:**
- Item 3: 31 functions is too many for complete coverage in one pass. Prioritize functions on the critical path: `GetPurchase`, `ListPurchasesByCampaign`, `GetPurchaseByCertNumber`, `UpdatePurchase`. Document remaining gaps.
- Item 1: prefer Go generics (`[T any]`) over code generation for the chunked bulk-lookup helper.

---

### P7 — adapters/clients+scheduler

**Scope:** `internal/adapters/clients/`, `internal/adapters/scheduler/`, `internal/adapters/advisortool/`  
**Run in worktree:** `.worktrees/plan-p7-clients-scheduler`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `scheduler/` | `financeService` nil guard at job registration site — add nil check before registering finance jobs |
| 2 | MEDIUM | `clients/google/` | OAuth CSRF: validate state parameter against session store on callback |
| 3 | MEDIUM | `scheduler/price_refresh.go` | price refresh errors not surfaced to health endpoint — record last-error timestamp |
| 4 | MEDIUM | `clients/dhprice/` | zero unit tests for retry/circuit-breaker paths — add tests using mock HTTP server |
| 5 | MEDIUM | `clients/tcgdex/` | repeated JSON deserialization on hot paths — add in-memory cache with TTL |
| 6 | MEDIUM | `adapters/advisortool/tools_portfolio.go` | verify nil guard fix was applied in prior session; add regression test |
| 7 | LOW | `clients/httpx/` | circuit breaker state not exposed to metrics/health endpoint |
| 8 | LOW | `clients/azureai/` | completion timeout hardcoded — add configurable env var |
| 9 | LOW | `clients/pricelookup/adapter.go` | add inline comment explaining `lookupByID` vs `lookupByName` strategy |
| 10 | LOW | `adapters/scoring/` | `ParseGrade` edge-case tests (empty string, non-numeric suffix) |
| 11 | LOW | `adapters/advisortool/` | document tool registration order in a comment block |
| 12 | LOW | `scheduler/` | add metrics for scheduler run duration and error count (use existing `observability.MetricsRecorder` interface) |

**Constraints:**
- Item 2 (OAuth CSRF): state validation requires access to session store. Check existing session infrastructure before implementing — do not add a new session dependency if one already exists.
- Item 5 (TCGDex cache): use the existing `platform/cache` package; do not introduce a new cache dependency.

---

### P8 — platform+cmd

**Scope:** `internal/platform/`, `cmd/`  
**Run in worktree:** `.worktrees/plan-p8-platform-cmd`

| # | Severity | File:Line | Item |
|---|----------|-----------|------|
| 1 | HIGH | `cmd/slabledger/admin_analyze.go` | internal error messages in HTTP response body — replace with generic client-facing messages, log details server-side |
| 2 | MEDIUM | `platform/cache/` | add max-size or TTL eviction policy to prevent unbounded growth |
| 3 | MEDIUM | `cmd/main.go` | graceful shutdown timeout hardcoded to 10s — add `SHUTDOWN_TIMEOUT_SECONDS` env var |
| 4 | MEDIUM | `cmd/init_services.go` | still 307 lines; split by domain area if natural boundaries exist |
| 5 | LOW | `platform/cardutil/` | compile normalization regex once at package init (not on every call) |
| 6 | LOW | `platform/config/` | `FromEnv`/`FromFlags` have duplicate parse logic — extract shared parse helper |
| 7 | LOW | `cmd/main.go` | startup log level not configurable at runtime |
| 8 | LOW | `cmd/handlers.go` | handler registration order should match `docs/API.md` order |

**Constraints:**
- Item 4: do not split if no natural boundary exists. 307 lines is borderline — acceptable if the file has a single cohesive responsibility.

---

### P9 — web (TypeScript/React)

**Scope:** `web/src/`  
**Run in worktree:** `.worktrees/plan-p9-web`

| # | Severity | File(s) | Item |
|---|----------|---------|------|
| 1 | HIGH | `CardIntakeSection.tsx`, `EbayExportTab.tsx`, `ImportSalesTab.tsx`, `ShopifySyncPage.tsx` | replace manual `useState(loading)` + try/catch with `useMutation` from React Query |
| 2 | HIGH | `react/ui/GradeBadge.tsx`, `react/components/social/slides/primitives/GradeBadge.tsx` | merge into single component; add `variant` prop for differentiation |
| 3 | MEDIUM | 14 files with inline error `<div>` | extract shared `ErrorAlert` component; add `role="alert"` for accessibility |
| 4 | MEDIUM | `useInventoryState.ts` | split 300-line/30-hook monolith into `useInventorySortFilter`, `useInventorySelection`, `useInventoryModals` |
| 5 | MEDIUM | `shopifyCSVParser.ts` | add table-driven tests for edge cases: quoted fields, embedded commas, double-quote escaping |
| 6 | MEDIUM | `useCampaignDerived.ts` | add unit tests for P&L and sell-through calculation logic |
| 7 | MEDIUM | `CampaignsPage.tsx:239-244` | replace inline `queryFn` with `useCampaignPNL` hook; integrate with cache invalidation graph |
| 8 | LOW | `useAdminQueries.ts` | refactor 14 `enabled: options?.enabled ?? true` copies via `createAdminQuery` factory function |
| 9 | LOW | `inventory/utils.ts` | split 365-line file into `inventoryCalcs.ts`, `inventoryDisplay.ts`, `syncDot.ts` |

**Constraints:**
- Item 1: use `useMutation` from `@tanstack/react-query` (already a project dependency). Match the existing mutation patterns in other components.
- Item 3: identify all 14 files before implementing `ErrorAlert`. Ensure the component API is consistent with existing UI component patterns in `react/ui/`.
- Item 4: splitting `useInventoryState` is a large refactor. Ensure each hook has a single clear responsibility and the split doesn't break dependent components. Add tests before splitting.
- Run `npm test` and `npm run build` after each change to catch regressions early.

---

## Execution Model

Each plan runs in an isolated git worktree. All 9 can run in parallel:

```
for plan in p1 p2 p3 p4 p5 p6 p7 p8 p9; do
  git worktree add .worktrees/plan-$plan -b feature/polish-$plan
done
```

Each worktree runs independently. After completion, merge each branch back to the feature branch in any order (no conflicts since file scopes are disjoint).

### Verification per plan

**Go plans (P1–P8):**
```bash
go build ./...
go test -race -timeout 10m ./...
make check
```

**Web plan (P9):**
```bash
npm test
npm run build
```

### Merge order (if sequential merges needed)

If parallel worktrees are not available, recommended order by risk:
1. P4 first (testutil fixes unblock P3 mock replacements)
2. P2 (data integrity, highest business risk)
3. P1 (domain/inventory silent failures)
4. P5 (httpserver)
5. P6, P7, P8 (lower risk)
6. P3 (advisor/social)
7. P9 (web, independent)

---

## Out of Scope

The following items from the improve findings are structural/architectural and excluded from these plans — they require separate design work:

- `Purchase` struct god-object (40+ fields) — requires API and DB migration
- `MarketSnapshot` 30+ fields — same
- `Router` 25-field god-object — requires handler decomposition design
- `auth.Service` zero test coverage (15-method interface) — requires dedicated auth testing plan
- `mmutil` orphaned package deletion — requires confirming no runtime usages first
- TCGDex caching (mentioned in P7 but scope-limited to not introducing new infra)
