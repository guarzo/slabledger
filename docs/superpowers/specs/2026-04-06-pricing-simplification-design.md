# Pricing Pipeline Simplification

**Date:** 2026-04-06
**Status:** Draft
**Trigger:** `make db-pull` timeout from 257MB DB; 97% of `price_history` is CardHedger data producing 0% full fusion

## Problem

The current pricing pipeline has five price sources feeding a fusion engine that produces no meaningful results:

- **CardHedger**: 361K rows in `price_history` (~20K/day), 59% card match rate, 537 discovery failures, **0% full fusion**
- **PriceCharting**: 98/467 card coverage in 7-day window
- **JustTCG**: NM-only pricing, redundant with DH per-grade data
- **PokemonPrice**: Already removed (March 29), test remnants remain
- **Fusion engine**: Weighted median across sources that rarely both hit — no value with 0% full fusion

Meanwhile, DoubleHolo (DH) is becoming the primary marketplace with actual transaction data, higher fusion weight (0.90), and — once current matching fix lands — 100% card match rate.

## Decision

Replace five price sources + fusion engine with DH as the sole price source. Keep CardLadder (value anchor) and TCGdex (metadata) unchanged.

### Remove

| Component | Location | Reason |
|-----------|----------|--------|
| CardHedger client | `internal/adapters/clients/cardhedger/` | 0% fusion value, 97% of DB bloat |
| CardHedger fusion adapter | `internal/adapters/clients/fusionprice/cardhedger_adapter.go` | Part of fusion |
| CardHedger schedulers | `internal/adapters/scheduler/cardhedger_*.go` | Batch, delta poll, batch discovery |
| PriceCharting client | `internal/adapters/clients/pricecharting/` (~28 files) | Replaced by DH |
| JustTCG client | `internal/adapters/clients/justtcg/` | NM-only, redundant with DH |
| JustTCG scheduler | `internal/adapters/scheduler/justtcg_refresh.go` | No longer needed |
| Fusion engine | `internal/domain/fusion/` | Nothing to fuse |
| Fusion price provider | `internal/adapters/clients/fusionprice/` | Replaced by DH provider |
| PokemonPrice remnants | Test fixtures referencing `source='pokemonprice'` | Dead code cleanup |

### Keep (unchanged)

| Component | Location | Role |
|-----------|----------|------|
| DH client | `internal/adapters/clients/dh/` | Primary price source + marketplace |
| DH schedulers | `internal/adapters/scheduler/dh_*.go` | Push, inventory poll, orders, intelligence, suggestions |
| CardLadder client | `internal/adapters/clients/cardladder/` | Value tracking, price anchor, sales comps |
| CardLadder scheduler | `internal/adapters/scheduler/cardladder_refresh.go` | Daily value sync |
| TCGdex client | `internal/adapters/clients/tcgdex/` | Card/set metadata catalog (not a price source) |

### Also Remove (missed in initial scope)

| Component | Location | Reason |
|-----------|----------|--------|
| Cert sweep adapter | `internal/adapters/scheduler/cert_sweep_adapter.go` | CardHedger-specific |
| `discovery_failures` table usage | `pricing_diagnostics.go`, domain interface | CardHedger-only; table can be dropped |
| `DiscoveryFailureTracker` interface | `internal/domain/pricing/repository.go` | CardHedger-only |
| `PCGrades` field on `pricing.Price` | `internal/domain/pricing/provider.go` | PriceCharting-specific |
| `RawNMCents` field on `GradedPrices` | `internal/domain/pricing/provider.go` | JustTCG-specific |
| `pricing_diagnostics.go` fusion logic | `internal/adapters/storage/sqlite/` | `queryCardQuality()` classifies "full_fusion"/"partial"/"pc_only" — obsolete |
| Price hints handler (PC/CH) | `internal/adapters/httpserver/handlers/price_hints.go` | Only supports `pricecharting` and `cardhedger` providers |
| Missing Cards admin tab | `web/src/react/pages/admin/MissingCardsTab.tsx` | CardHedger-dependent |
| API status CardHedger stats | `handlers/api_status_handler.go` | Dead provider references |
| `PriceHintDialog` provider selector | `web/src/react/PriceHintDialog.tsx` | Only lists PriceCharting + CardHedger |

### Keep (interface preserved, implementation replaced)

| Interface | Location | Change |
|-----------|----------|--------|
| `pricing.PriceProvider` | `internal/domain/pricing/provider.go` | New DH-backed implementation replaces `FusionPriceProvider` |
| `pricing.Price` struct | `internal/domain/pricing/provider.go` | Market signal fields preserved (zero-valued until rebuilt); remove dead fields (`PCGrades`, `RawNMCents`, `FusionMetadata`) |
| `campaigns.PriceLookup` | `internal/domain/campaigns/` | No change — wraps whatever `PriceProvider` is injected; remove JustTCG NM logic |
| `ai.ToolExecutor` tools | `internal/adapters/advisortool/` | No change — calls through `PriceLookup` / `PriceProvider` interfaces |

## Architecture: New DH Price Provider

The new `DHPriceProvider` implements `pricing.PriceProvider` by querying DH market data for per-grade prices.

```
DH MarketData API
  └─ DHPriceProvider (implements pricing.PriceProvider)
       ├─ GetPrice(card) → pricing.Price with per-grade data from DH recent sales
       ├─ LookupCard(set, card) → resolve via TCGdex + DH match, then GetPrice
       └─ Caches in memory (existing cache infrastructure)
            └─ Falls back to market_intelligence DB table on cache miss
                 └─ Returns zero-value Price if no data available

CardLadder (separate, unchanged)
  └─ CLValueCents on purchases — price floor/anchor via applyCLCorrection()
```

### What the DH provider populates on `pricing.Price`

| Field | Source | Status |
|-------|--------|--------|
| `PriceCents` (per grade) | DH recent sales median | Populated |
| `GradeDetails[].Estimate` | DH sale prices as estimates | Populated (repurposed from CardHedger) |
| `Confidence` | Based on DH data freshness + sale count | Populated (simplified) |
| `Source` | `"doubleholo"` | Populated |
| `MarketData` (velocity, listings, volatility) | Not available from DH yet | Zero-valued — rebuild later on DH+CL data |
| `Conservative` (P25 exits) | Not available from DH yet | Zero-valued — rebuild later |

### Fields to remove from `pricing.Price`

| Field | Reason |
|-------|--------|
| `PCGrades` | PriceCharting-specific; no longer populated |
| `FusionMetadata` | Fusion engine removed |
| `RawNMCents` (on `GradedPrices`) | JustTCG-specific |

### Domain constants to remove

| Constant | Location |
|----------|----------|
| `SourcePriceCharting` | `internal/domain/pricing/provider.go` |
| `SourceCardHedger` | `internal/domain/pricing/provider.go` |
| `SourceJustTCG` | `internal/domain/pricing/provider.go` |

Keep: `SourceDH = "doubleholo"`

### Downstream consumers — no changes needed

These all consume `pricing.Price` through interfaces and handle missing/zero fields gracefully:

- **Campaign snapshots** (`service_snapshots.go`) — `GetMarketSnapshot()` works with any provider
- **Expected values** (`service_advanced.go`) — uses snapshot data, handles nil market data
- **AI advisor tools** (`advisortool/`) — LLM sees whatever data the tools return
- **Card pricing API** (`handlers/card_pricing.go`) — returns available fields, zeros for missing
- **Frontend** (`CardPriceCard.tsx`) — already handles missing grade data / market signals

## Quick Fix: `make db-pull` Resilience

Independent of the phased work below, add SSH keepalive options to the `db-pull` and `db-push` Makefile targets so they survive larger DB transfers:

```makefile
SSH_OPTS ?= -o ServerAliveInterval=60 -o ServerAliveCountMax=10
```

Apply to all `ssh` and `scp` commands in those targets. This is a one-line fix that unblocks the immediate problem regardless of simplification timeline.

## Phased Implementation

### Prerequisites

- DH card matching fix must be landed and validated (100% match rate confirmed)
- Phase 1 can proceed immediately (CardHedger removal has no DH dependency — 0% fusion means no data loss)
- Phases 2-3 should wait for DH matching to be solid

### Phase 1: Remove CardHedger

**Goal:** Immediate DB bloat relief — eliminates ~20K rows/day (97% of growth).

**Packages to delete:**
- `internal/adapters/clients/cardhedger/` (entire package)
- `internal/adapters/clients/fusionprice/cardhedger_adapter.go`
- `internal/adapters/scheduler/cardhedger_batch.go`, `cardhedger_batch_discovery.go`, `cardhedger_refresh.go`
- `internal/adapters/scheduler/cert_sweep_adapter.go`

**Wiring to update:**
- `cmd/slabledger/main.go` — remove CardHedger client init, `CardHedgerStats`, `CardDiscoverer` from ServerDeps
- `cmd/slabledger/init.go` — remove CardHedger from `initializePriceProviders()` secondary sources, remove from `initializeCampaignsService()` cert resolver
- `internal/adapters/scheduler/builder.go` — remove `CardHedgerClient` interface, `CardHedgerClientImpl` field, batch/refresh scheduler creation, `CardDiscoverer` from BuildResult

**Config to remove:**
- `internal/platform/config/types.go` — `CardHedgerSchedulerConfig` struct, `CardHedgerKey`/`CardHedgerClientID` fields
- `internal/platform/config/loader.go` — `CARD_HEDGER_*` env var loading
- `.env.example` — `CARD_HEDGER_*` vars

**Domain cleanup:**
- Remove `SourceCardHedger` constant from `pricing/provider.go`
- Remove `DiscoveryFailureTracker` interface from `pricing/repository.go`
- `discovery_failures` table — stop writing (no migration yet, just stop using)

**Frontend:**
- Remove CardHedger from `api_status_handler.go` provider lists and stats
- Remove CardHedger from `PriceHintDialog.tsx` provider selector
- Remove `MissingCardsTab.tsx` or make it DH-aware

**Tests:**
- Remove CardHedger-specific mocks and test fixtures
- Update any test that references `SourceCardHedger`

### Phase 2: Replace fusion with DH provider

**Goal:** Remove the fusion engine; DH becomes the direct price source.

**New code:**
- Create `internal/adapters/clients/dhprice/provider.go` implementing `pricing.PriceProvider`

**Packages to delete:**
- `internal/domain/fusion/` (entire package)
- `internal/adapters/clients/fusionprice/` (entire package — DH adapter moves to new dhprice package)

**Wiring to update:**
- `cmd/slabledger/init.go` — replace `FusionPriceProvider` creation with `DHPriceProvider`
- Price refresh scheduler — simplify to use DH provider directly

**Domain cleanup:**
- Remove `FusionMetadata` from `pricing.Price` struct
- Remove `fusion_source_count`/`fusion_outliers_removed`/`fusion_method` from new `price_history` writes
- Simplify `pricing_diagnostics.go` — remove "full_fusion"/"partial"/"pc_only" classification
- Remove `FusionConfig` from `internal/platform/config/types.go`
- Remove `FUSION_*` env vars from loader and `.env.example`

**Frontend:**
- Remove `fusionConfidence` display from inventory rows (`DesktopRow.tsx`, `MobileCard.tsx`)

### Phase 3: Remove PriceCharting + JustTCG

**Goal:** Complete source simplification.

**Packages to delete:**
- `internal/adapters/clients/pricecharting/` (~28 files)
- `internal/adapters/clients/justtcg/`
- `internal/adapters/scheduler/justtcg_refresh.go`

**Wiring to update:**
- `cmd/slabledger/main.go` — remove PriceCharting/JustTCG client init, `pcProvider.Close()` cleanup
- `cmd/slabledger/init.go` — remove PriceCharting from `initializePriceProviders()`
- `internal/adapters/scheduler/builder.go` — remove `JustTCGClient` field, JustTCG scheduler creation

**Config to remove:**
- `internal/platform/config/types.go` — `JustTCGConfig` struct, `PriceChartingToken`/`JustTCGKey` fields
- `internal/platform/config/loader.go` — `PRICECHARTING_TOKEN`, `JUSTTCG_*` env var loading
- `.env.example` — all PriceCharting/JustTCG vars
- `cmd/slabledger/server.go` — remove PriceCharting from required env validation

**Domain cleanup:**
- Remove `SourcePriceCharting`, `SourceJustTCG` constants
- Remove `PCGrades` field from `pricing.Price`
- Remove `RawNMCents` from `GradedPrices`
- Remove JustTCG NM logic from `pricelookup/adapter.go`
- Update `price_hints.go` handler — remove PriceCharting/CardHedger provider validation
- Clean up PokemonPrice test remnants

**Frontend:**
- Remove PriceCharting from `PriceHintDialog.tsx`
- Remove PriceCharting/JustTCG from `ApiStatusTab.tsx` provider lists
- Update pricing type definitions (`web/src/types/pricing.ts`)

**Database migration (new migration 000032 or later):**
- Update `CHECK` constraints on `price_history.source`, `api_calls.provider`, `api_rate_limits.provider`, `price_refresh_queue.source` to remove dead providers
- Keep `'pokemonprice'` and `'cardmarket'` in constraints for historical data compatibility
- Add `'doubleholo'` if not already present
- Optionally: `DELETE FROM discovery_failures` (or drop table if fully unused)
- Optionally: `DELETE FROM card_id_mappings WHERE provider IN ('cardhedger', 'pricecharting', 'justtcg')`

### Phase 4: Data retention + cleanup (future)

**Goal:** Shrink DB, add retention policies.

- Add retention scheduler for `price_history` (30-day window for historical rows)
- Consider `VACUUM` after bulk deletion to reclaim space
- Rebuild market signal fields (velocity, volatility, conservative exits) on DH + CardLadder data
- Rebuild Monte Carlo / liquidation analysis on trustworthy data
- Clean up `api_rate_limits` entries for dead providers
- Evaluate whether `price_history` table is still needed at all (DH data lives in `market_intelligence`)

## DB Impact

### Immediate (after Phase 3)

- No new rows written to `price_history` with sources: `cardhedger`, `pricecharting`, `justtcg`, `fusion`
- Only `doubleholo` source rows going forward
- Historical rows preserved (no destructive migration)
- `fusion_source_count`, `fusion_outliers_removed`, `fusion_method` columns go unpopulated (no migration needed)

### With retention (Phase 4)

- Delete `price_history` rows older than 30 days
- Expected DB reduction: ~200MB+ (from 257MB current)
- `VACUUM` to reclaim disk space

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| DH matching not ready | Phase 1 (CardHedger removal) is safe regardless — 0% fusion means no data loss |
| DH API downtime | DHPriceProvider falls back to cached `market_intelligence` DB rows, then to zero-value Price |
| Market signals lost | Fields preserved on `pricing.Price`; rebuild on DH+CL data in Phase 4 |
| LLM advisor degraded | Tools work through interfaces; missing market data degrades gracefully (already handles nil) |
| CardLadder anchor breaks | CardLadder is fully independent of this change |

## Files Removed (approximate count)

| Package | Files | Lines (est.) |
|---------|-------|-------------|
| `clients/cardhedger/` | ~15 | ~2,500 |
| `clients/pricecharting/` | ~28 | ~4,000 |
| `clients/justtcg/` | ~5 | ~800 |
| `clients/fusionprice/` | ~15 | ~3,000 |
| `domain/fusion/` | ~5 | ~500 |
| Schedulers (CH, JustTCG, cert sweep) | ~7 | ~1,800 |
| Config structs (FusionConfig, CH, JustTCG) | partial | ~200 |
| **Total** | **~75+** | **~12,800** |

## Files Modified

| File | Change |
|------|--------|
| `cmd/slabledger/main.go` | Remove CH/PC/JustTCG client init and wiring |
| `cmd/slabledger/init.go` | Replace `initializePriceProviders()` entirely |
| `cmd/slabledger/server.go` | Remove PriceCharting required check, CH stats |
| `internal/adapters/scheduler/builder.go` | Remove CH/JustTCG deps and scheduler creation |
| `internal/domain/pricing/provider.go` | Remove dead source constants and struct fields |
| `internal/domain/pricing/repository.go` | Remove `DiscoveryFailureTracker` interface |
| `internal/adapters/clients/pricelookup/adapter.go` | Remove JustTCG NM logic |
| `internal/adapters/storage/sqlite/pricing_diagnostics.go` | Remove fusion classification logic |
| `internal/adapters/httpserver/handlers/api_status_handler.go` | Remove dead provider entries |
| `internal/adapters/httpserver/handlers/price_hints.go` | Remove CH/PC provider validation |
| `internal/platform/config/types.go` | Remove dead config structs |
| `internal/platform/config/loader.go` | Remove dead env var loading |
| `.env.example` | Remove dead env vars |
| `web/src/react/PriceHintDialog.tsx` | Remove CH/PC provider selector |
| `web/src/react/pages/admin/ApiStatusTab.tsx` | Remove dead provider display |
| `web/src/react/pages/admin/MissingCardsTab.tsx` | Remove or repurpose |
| `web/src/types/pricing.ts` | Update provider types |

## Files Created

| File | Purpose |
|------|--------|
| `internal/adapters/clients/dhprice/provider.go` | DH-backed `PriceProvider` implementation |
| `internal/adapters/clients/dhprice/provider_test.go` | Tests |
| New migration (000032+) | Update CHECK constraints for dead providers |

## Additional Cleanup Opportunities

Items not strictly required for the simplification but worth addressing:

| Item | Location | Notes |
|------|----------|-------|
| `discovery_failures` table | migrations/000004 | Can be dropped entirely — only CardHedger used it |
| `card_id_mappings` dead rows | DB data | Rows for `cardhedger`/`pricecharting`/`justtcg` providers are dead |
| `api_rate_limits` dead rows | DB data | Entries for removed providers are dead |
| `price_history` historical rows | DB data | 371K+ rows from removed sources; retention scheduler in Phase 4 |
| `httpx` client | `clients/httpx/` | Still used by DH, CardLadder, Instagram, Azure — keep as-is |
| `price_refresh_queue` table | migrations | Evaluate if still needed — may be obsolete without fusion-based refresh |
| `scoring` domain | `domain/scoring/` | Independent of pricing sources — no changes needed |
| `social` domain | `domain/social/` | Independent of pricing sources — no changes needed |
| Memory file `feedback_cardhedger_freely.md` | `.claude/projects/` memory | Obsolete after CardHedger removal |

## Success Criteria

- `make db-pull` completes reliably (DB under 100MB after retention)
- `go test ./...` passes with no fusion/CardHedger/PriceCharting/JustTCG references
- Card pricing API returns DH-sourced prices for all matched cards
- AI advisor tools continue functioning (digest, liquidation, campaign analysis)
- `price_history` growth rate drops from ~20K rows/day to near-zero (DH data in `market_intelligence`)
- `make check` passes (lint + architecture + file size)
