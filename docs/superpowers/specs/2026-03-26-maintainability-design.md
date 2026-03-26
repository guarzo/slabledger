# Maintainability & AI-Agent Friendliness Improvements

**Date**: 2026-03-26
**Status**: Approved (revised after code review)
**Priority**: B (Structure) > A (Documentation) > C (Guardrails)

## Goal

Make the codebase more maintainable and easier for AI agents to work on by:
1. Decomposing large files into focused, single-responsibility units
2. Improving documentation discoverability for AI agents
3. Adding CI guardrails that catch common mistakes automatically

## Section B: Structural Decomposition

Six files need splitting. Each split follows existing package conventions — no new packages, no interface changes, no behavioral changes. Pure file reorganization.

### B1. `internal/adapters/advisortool/executor.go` (650L) -> 3 files

| New File | Contents | ~Lines |
|----------|----------|--------|
| `executor.go` | Core executor struct (`CampaignToolExecutor`), `Execute`, `Definitions`, `DefinitionsFor`, `register`, `registerCampaignTool`, `toJSON`, `truncateJSON`, `truncateSlice`, `truncateMapArrays`, `parseCampaignID`, `registerTools`, `emptyObjectParams`, `campaignIDParams`, `jsonSchema` type | ~230 |
| `tools_campaign.go` | 7 campaign tools: `registerListCampaigns`, `registerGetCampaignPNL`, `registerGetPNLByChannel`, `registerGetCampaignTuning`, `registerGetInventoryAging`, `registerGetGlobalInventory`, `registerGetSellSheet` | ~75 |
| `tools_portfolio.go` | 14 portfolio/analysis tools (`registerGetPortfolioHealth` through `registerGetSuggestionStats`) + `dashboardSummary` type + `registerGetDashboardSummary` | ~345 |

### B2. `internal/adapters/clients/fusionprice/fusion_provider.go` (635L) -> 2 files

Revised from 3-file to 2-file split: `attachSourceDetails` (66L) is too small for its own file and is called by `getPriceFromSources`, so it stays with the core pipeline.

| New File | Contents | ~Lines |
|----------|----------|--------|
| `fusion_provider.go` | Provider struct, `NewFusionProviderWithRepo`, `WithCardProvider`, `GetPrice`, `getPriceFromSources`, `attachSourceDetails`, `Available`, `Name`, `Close`, `GetStats`, `getPriceResult` type | ~410 |
| `card_resolver.go` | `LookupCard`, `applyPCData`, `cleanupStaleName`, DB supplementation calls | ~225 |

### B3. `internal/adapters/clients/azureai/client.go` (613L) -> 3 files

Note: `parseResponsesSSEStream` is already in a separate file (`responses.go`); this split only touches `client.go`.

| New File | Contents | ~Lines |
|----------|----------|--------|
| `client.go` | `Config`, `Option`, `WithLogger`, `Client` struct, `NewClient`, `StreamCompletion` (retry + poll fallback orchestration), `pollResponseFallback`, error types (`rateLimitError`, `capacityError`), `maxStreamRetries` | ~330 |
| `request.go` | `doStreamCompletion`, `buildRequest`, `buildURL`, `isAzureOpenAI`, `useResponsesAPI` | ~180 |
| `stream.go` | `parseSSEStream`, `isPermanentError`, `flattenToolCalls` | ~110 |

### B4. `internal/adapters/clients/pricecharting/domain_adapter.go` (637L) -> 3 files

| New File | Contents | ~Lines |
|----------|----------|--------|
| `domain_adapter.go` | Public interface (`LookupCard`, `GetStats`), `lookupCardInternal` orchestration, `toLookupPrice`, `toDomainProviderStats`, `logTrace`, `getStatsInternal` | ~275 |
| `lookup_strategies.go` | `tryCache`, `tryUPC`, `tryAPI`, `tryFuzzy`, `resolveExpectedNumber`, `extractSetHint`, `knownSetTokens` | ~290 |
| `enrichment.go` | `enrichMatch`, `applyConservativeExits`, `applyLastSoldByGrade`, `saleRecordsFromRecentSales` | ~75 |

### B5. `internal/domain/social/service_impl.go` (984L) -> 3 files

File grew from 773L to 984L since initial exploration (added `generateBackgroundsAsync` + `deduplicateByCardIdentity`). Split is more justified now.

| New File | Contents | ~Lines |
|----------|----------|--------|
| `service_impl.go` | `service` struct, `NewService`, `DetectAndGenerate`, `llmGenerate`, `ruleBasedGenerate`, `detectPostType`, `detectNewArrivals`, `filterPriceMovers`, `filterHotDeals`, `deduplicateByCardIdentity` | ~430 |
| `publishing.go` | `Publish`, `publishAsync`, `setPublishError`, `RegenerateCaption`, `ListPosts`, `GetPost`, `UpdateCaption`, `Delete` | ~215 |
| `caption.go` | `generateCaptionAsync`, `generateBackgroundsAsync`, `logError`, `safeGo`, `Wait`, `parseCaption`, `truncateCaption`, `captionResponse` type, `parseCaptionResponse`, `stripMarkdownFences`, `sanitizeLLMJSON`, `generateID` | ~340 |

### B6. `internal/domain/campaigns/service_analytics.go` (715L) -> 2 files

File grew from 609L to 715L (added `SetReviewedPrice`, `GetReviewStats`, `GetGlobalReviewStats`, price flag methods). Revised from 3-file to 2-file split: PNL section is only ~96L, too small alone. Tuning calls `snapshotFromPurchase` and `applyCLSignal`, so it stays with those helpers. Implementation will reorder tuning before sell sheet for a clean split point.

| New File | Contents | ~Lines |
|----------|----------|--------|
| `service_analytics.go` | PNL (`GetCampaignPNL`, `GetPNLByChannel`, `GetDailySpend`, `GetDaysToSellDistribution`), snapshot helpers (`hasAnyPriceData`, `snapshotFromPurchase`), aging (`enrichAgingItem`, `applyCLSignal`, `GetInventoryAging`, `GetGlobalInventoryAging`, `applyOpenFlags`), tuning (`GetCampaignTuning`) | ~380 |
| `service_sell_sheet.go` | `enrichSellSheetItem`, `recommendChannel`, `GenerateSellSheet`, `GenerateGlobalSellSheet`, `computeRecommendation`, `computeTargetPrice`, `MatchShopifyPrices`, `SetReviewedPrice`, `GetReviewStats`, `GetGlobalReviewStats`, `CreatePriceFlag`, `ListPriceFlags`, `ResolvePriceFlag`, `recommendedPrice` | ~335 |

### Scope boundaries

- **No campaigns package split** — 49 files are already well-factored internally; splitting the domain package cascades through every consumer
- **No main.go split** — 572 lines, reads well top-to-bottom, adapter types are small
- **No interface segregation on campaigns.Service** — 62-method interface is large but stable; splitting cascades through mocks, handlers, and tests

## Section A: Documentation Improvements

### A1. Add "Testing & Mocks" section to CLAUDE.md

Add after the "Code Style" section:

```markdown
## Testing

- **Pattern**: Table-driven tests with `[]struct` for all test cases
- **Mocks**: Import from `internal/testutil/mocks/` — never create inline mocks
  - Uses Fn-field pattern: override any method by setting `mock.CreateCampaignFn = func(...) { ... }`
  - Full guide: `internal/testutil/mocks/README.md`
- **Error assertions**: Use `errors.Is(err, campaigns.ErrCampaignNotFound)` with sentinel errors
- **Deterministic data**: Use fixed seeds for Monte Carlo, atomic counters for IDs
- **Race detection**: Always run `go test -race` before committing
```

### A2. Add error handling recipe to Common Recipes

```markdown
### Add a new domain error

1. Add error code in `internal/domain/<package>/errors.go`: `ErrCodeMyError errors.ErrorCode = "ERR_MY_ERROR"`
2. Add sentinel: `var ErrMyError = errors.NewAppError(ErrCodeMyError, "description")`
3. Add predicate: `func IsMyError(err error) bool { return errors.HasErrorCode(err, ErrCodeMyError) }`
4. Test with `errors.Is(err, ErrMyError)` in callers
```

### A3. Fix ARCHITECTURE.md Go version

Change "Go 1.25.2" to "Go 1.26" on line 7.

### A4. Add "Key Reference Files" section to CLAUDE.md

```markdown
## Key Reference Files

- `internal/README.md` — Architecture rules, decision tree for code placement, anti-patterns
- `internal/testutil/mocks/README.md` — Mock patterns with examples
- `docs/API.md` — All endpoint request/response shapes
- `docs/SCHEMA.md` — Full database schema with indexes
- `.env.example` — All environment variables with comments
```

### A5. Add file size guidance to Code Style

```markdown
- Keep source files under 500 lines. If a file grows beyond this, look for natural split points (separate strategies, separate concerns, utilities)
```

### A6. Normalize migration count

CLAUDE.md references "17 pairs" in the Database section header area. Normalize to the actual count: 19 pairs, 000001-000019.

## Section C: Guardrails

### C1. CI check for hexagonal architecture violations

`scripts/check-imports.sh` — fails CI if any file in `internal/domain/` imports `internal/adapters/`. Grep-based, runs in <1 second.

Added as a step in `.github/workflows/test.yml` before the test step.

### C2. CI check for file size

`scripts/check-file-size.sh` — fails CI if any non-test `.go` file exceeds 600 lines. Warns at 500 lines.

Threshold: 600 gives headroom above the 500-line guideline. Test files excluded (table-driven tests naturally grow long).

### C3. Makefile `check` target

```makefile
check: lint
	./scripts/check-imports.sh
	./scripts/check-file-size.sh
```

Single command for AI agents to validate work.

### Scope boundaries

- **No test coverage gates** — enforcing percentages creates perverse incentives
- **No mandatory test-per-file** — some files (constants, types) don't need tests
- **No pre-commit hooks** — CI is the right enforcement point
