# Polish Report — Residual Items

**Date:** 2026-04-12  
**Source:** `docs/polish-report.md` (base commit `7cb661c`)  
**Scope:** Items from improve findings NOT covered by P1–P9 plans and NOT already fixed.

These items require separate design or are deferred due to architectural scope.

---

## Category A: Structural / Architectural (Require Separate Design)

These were explicitly called out as out-of-scope in the parallel plans design spec.

| Severity | Location | Item |
|----------|----------|------|
| HIGH | `internal/domain/inventory/types_core.go:141` | `Purchase` struct 40+ fields — god object; reducing requires API + DB migration |
| MEDIUM | `internal/domain/inventory/service.go:29` | `MarketSnapshot` struct 30+ fields — too wide; same concern as Purchase |
| LOW | `internal/adapters/httpserver/router.go:23-94` | `Router` 25-field + `RouterConfig` 27-field god object — decomposition requires handler architecture redesign |
| HIGH | `internal/domain/auth/service_test.go` | `auth.Service` interface (15 methods) has zero test coverage — requires dedicated auth testing plan |
| HIGH | `internal/domain/mmutil/` | Orphaned package — never imported, all functions unexported, verbatim copy of `inventory/mm_clean.go`; delete after confirming zero runtime usage |
| HIGH | `internal/domain/inventory/service_portfolio.go` (entire file) | Portfolio methods on `inventory.Service` appear dead — callers use `portfolio.Service`; remove after confirming dead |

---

## Category B: Improve Findings Not Promoted to Needs Review

These appeared in segment improve findings but had no diff to review and were not assigned to a plan.

### Segment 2 — domain/advisor+social+scoring (10 improve findings, 0 needs-review)

All 10 items were in improve findings only. Items 1, 2, 3, 4, 6, 7, 8, 9, 10 were assigned to P3. The remaining item below was NOT assigned to any plan:

| Severity | Location | Item |
|----------|----------|------|
| MEDIUM | `internal/domain/scoring/profiles.go:12` | `FactorCapitalPressure = "credit_pressure"` — constant name mismatches string value; silent misalignment risk |
| MEDIUM | `internal/domain/scoring/factors_test.go` | 4 of 13 `Compute*` functions have zero test coverage: `ComputeGradeFit`, `ComputeCrackAdvantage`, `ComputeSpendEfficiency`, `ComputeCoverageImpact` |
| LOW | `internal/domain/scoring/fallback.go:36-38,73-75` | `displayName` 3-line lookup block duplicated in `FallbackResult` and `generateInsight` |
| LOW | `internal/domain/advisor/service_impl.go:491` | `truncateToolResult` uses `fmt.Sprintf` concatenation — minor style issue |

### Segment 3 — domain/favorites+picks+cards+auth+small (8 improve findings, 1 needs-review)

Items 4, 5, 6, 7, 8, 9 assigned to P4. Items below were NOT assigned to any plan:

| Severity | Location | Item |
|----------|----------|------|
| MEDIUM | `internal/domain/cards/provider.go:47`, `internal/domain/pricing/provider.go:27` | Duplicate `Card` type across `cards` and `pricing` packages — confusing boundary |
| LOW | `internal/domain/picks/prompts.go:205-211`, `internal/domain/social/caption.go:322-329` | `cleanJSONResponse`/`stripMarkdownFences` duplicated verbatim in `picks` and `social` packages |
| LOW | `internal/domain/inventory/import_parsing.go:15-24` | `inventory.IsGenericSetName` is a three-layer re-export chain with deprecated private alias |

### Segment 4 — domain/decomposed-siblings (8 improve findings, 15 needs-review)

Improve findings not directly tracked as needs-review items and not in P2:

| Severity | Location | Item |
|----------|----------|------|
| HIGH | `internal/domain/finance/`, `export/`, `dhlisting/` | Zero tests in three packages — includes high-stakes `EvaluateHoldTriggers` safety system |
| HIGH | `internal/domain/arbitrage/service.go` | `arbitrage.Service` has no service-level tests — orchestration logic (nil-priceProv, error propagation, cross-campaign iteration) entirely untested |
| MEDIUM | `internal/domain/arbitrage/expected_value.go:49` | `computeExpectedValue` 11-parameter signature with undiscoverable variadic fee override |
| LOW | `internal/domain/arbitrage/montecarlo.go:171-196` | `mcPercentileFloat` and `mcPercentileInt` identical except for type — natural generic candidate |
| LOW | `internal/domain/dhlisting/service.go:6` | `dhlisting.Service` is a type alias for `DHListingService` — two identical interface names in same package |
| LOW | `internal/domain/dhlisting/dh_listing_service.go:133-298` | nil-guard pattern asymmetric across 7 optional dependencies — no documented valid combinations |

### Segment 5 — adapters/httpserver (10 improve findings, 11 needs-review)

Improve findings not in P5:

| Severity | Location | Item |
|----------|----------|------|
| HIGH | `internal/adapters/httpserver/` | Redundant in-handler method checks on method-dispatched routes — `campaigns_analytics.go:147`, `advisor.go:183,198,224,239` — router handles 405 before handler runs |
| MEDIUM | `internal/adapters/httpserver/campaigns_purchases.go:232-235` | `HandleCertLookup` maps all errors to 404 — hides DB errors |
| MEDIUM | `internal/adapters/httpserver/router.go:136,148` | `NewRouter` reads env vars directly via `os.Getenv` — bypasses config layer (tracked in P5 item 10, but also a broader design issue) |
| HIGH | `internal/adapters/httpserver/campaigns_purchases.go` | 7 purchase-mutation handlers with zero test coverage |

### Segment 8 — adapters/scheduler+scoring+advisor

| Severity | Location | Item |
|----------|----------|------|
| MEDIUM | `internal/adapters/scoring/provider_test.go` | P8 item 2: verify inline mocks were fully replaced — this is now done (Phase 8 commit `7fc1ec4`), mark as resolved |

### Segment 10 — cmd

| Severity | Location | Item |
|----------|----------|------|
| LOW | `cmd/slabledger/init_services.go` | `initializePriceProviders` returns multiple values — consider result struct (P8 item 5 covers this but is LOW priority) |

### Segment 11 — testutil

| Severity | Location | Item |
|----------|----------|------|
| MEDIUM | `internal/testutil/mocks/` | `MockAnalyticsService` vs `MockInventoryService` divergent defaults for shared methods (P4 item 5 in design, but listed as "reconcile defaults" which is scope-unclear) |

---

## Summary

| Category | Count |
|----------|-------|
| A: Structural/architectural (require design) | 6 |
| B: Improve findings not in any plan | ~22 |

**Action required:** Category A items need individual design docs before implementation. Category B items should be triaged — the HIGH-severity ones (zero tests in finance/export/dhlisting, arbitrage service tests) are strong candidates for P-class follow-up plans.
