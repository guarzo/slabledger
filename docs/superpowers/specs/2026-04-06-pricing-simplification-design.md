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

### Keep (interface preserved, implementation replaced)

| Interface | Location | Change |
|-----------|----------|--------|
| `pricing.PriceProvider` | `internal/domain/pricing/provider.go` | New DH-backed implementation replaces `FusionPriceProvider` |
| `pricing.Price` struct | `internal/domain/pricing/provider.go` | All fields preserved; market signal fields zero-valued until rebuilt on DH+CL data |
| `campaigns.PriceLookup` | `internal/domain/campaigns/` | No change — wraps whatever `PriceProvider` is injected |
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
| `PCGrades` | N/A (PriceCharting removed) | Nil |
| `FusionMetadata` | N/A (fusion removed) | Nil |
| `Conservative` (P25 exits) | Not available from DH yet | Zero-valued — rebuild later |

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

- Delete `internal/adapters/clients/cardhedger/`
- Delete `internal/adapters/clients/fusionprice/cardhedger_adapter.go`
- Delete `internal/adapters/scheduler/cardhedger_batch.go`, `cardhedger_batch_discovery.go`, `cardhedger_refresh.go`
- Remove CardHedger from fusion provider's secondary sources
- Remove CardHedger wiring from `cmd/slabledger/main.go` and scheduler builder
- Remove CardHedger config/env vars
- Update tests
- Clean up `GradeDetails[].Estimate` to be populated by DH instead of CardHedger

### Phase 2: Replace fusion with DH provider

**Goal:** Remove the fusion engine; DH becomes the direct price source.

- Create `internal/adapters/clients/dhprice/provider.go` implementing `pricing.PriceProvider`
- Wire DH provider into `PriceLookup` adapter (replacing fusion provider)
- Delete `internal/domain/fusion/`
- Delete `internal/adapters/clients/fusionprice/` (entire package)
- Update price refresh scheduler to use DH provider directly
- Update `price_history` writes: DH prices stored with `source='doubleholo'`
- Remove fusion-specific columns from new writes (leave existing data intact)

### Phase 3: Remove PriceCharting + JustTCG

**Goal:** Complete source simplification.

- Delete `internal/adapters/clients/pricecharting/` (~28 files)
- Delete `internal/adapters/clients/justtcg/`
- Delete `internal/adapters/scheduler/justtcg_refresh.go`
- Remove PriceCharting/JustTCG wiring from main.go and scheduler builder
- Remove config/env vars (`PRICECHARTING_TOKEN`, `JUSTTCG_API_KEY`, etc.)
- Clean up PokemonPrice test remnants
- Update integration tests

### Phase 4: Data retention + cleanup (future)

**Goal:** Shrink DB, add retention policies.

- Add retention scheduler for `price_history` (30-day window for historical rows)
- Consider `VACUUM` after bulk deletion to reclaim space
- Rebuild market signal fields (velocity, volatility, conservative exits) on DH + CardLadder data
- Rebuild Monte Carlo / liquidation analysis on trustworthy data

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
| Schedulers (CH, JustTCG) | ~6 | ~1,500 |
| **Total** | **~74** | **~12,300** |

## Files Created

| File | Purpose |
|------|---------|
| `internal/adapters/clients/dhprice/provider.go` | DH-backed `PriceProvider` implementation |
| `internal/adapters/clients/dhprice/provider_test.go` | Tests |

## Success Criteria

- `make db-pull` completes reliably (DB under 100MB after retention)
- `go test ./...` passes with no fusion/CardHedger/PriceCharting/JustTCG references
- Card pricing API returns DH-sourced prices for all matched cards
- AI advisor tools continue functioning (digest, liquidation, campaign analysis)
- `price_history` growth rate drops from ~20K rows/day to near-zero (DH data in `market_intelligence`)
- `make check` passes (lint + architecture + file size)
