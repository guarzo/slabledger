# Maintainability Round 2: Remaining Improvements

**Date**: 2026-03-26
**Status**: Approved
**Branch**: guarzo/refactor (builds on round 1 work)

## Goal

Address remaining maintainability issues found during post-refactor review: documentation gaps, error pattern inconsistency, httpx migration for 3 more clients, and 3 file splits.

## Cross-Cutting: Remove Redundant Comments

During every code task (D2–D6), remove comments that merely restate what the code does. Keep only comments that explain *why* something is non-obvious or warn about gotchas.

**Remove** (restates the code):
```go
// CreatePurchase creates a new purchase.
func (s *service) CreatePurchase(...) { ... }
```

**Keep** (explains why):
```go
// Chinese sets: mapChineseNumber translates PSA printed numbers to
// PC species-based numbers (CBB1=700+n, CBB2=600+n).
```

Apply this judgment to every file touched in D2–D6. Do not go hunting through untouched files.

## D1. Documentation fixes (CLAUDE.md)

### D1a. Fix migration count

CLAUDE.md says "19 pairs, 000001-000019" in two places (lines 86 and 228). Actual count is **21 pairs** (42 files, 000001-000021).

### D1b. Document `make check` and guardrail scripts

Add to Quick Commands section:
```markdown
make check                                 # Full quality check (lint + architecture + file size)
```

Add a new section after Resilience Patterns:
```markdown
## Quality Checks

- `make check` — runs lint + architecture import check + file size check
- `scripts/check-imports.sh` — fails if domain packages import adapter packages (hexagonal invariant)
- `scripts/check-file-size.sh` — warns at 500 lines, fails at 600 lines (excludes test files and mocks)
```

### D1c. Replace env vars section with pointer

Replace the `## Environment Variables` section (currently listing ~28 vars) with:
```markdown
## Environment Variables

See `.env.example` for the complete list with descriptions. Key groups:

- **Required**: `PRICECHARTING_TOKEN`
- **AI**: `AZURE_AI_ENDPOINT`, `AZURE_AI_API_KEY`, `AZURE_AI_DEPLOYMENT`
- **Auth**: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `ENCRYPTION_KEY`
- **Schedulers**: `PRICE_REFRESH_ENABLED`, `ADVISOR_REFRESH_HOUR`, `SOCIAL_CONTENT_HOUR`

Full reference: [.env.example](.env.example)
```

## D2. Standardize `social/errors.go`

Current (non-standard):
```go
var ErrPostNotFound = fmt.Errorf("post not found")
```

Target (matches all other domain packages):
```go
const ErrCodePostNotFound errors.ErrorCode = "ERR_POST_NOT_FOUND"
var ErrPostNotFound = errors.NewAppError(ErrCodePostNotFound, "post not found")
```

Apply to all 3 errors: `ErrPostNotFound`, `ErrNotConfigured`, `ErrNotPublishable`. Add predicate functions. Verify callers use `errors.Is()`.

## D3. httpx migrations

Migrate 3 clients from raw `*http.Client` to `*httpx.Client` for retry + circuit breaker. Same pattern as the `image_client.go` migration in round 1.

### D3a. `internal/adapters/clients/psa/client.go`
- GET only, Bearer auth, 15s timeout
- Replace `http.NewRequestWithContext` + `Do` with `httpx.Client.Get`

### D3b. `internal/adapters/clients/instagram/client.go`
- GET + POST, query param auth, 30s timeout
- Replace `Do` calls with `httpx.Client.Get`/`Post`
- Polling-based waiting (not streaming) — safe to migrate

### D3c. `internal/adapters/clients/google/oauth.go`
- GET + POST, Bearer + form auth, 10s timeout
- Custom OAuth implementation (not golang.org/x/oauth2) — safe to migrate

Note: `azureai/client.go` intentionally uses raw `*http.Client` for SSE streaming — do not migrate.

## D4. Split `purchases_repository_queries.go` (582L) → 2 files

| File | Contents | ~Lines |
|------|----------|--------|
| `purchases_repository_queries.go` | Lookups: `GetPurchaseByCertNumber`, `GetPurchasesByGraderAndCertNumbers`, `GetPurchasesByCertNumbers`, `GetPurchasesByIDs`. Field updates: `UpdateExternalPurchaseFields`, `UpdatePurchaseMarketSnapshot`, `ListSnapshotPurchasesByStatus`, `UpdatePurchaseSnapshotStatus`, `UpdatePurchasePSAFields`, `ListPurchasesMissingImages`, `UpdatePurchaseImageURLs` | ~360 |
| `purchases_repository_pricing.go` | Price overrides: `UpdatePurchasePriceOverride`, `UpdateReviewedPrice`, `UpdatePurchaseAISuggestion`, `GetPriceOverrideStats`, `ClearPurchaseAISuggestion`, `AcceptAISuggestion`. eBay export: `SetEbayExportFlag`, `ClearEbayExportFlags`, `ListEbayFlaggedPurchases` | ~220 |

## D5. Split `suggestion_rules.go` (552L) → 2 files

| File | Contents | ~Lines |
|------|----------|--------|
| `suggestion_rules.go` | Expansion/coverage rules: `suggestTopCharacterExpansion` (rule 1), `suggestGradeSweetSpot` (rule 2), `suggestCoverageGapCampaigns` (rule 3), `suggestChannelInformedBuyTerms` (rule 4) | ~270 |
| `suggestion_rules_optimization.go` | Optimization/lifecycle rules: `suggestSpendCapRebalancing` (rule 5), `suggestCharacterAdjustments` (rule 6), `suggestPhaseTransitions` (rule 7) + any shared helper functions used only by these rules | ~280 |

## D6. Split `service_import_psa.go` (517L) → 2 files

| File | Contents | ~Lines |
|------|----------|--------|
| `service_import_psa.go` | Main import orchestration: `ImportPSAExportGlobal`, `collectAllocatedCerts`, `handleExistingPSAPurchase`, `handleNewPSAPurchase`, `autoDetectInvoices` | ~350 |
| `service_import_psa_enrich.go` | Background cert enrichment: `batchResolveCardIDs`, `certEnrichWorker`, `enrichSingleCert` | ~170 |
