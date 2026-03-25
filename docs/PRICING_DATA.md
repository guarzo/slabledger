# Pricing Data Architecture

Comprehensive reference for how price data flows from external APIs through the fusion engine to the UI.

## Table of Contents

- [Data Sources](#data-sources)
- [Fusion Engine](#fusion-engine)
- [Data Flow](#data-flow)
- [Caching & Persistence](#caching--persistence)
- [Rate Limit Management](#rate-limit-management)
- [Frontend Display](#frontend-display)
- [Key Files](#key-files)
- [Historical Bugs & Fixes](#historical-bugs--fixes)

---

## Data Sources

### PriceCharting (Primary)

**Role**: Market data provider — sales history, listings, conservative exits. Grade prices are stored separately but NOT fed into the fusion engine.

**Client**: `internal/adapters/clients/pricecharting/`

| Field | Description |
|-------|-------------|
| `PSA10Cents`, `Grade9Cents`, `PSA8Cents` | Grade-specific market prices (cents) |
| `LooseCents` | Raw/ungraded price (mapped to `RawCents`) |
| `Grade95Cents`, `BGS10Cents` | Additional grade formats |
| `ActiveListings` | Current number of marketplace listings |
| `LowestListing` | Lowest active listing price |
| `ListingVelocity` | Sales per day |
| `PriceVolatility` | Coefficient of variation |
| `Sales30d`, `Sales90d` | Sales volume by time period |
| `RecentSales[]` | Individual sale records (price, date, grade) |
| `LastSoldByGrade` | Per-grade last sold price + date |
| `ConservativePSA10USD` | P25 percentile — risk-adjusted floor |
| `PSA10Distribution` | Full percentile data (P10/P25/P50/P75/P90) |

**Grade mapping** (Pokemon): `cib-price` = PSA 8, `graded-price` = PSA 9, `manual-only-price` = PSA 10.

**Key behavior**: PriceCharting data is used for market context (listings, velocity, conservative exits, last sold) but its grade prices are stored as `PCGrades` on `pricing.Price` separately from the fused grades. This ensures `buildSourcePrices()` shows PriceCharting's actual price, not the fused result.

### PokemonPrice (eBay Sales Data)

**Role**: Primary graded pricing source via eBay completed sales.

**Client**: `internal/adapters/clients/pokemonprice/`

| Field | Description |
|-------|-------------|
| `SmartMarketPrice.Price` | Weighted market estimate per grade |
| `SmartMarketPrice.Confidence` | `"high"` / `"medium"` / `"low"` |
| `Count` | Number of eBay sales in lookback window |
| `MedianPrice`, `MinPrice`, `MaxPrice` | Price distribution |
| `MarketPrice7Day` | 7-day rolling average |
| `MarketTrend` | `"up"` / `"down"` / `"stable"` |
| `DailyVolume7Day` | 7-day daily volume |
| `SalesVelocity.DailyAverage` | Cards sold per day |
| `SalesVelocity.MonthlyTotal` | Total monthly sales |

**Set name normalization**: PSA categories (e.g. "2013 Pokemon Black & White Promos") don't match PokemonPrice naming. Fixed with `cardutil.NormalizeSetNameSimple()` stripping year + "Pokemon" prefix, plus retry-without-set fallback.

**Stored as**: `pricing.Price.GradeDetails["psa10"].Ebay` (EbayGradeDetail struct, prices in cents).

### CardHedger (Multi-platform Estimate)

**Role**: Supplementary pricing from cross-platform aggregation.

**Client**: `internal/adapters/clients/cardhedger/`

| Field | Description |
|-------|-------------|
| `GradePrice.Price` | Estimated value per grade (string, USD) |
| `PriceEstimate.Price` | Point estimate (float64) |
| `PriceEstimate.PriceLow`, `PriceHigh` | Confidence interval |
| `PriceEstimate.Confidence` | 0–1 numeric score |

**Stored as**: `pricing.Price.GradeDetails["psa10"].Estimate` (EstimateGradeDetail struct, prices in cents).

**Rate limits**: Unlimited plan; actual limits monitored via 429 tracking. Data comes from both on-demand lookups and batch/delta schedulers.

---

## Fusion Engine

**Location**: `internal/domain/fusion/engine.go`

### Algorithm: Weighted Median

1. **Convert to weighted source data** — each price gets a composite weight:
   ```
   weight = base_weight × confidence × freshness × volume
   ```

   | Factor | Multiplier |
   |--------|-----------|
   | **Base weight** | PriceCharting: 0.9, PokemonPrice: 0.90, CardHedger: 0.85 |
   | **Freshness** | < 1h: 1.0, < 24h: 0.9, < 7d: 0.7, > 7d: 0.5 |
   | **Volume** | 1–5 items: 0.6, 6–20: 0.8, 21+: 1.0 |
   | **Confidence** | Direct multiplier from source |

2. **Outlier removal** (IQR method) — remove prices outside [Q1 − 1.5×IQR, Q3 + 1.5×IQR]

3. **Weighted median** — sort by value, find price where cumulative weight ≥ 50%

4. **Confidence scoring** (4-factor composite):
   - Source count: `sources / MinSources` (30% weight)
   - Price variance: `1 / (1 + CV)` (30% weight)
   - Outlier ratio: `1 − outliers / total` (20% weight)
   - Weight quality: average weight of filtered sources (20% weight)

### What enters the fusion vs what doesn't

| Source | Enters fusion engine? | Why |
|--------|----------------------|-----|
| PokemonPrice | Yes | Primary graded pricing |
| CardHedger | Yes (when available) | Supplementary estimate |
| PriceCharting | **No** | Used for market data only (listings, velocity, conservative exits, last sold) |

This means `pricing.Price.Grades` (the fused result) is driven by PokemonPrice and CardHedger. PriceCharting's raw grade prices are carried separately in `pricing.Price.PCGrades`.

---

## Data Flow

```
┌──────────────────────────────────────────────────────┐
│                   EXTERNAL APIs                       │
├─────────────────┬──────────────────┬─────────────────┤
│  PriceCharting  │  PokemonPrice    │  CardHedger     │
│  (market data)  │  (eBay sales)    │  (estimates)    │
└────────┬────────┴────────┬─────────┴────────┬────────┘
         │                 │                   │
         ▼                 ▼                   ▼
┌──────────────────────────────────────────────────────┐
│  Source Adapters (fusionprice/source_adapters.go)     │
│  convertPokemonPriceWithDetails()                     │
│  convertCardHedgerWithDetails()                       │
└──────────┬──────────────────────────┬────────────────┘
           │ fusion.PriceData[]       │ FetchResult
           ▼                          │ (EbayDetails,
┌──────────────────────┐              │  EstimateDetails,
│  Fusion Engine       │              │  Velocity)
│  (domain/fusion/)    │              │
│  Weighted median     │              │
│  per grade           │              │
└──────────┬───────────┘              │
           ▼                          ▼
┌──────────────────────────────────────────────────────┐
│  FusionPriceProvider.GetPrice()                       │
│  (fusionprice/fusion_provider.go)                     │
│                                                       │
│  result.Grades ← fused prices (PokemonPrice+CH)      │
│  result.PCGrades ← PriceCharting raw grade prices     │
│  result.GradeDetails ← per-source detail data         │
│  result.Market ← PriceCharting market data            │
│  result.LastSoldByGrade ← PriceCharting sales history │
│  result.Conservative ← PriceCharting P25 exits        │
│  result.Distributions ← PriceCharting percentiles     │
│  result.Velocity ← PokemonPrice sales velocity        │
└──────────┬───────────────────────────┬───────────────┘
           │                           │
    ┌──────▼──────┐          ┌─────────▼──────────┐
    │ Memory Cache│          │ SQLite DB           │
    │ (4h TTL)    │          │ (48h freshness)     │
    │ Details (48h│          │ Per-grade entries:   │
    │  TTL)       │          │  source="fusion"    │
    └──────┬──────┘          │  source="pricecharting"│
           │                 │  source="cardhedger"│
           │                 └─────────┬──────────┘
           └──────────┬────────────────┘
                      ▼
┌──────────────────────────────────────────────────────┐
│  pricelookup/Adapter.GetMarketSnapshot()              │
│  Converts pricing.Price → campaigns.MarketSnapshot    │
│  buildSourcePrices() → SourcePrice[] array            │
└──────────────────────────┬───────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────┐
│  HTTP API → JSON → Frontend TypeScript types          │
│  campaigns.MarketSnapshot → MarketSnapshot            │
│  campaigns.SourcePrice → SourcePrice                  │
└──────────────────────────────────────────────────────┘
```

### MarketSnapshot Construction

`pricelookup/adapter.go` builds `campaigns.MarketSnapshot` from `pricing.Price`:

| MarketSnapshot field | Source | Notes |
|---------------------|--------|-------|
| `GradePriceCents` | Fused `price.Grades` | Grade-specific fused price |
| `MedianCents` | `Distributions.P50` | Falls back to `GradePriceCents` |
| `ConservativeCents` | `Conservative` or `Distributions.P25` | Falls back to 85% of median |
| `OptimisticCents` | `Distributions.P75` | Falls back to 115% of median |
| `P10Cents`, `P90Cents` | `Distributions` | Falls back to 70%/130% of median |
| `LastSoldCents`, `LastSoldDate` | Fallback chain: `LastSoldByGrade` → `PCGrades` → `GradeDetails.Ebay` → `GradeDetails.Estimate` | See [LastSold Fallback Chain](#lastsold-fallback-chain) |
| `LowestListCents` | `Market.LowestListing` | From PriceCharting |
| `ActiveListings` | `Market.ActiveListings` | From PriceCharting |
| `SalesLast30d`, `SalesLast90d` | `Market` | From PriceCharting |
| `Volatility` | `Market.Volatility` | From PriceCharting |
| `DailyVelocity`, `MonthlyVelocity` | `Velocity` | From PokemonPrice |
| `Avg7DayCents` | First `SourcePrice` with value | From PokemonPrice |
| `FusionConfidence` | `price.Confidence` | 0–1 composite score |
| `SourceCount` | `FusionMetadata.SourceCount` | Number of sources in fusion |
| `SourcePrices[]` | `buildSourcePrices()` | See below |

### buildSourcePrices()

Builds the per-source price breakdown displayed in expanded detail and sell sheet:

| Source label | Data origin | Fields populated |
|-------------|-------------|-----------------|
| `"PriceCharting"` | `price.PCGrades` (raw API price) | `PriceCents` only |
| `"PokemonPrice"` | `GradeDetails[grade].Ebay` | `PriceCents`, `SaleCount`, `Trend`, `Confidence`, `MinCents`, `MaxCents` |
| `"CardHedger"` | `GradeDetails[grade].Estimate` | `PriceCents`, `MinCents`, `MaxCents`, `Confidence` (bucketed to high/medium/low) |

---

## Caching & Persistence

### In-Memory Cache

| Cache | TTL | Key pattern | Contents |
|-------|-----|-------------|----------|
| Main price cache | 4 hours | `fusion:{set}:{name}:{number}` | Full `pricing.Price` |
| Details cache | 48 hours | `details:{set}:{name}:{number}` | `GradeDetails`, `Velocity`, `Sources`, `PCGrades` |

The details cache has a longer TTL so grade-level detail data (per-source prices, velocity) survives between main cache expiry and DB freshness window.

### Database Persistence

**Location**: `internal/adapters/storage/sqlite/` — `price_history` table

**Freshness duration**: 48 hours (configurable, `DefaultFreshnessDuration`)

Prices are stored per-grade with a source label:

| Source | Grade examples | Stored by |
|--------|---------------|-----------|
| `"fusion"` | PSA 10, PSA 9, PSA 8, Raw | `GetPrice()` after fusion |
| `"pricecharting"` | PSA 10, PSA 9, PSA 8, Raw | `GetPrice()` alongside fusion |
| `"cardhedger"` | PSA 10, PSA 9, PSA 8, Raw | Batch/delta schedulers |

### Cache Miss Recovery

When the in-memory details cache expires, `supplementWithCachedDetails()` reconstructs data from the DB:

1. **CardHedger**: Queries `GetLatestPrice(card, grade, "cardhedger")` for each grade, reconstructs `EstimateGradeDetail` entries in `GradeDetails`
2. **PriceCharting grades**: Queries `GetLatestPrice(card, grade, "pricecharting")` for each grade, reconstructs `PCGrades`

This ensures source price data survives server restarts and cache expiry.

---

## Rate Limit Management

### CardHedger

| Parameter | Value | Env var |
|-----------|-------|---------|
| Plan | Unlimited | — |
| Rate limit | 100 req/min, burst 5 | — |
| 429 monitoring | Atomic counter + structured logging | — |
| Batch interval | 24 hours | `CARD_HEDGER_BATCH_INTERVAL` |
| Delta poll interval | 1 hour | `CARD_HEDGER_POLL_INTERVAL` |
| Max cards per batch | 200 | `CARD_HEDGER_MAX_CARDS_PER_RUN` |

CardHedger data enters the system through:
- **On-demand lookups** — uses `card-match` endpoint for AI-powered card resolution
- **Batch scheduler** (daily) — refreshes favorites + active campaign cards
- **Delta poll scheduler** (hourly) — fetches price updates for mapped cards
- **Post-import cert resolution** — `details-by-certs` batch resolves cert→card_id mappings

### PriceCharting

Rate-limited via circuit breaker in `httpx`. No daily call limit but throttled to avoid 429s.

### Call Path Summary

| Path | PriceCharting | PokemonPrice | CardHedger |
|------|:---:|:---:|:---:|
| On-demand (user views inventory) | Yes | Yes | Yes |
| Batch scheduler (background) | Yes | Yes | Yes |
| Delta poll (background) | — | — | Yes |
| Post-import cert resolution | — | — | Yes |

---

## Frontend Display

### Inventory Tab (`InventoryTab.tsx`)

**Desktop columns**:

| Column | Width | Data | Source |
|--------|-------|------|--------|
| Card | flex | `cardName` | Purchase record |
| Cert # | 80px | `certNumber` | Purchase record |
| Gr | 36px | `psaGrade` | Purchase record |
| Cost | 72px | `buyCostCents + psaSourcingFeeCents` | Purchase record |
| Market | 90px | `bestPrice(snap)` = median → gradePrice → lastSold | MarketSnapshot |
| Range | 120px | `conservativeCents – optimisticCents` (P25–P75) | MarketSnapshot |
| Sales | 70px | `velocityLabel()` + active listings count | MarketSnapshot |
| P/L | 72px | `bestPrice - costBasis` (color-coded) | Computed |
| Days | 44px | `daysHeld` (30d yellow, 60d red thresholds) | AgingItem |
| Signal | 80px | Rising/Falling/Stable badge + delta % | AgingItem.signal or snap.trend30d |
| Conf | 36px | `ConfidenceIndicator` dots | snap.fusionConfidence |

**Signal derivation** (`deriveSignalDirection()`):
1. If `item.signal.direction` exists → use it (from backend MarketSignal)
2. Else derive from `snap.trend30d`: > 5% = rising, < -5% = falling, else stable

**Expanded detail row** (click to expand):

| Left column | Right column |
|-------------|-------------|
| Per-source prices with trend, confidence, ranges | P10–P90 extended range |
| 7-day averages per source | Volatility indicator |
| Last sold price + date | 90-day trend |
| Lowest listing + active count | 90-day sales count |
| | Est. P/L |
| | Full recommendation text |

**Mobile card**: Compact grid with signal badge, confidence dots inline with market price, cost, range, last sold, low list, velocity, P/L.

**Summary bar**: Count, total cost, total market value, total P/L with ROI %.

**Market cell tooltip**: Full P10–P90 range, last sold + date, 7-day avg, lowest listing + active count, velocity, trend %, P/L, all per-source breakdowns.

### Sell Sheet (`SellSheetView.tsx`)

Buyer-facing — hides internal cost data, shows market justification.

**Columns**:

| Column | Data | Purpose |
|--------|------|---------|
| Card, Cert, Gr | Identity | — |
| Asking Price | `targetSellPrice` | What the seller wants |
| Market Value | `bestPrice(currentMarket)` | Validates the ask |
| Range | Conservative–Optimistic | Shows price spread |
| Trend | Rising/Falling/Stable badge | Creates urgency if rising |
| Demand | Velocity text | Proves market activity |
| Market Data | Per-source prices + last sold | Multiple data points as evidence |

**Removed from sell sheet** (internal only):
- Cost basis, minimum accept price, P/L (reveals negotiation floor)
- Lowest listing + active listings (reveals competitive floor)
- Per-source confidence dots (internal quality metric)
- Per-source min/max ranges (reveals floor prices)

**Summary cards**: Items count, Total Market Value, Data Sources count.

**Footer**: "Prices based on recent completed sales from multiple verified sources including PriceCharting and PokemonPrice."

### Other Components Using Price Data

| Component | What it shows |
|-----------|--------------|
| `PurchasesTab.tsx` | Snapshot-at-purchase prices |
| `SalesTab.tsx` | Sale prices, P/L per sale |
| `AnalyticsTab.tsx` | Aggregate P&L, ROI, channel breakdown |
| `TuningTab.tsx` | Grade/tier performance, market alignment |
| `OverviewTab.tsx` | Summary stats with market values |
| `QuickAddSection.tsx` | Live cert lookup with market data |
| `PricingPage.tsx` | Price lookup tool with per-source display |
| `CardPriceCard.tsx` | Individual card price display |
| `CampaignsPage.tsx` | Campaign-level P&L badges |

---

## Key Files

| Aspect | Path |
|--------|------|
| PriceCharting client | `internal/adapters/clients/pricecharting/` |
| PokemonPrice client | `internal/adapters/clients/pokemonprice/` |
| CardHedger client | `internal/adapters/clients/cardhedger/` |
| Source adapters | `internal/adapters/clients/fusionprice/source_adapters.go` |
| Fusion engine | `internal/domain/fusion/engine.go` |
| Fusion provider | `internal/adapters/clients/fusionprice/fusion_provider.go` |
| Source fetcher | `internal/adapters/clients/fusionprice/source_fetcher.go` |
| Cache/persistence | `internal/adapters/clients/fusionprice/freshness.go` |
| Price merger | `internal/adapters/clients/fusionprice/price_merger.go` |
| MarketSnapshot builder | `internal/adapters/clients/pricelookup/adapter.go` |
| Domain pricing types | `internal/domain/pricing/provider.go` |
| Campaign types | `internal/domain/campaigns/types.go` |
| Frontend types | `web/src/types/campaigns.ts` |
| Inventory UI | `web/src/react/pages/campaign-detail/InventoryTab.tsx` |
| Sell Sheet UI | `web/src/react/pages/campaign-detail/SellSheetView.tsx` |
| Signal/format helpers | `web/src/react/utils/formatters.ts` |
| Confidence dots | `web/src/react/ui/ConfidenceIndicator.tsx` |
| Batch scheduler | `internal/adapters/scheduler/cardhedger_batch.go` |
| Delta poll scheduler | `internal/adapters/scheduler/cardhedger_refresh.go` |
| Statistics/percentiles | `internal/domain/pricing/analysis/statistics.go` |

---

## Historical Bugs & Fixes

1. **Inventory showing no data**: `GetInventoryAging` had `p.CLValueCents > 0` guard — removed since CL$ input was removed from purchase flow.

2. **Still no data**: Inner guard `snap.LastSoldCents > 0` was too restrictive for cards where PriceCharting has grade price but no per-grade last-sold. Changed to `snap.LastSoldCents > 0 || snap.MedianCents > 0 || snap.GradePriceCents > 0`.

3. **PokemonPrice set mismatch**: PSA category format vs PokemonPrice naming. Fixed with `cardutil.NormalizeSetNameSimple()` + retry-without-set fallback.

4. **PriceCharting and PokemonPrice showing identical prices**: `buildSourcePrices()` was using the fused `price.Grades` for the "PriceCharting" entry, but PriceCharting prices aren't in the fusion engine — so both entries showed PokemonPrice's price. Fixed by adding `PCGrades` field to carry PriceCharting's raw grade prices separately.

5. **CardHedger data never appearing in sourcePrices**: On-demand calls skip CardHedger (by design), and when DB-cached data was loaded, `convertEntryToPrice()` never populated `GradeDetails` with `EstimateGradeDetail`. Fixed by adding `supplementCardHedgerFromDB()` to reconstruct from stored batch data.

6. **`LastSoldCents = 0` for all 21 cards**: PriceCharting's `sales-data` API field is never populated ("Historic prices and historic sales are not supported"). `LastSoldByGrade` was always nil → `LastSoldCents` always 0. Fixed with a fallback chain (see [LastSold Fallback Chain](#lastsold-fallback-chain)).

7. **CardHedger set matching too strict**: `resolveCardID()` used `strings.EqualFold()` for exact set name match. CardHedger returns decorated names like `"2021 Pokemon Celebrations Classic Collection"` while purchases have `"CELEBRATIONS CLASSIC COLLECTION"`. Fixed by switching to `cardutil.MatchesSetOverlap()` (token overlap, same as PokemonPrice).

8. **Card name suffixes break DB/cache lookups**: PSA-style names like `"DARK GYARADOS-HOLO"` or `"MEWTWO-REV.FOIL"` store data under normalized name (`"DARK GYARADOS Holo"`) but subsequent lookups use the original name → cache miss. Fixed by applying `cardutil.NormalizePurchaseName()` at the start of `GetPrice()`.

9. **"GAME" not recognized as generic set name**: `"GAME"` passed `IsGenericSetName()` check but is meaningless for price lookups. Added to the generic sets list.

10. **PriceCharting wrong variant matches (card number mismatch)**: `tryAPI()` didn't validate card numbers. `Pikachu #002` matched `Pikachu [Holo] #5` (wrong card entirely). Fixed by adding `VerifyProductMatch()` check in `tryAPI()`.

11. **Inventory page making live API calls on every load**: `enrichAgingItem` was calling `batchFetchSnapshots()` which made N concurrent `GetMarketSnapshot` calls (each potentially hitting PriceCharting + PokemonPrice APIs). Fixed by reverting to stored snapshot data only — the inventory refresh scheduler (runs on startup + hourly) keeps snapshots fresh. Full `MarketSnapshot` now persisted as JSON in `snapshot_json` DB column.

12. **`EnrichWithHistoricalData` removed**: PriceCharting's `/api/product/history` endpoint returns 404. The no-op method and all callers were removed.

13. **CardHedger `details-by-certs` parsing empty**: `CertDetailResult` used flat JSON tags (`json:"cert"`, `json:"card_id"`) but the real API returns nested `cert_info` + `card` objects. All fields parsed as zero values, so zero card_id mappings were saved post-import. Fixed by restructuring into `CertInfo` (cert, grade, description) + `*CardDetail` (card_id, player, set, number, etc.). The `Card` pointer is nil when the cert's card isn't in CardHedger's DB.

---

## LastSold Fallback Chain

`pricelookup/adapter.go` `GetMarketSnapshot()` populates `LastSoldCents` via a 4-level fallback:

1. **`LastSoldByGrade`** — actual per-grade last sold data (PriceCharting `sales-data`). **Never populated** because PriceCharting's API doesn't return it.
2. **`PCGrades`** — PriceCharting grade prices (`manual-only-price` = PSA 10, `graded-price` = PSA 9, `loose-price` = Raw). These are based on recent eBay sold listings and serve as effective "last sold" proxies.
3. **`GradeDetails[grade].Ebay.PriceCents`** — PokemonPrice `smartMarketPrice` (median of recent eBay sales). Also sets `SaleCount`.
4. **`GradeDetails[grade].Estimate.PriceCents`** — CardHedger price estimate.

This ensures every card with **any** pricing data shows a non-zero `LastSoldCents`.

---

## Inventory Snapshot Persistence

### Problem

`MarketSnapshotData` (stored on purchases) only has 8 fields: `LastSoldCents`, `LowestListCents`, `ConservativeCents`, `MedianCents`, `ActiveListings`, `SalesLast30d`, `Trend30d`, `SnapshotDate`. The frontend uses many more fields (`SourcePrices`, `GradePriceCents`, velocity, percentiles, etc.) that were lost between scheduler refresh and page load.

### Solution

A `snapshot_json` TEXT column stores the full `MarketSnapshot` as JSON. The individual columns remain for SQL queries/sorting. On read, `snapshotFromPurchase()` deserializes the JSON blob to get all fields. On write, `applySnapshot()` serializes the full snapshot.

The inventory refresh scheduler (runs on startup + hourly) calls `RefreshPurchaseSnapshot()` which calls `GetMarketSnapshot()` → populates both individual columns and `snapshot_json`. Page loads are instant — zero API calls.

### Card Name Normalization

`cardutil.NormalizePurchaseName()` handles PSA-style purchase names before DB/cache lookups:

| Input | Output |
|-------|--------|
| `DARK GYARADOS-HOLO` | `DARK GYARADOS Holo` |
| `MEWTWO-REV.FOIL` | `MEWTWO Reverse Foil` |
| `SNORLAX-REV.HOLO` | `SNORLAX Reverse Holo` |
| `Charizard ex #161` | `Charizard ex` |

Applied in `FusionPriceProvider.GetPrice()` so all cache keys and DB queries use the normalized form.
