# Phase 4: Data Retention & Analytics Rebuild

**Status:** Deferred — implement after Phase 3 is merged and DH matching is stable.

**Context:** Phases 1-3 removed CardHedger, PriceCharting, JustTCG, and the fusion engine. No new rows are written for these sources, but ~374K historical rows remain in `price_history` (365K cardhedger, 6K fusion, 3K pricecharting). The DB is 257MB.

## 1. Data Cleanup

### price_history retention

| Source | Rows | Action |
|--------|------|--------|
| cardhedger | 365,050 | Delete — source removed, data not used |
| fusion | 6,008 | Delete — source removed, data not used |
| pricecharting | 3,121 | Delete — source removed, data not used |
| doubleholo | 0 (new) | Retain with 30-day rolling window |

**Implementation:** Add a retention scheduler (similar to `AccessLogCleanupScheduler`) that runs daily and deletes `price_history` rows older than 30 days. Consider a one-time migration to bulk-delete all rows from removed sources.

### Other tables to clean

| Table | Rows | Action |
|-------|------|--------|
| `api_calls` | 27,540 | Delete rows for removed providers (cardhedger, pricecharting, justtcg) |
| `api_rate_limits` | 2 | Delete rows for removed providers |
| `discovery_failures` | 538 | Delete all — table was CardHedger-only |
| `card_id_mappings` | varies | Delete rows where `provider IN ('cardhedger', 'pricecharting', 'justtcg')` |
| `price_refresh_queue` | 0 | Already empty; evaluate if table is still needed |

### VACUUM

After bulk deletion, run `VACUUM` to reclaim disk space. This rebuilds the entire database file and should reduce the DB from ~257MB to well under 50MB.

**Warning:** `VACUUM` requires exclusive access and temporarily doubles disk usage. Run during a maintenance window or on a quiet server.

## 2. Evaluate price_history usage

With DH as the sole source, `price_history` may no longer be the right place to store prices. DH data already lives in `market_intelligence`. Consider:

- Does `DHPriceProvider` write to `price_history` at all? If not, the table may become fully historical.
- Could the price refresh scheduler be simplified to just refresh `market_intelligence`?
- Should `price_history` be retained only for the `card_access_log` cleanup pattern, or dropped entirely?

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
