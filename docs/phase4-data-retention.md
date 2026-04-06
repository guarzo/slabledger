# Phase 4: Data Retention & Analytics Rebuild

**Status:** Phase 4a/4b complete (2026-04-06). Phase 4c/4d deferred.

**Context:** Phases 1-3 removed CardHedger, PriceCharting, JustTCG, and the fusion engine. Phase 4a/4b dropped `price_history` entirely (no production code wrote to it) and rewrote the price refresh scheduler.

## 1. Data Cleanup — COMPLETE (Phase 4a)

Migration `000038_drop_price_history` dropped:
- `price_history` table (~374K legacy rows, 0 active writes)
- `stale_prices` VIEW (depended on price_history)
- `price_refresh_queue` table (empty, unused)
- `discovery_failures` table (CardHedger-only)
- Orphan rows in `api_calls`, `api_rate_limits`, `card_id_mappings` for removed providers

`PriceRepository` interface and struct renamed to `DBTracker` (retains APITracker, AccessTracker, HealthChecker).

### VACUUM

After deploying migration 000038, run `VACUUM` manually to reclaim disk space (~257MB → expected well under 50MB).

## 2. price_history — DROPPED (Phase 4b)

Confirmed: `DHPriceProvider` never wrote to `price_history`. It computes prices in-memory from DH API calls. The table was 100% legacy data.

The price refresh scheduler was rewritten to use `RefreshCandidateProvider` — queries unsold inventory from `campaign_purchases` instead of the old `stale_prices` VIEW. This is a cache-warming pass: it calls `GetPrice` to resolve DH card IDs for inventory cards.

Two bugs fixed:
- **Nil provider silent no-op**: Scheduler now checks `priceProvider.Available()` at top of batch and returns early with warning
- **Nil result counted as success**: `GetPrice` returning `(nil, nil)` now tracked as `noDataCount`, not `successCount`

## 3. Rebuild Market Analytics on DH + CardLadder

The removed sources (PriceCharting) provided market signals that are currently zero-valued on `pricing.Price`:

| Signal | Old Source | Rebuild Strategy |
|--------|-----------|------------------|
| `ActiveListings` | PriceCharting | Not available from DH — needs new data source or scraping |
| `SalesLast30d` / `SalesLast90d` | PriceCharting | Derive from DH `recent_sales` by counting within window |
| `SalesVelocity` | PriceCharting | Derive from DH recent_sales count / time window |
| `Volatility` | PriceCharting | Compute stddev from DH recent sale prices |
| `ListingVelocity` | PriceCharting | Not available from DH |
| `Conservative` (P25 exits) | PriceCharting | Compute P25 from DH recent sale prices |
| `Distributions` (P10-P90) | PriceCharting | Compute from DH recent sale prices |
| `LastSoldByGrade` | PriceCharting | Extract from DH recent_sales (already has grade + date) |

**Priority order:**
1. `LastSoldByGrade` — easiest, most useful for campaign snapshots
2. `SalesLast30d` / velocity — needed for Monte Carlo
3. `Conservative` P25 — needed for profit optimization
4. `Distributions` — needed for Monte Carlo
5. `Volatility` — nice-to-have for risk assessment
6. `ActiveListings` / `ListingVelocity` — blocked without new data source

### CardLadder data enrichment

CardLadder provides:
- `cl_sales_comps` — historical sale records with platform, price, date
- `cl_value_history` — daily value snapshots per card
- Weekly/monthly percent change from collection sync

These could supplement DH data for:
- Cross-validating price trends (CL value trend vs DH sale trend)
- Filling gaps where DH has few recent sales but CL has active comps
- Anchoring Monte Carlo simulations with CL value as a floor

## 4. Rebuild Monte Carlo / Liquidation Analysis

The Monte Carlo simulation (`internal/domain/campaigns/montecarlo.go`) and liquidation analysis (`internal/domain/advisor/`) currently use market signals that are zero-valued. Once the signals above are rebuilt:

1. Verify Monte Carlo inputs are populated from rebuilt signals
2. Re-enable liquidation confidence scoring with actual data
3. Validate output against known outcomes (historical sales with known P&L)

This is the long-term goal — making these analyses trustworthy on actual DH + CardLadder data rather than questionable PriceCharting data with thin coverage.
