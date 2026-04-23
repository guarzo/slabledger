# API Cheatsheet

Look-up reference for writing curls against the SlabLedger API. Read this when you need to parse a response or remember which JSON key a concept lives under.

## Parsing responses

Pipe every curl through `python3 -c` or `jq` and project only the fields you'll cite. Never paste raw JSON into the response — large endpoints (snapshot, inventory) return multi-KB payloads that bury the signal.

```bash
# cents → dollars
jq '.amountCents / 100'

# drop archived campaigns
jq 'map(select(.phase != "archived"))'

# trim a campaign list to the fields we actually cite
jq '[.[] | {id, name, phase, buyTermsCLPct, dailySpendCapCents}]'

# weekly-review: extract week-over-week deltas
jq '{purchases: [.purchasesThisWeek, .purchasesLastWeek],
     spend: [(.spendThisWeekCents/100), (.spendLastWeekCents/100)],
     sales: [.salesThisWeek, .salesLastWeek],
     profit: [(.profitThisWeekCents/100), (.profitLastWeekCents/100)],
     topPerformers: [.topPerformers[] | {cardName, profitCents, channel, daysToSell}]}'
```

## /portfolio/snapshot — composite endpoint

**This is the primary data source.** A single `GET /api/portfolio/snapshot` returns everything under one JSON object. The top-level keys are:

```
health, insights, weeklyReview, weeklyHistory, channelVelocity, suggestions, creditSummary, invoices
```

### snapshot.health

**Structure:** `health` is a dict (NOT a direct array). Top-level fields:

| Key | Type | Notes |
|-----|------|-------|
| `campaigns` | array | Per-campaign health entries (see below) |
| `totalDeployedCents` | int | Portfolio-wide total spend |
| `totalRecoveredCents` | int | Portfolio-wide total revenue |
| `totalAtRiskCents` | int | Portfolio-wide unsold capital |
| `overallROI` | float | Portfolio ROI (decimal, 0.05 = 5%) |
| `realizedROI` | float | Realized ROI on sold items only |

Each entry in `campaigns` array:

| Key | Type | Notes |
|-----|------|-------|
| `campaignId` | string UUID | |
| `campaignName` | string | |
| `kind` | string | `"standard"` or `"external"` — filter external from all calculations |
| `phase` | string | `"active"`, `"paused"`, `"archived"` |
| `roi` | float | Decimal (0.05 = 5%) |
| `sellThroughPct` | float | Decimal (0.5 = 50%) |
| `avgDaysToSell` | float | |
| `totalPurchases` | int | |
| `totalUnsold` | int | |
| `capitalAtRiskCents` | int | Total unsold capital (in-hand + in-transit) |
| `inHandUnsoldCount` | int | Received, sellable now |
| `inHandCapitalCents` | int | |
| `inTransitUnsoldCount` | int | Purchased but not yet received |
| `inTransitCapitalCents` | int | |
| `healthStatus` | string | `"healthy"`, `"warning"`, `"critical"` |
| `healthReason` | string | Human-readable explanation |

### snapshot.weeklyReview

A single `WeeklyReviewSummary` for the current week:

- `weekStart`, `weekEnd` — date range (YYYY-MM-DD)
- `purchasesThisWeek` / `purchasesLastWeek` — purchase counts
- `spendThisWeekCents` / `spendLastWeekCents` — total spend
- `salesThisWeek` / `salesLastWeek` — sale counts
- `revenueThisWeekCents` / `revenueLastWeekCents` — gross revenue
- `profitThisWeekCents` / `profitLastWeekCents` — net profit
- `byChannel` — array of `{channel, saleCount, revenueCents, feesCents, netProfitCents, avgDaysToSell}`
- `weeksToCover` — capital deployment estimate
- `daysIntoWeek` — how many days into the current week (critical for partial-week awareness)
- `topPerformers` / `bottomPerformers` — arrays of `{cardName, certNumber, grade, profitCents, channel, daysToSell}`

### snapshot.weeklyHistory

**Array of `WeeklyReviewSummary` objects, newest first.** Each entry has the SAME field names as `weeklyReview` — specifically `purchasesThisWeek`, `spendThisWeekCents`, `salesThisWeek`, `revenueThisWeekCents`, `profitThisWeekCents` (NOT `purchases`, `spendCents`, `sales`, `revenueCents`). The `*LastWeek` fields in each entry refer to that entry's prior week.

```bash
# Trailing 4-week mean for hold-verdict rule
jq '.weeklyHistory[0:4] | (map(.profitThisWeekCents) | add / length / 100)'

# Weekly purchase/spend/sales/revenue table
jq '.weeklyHistory[] | {weekEnd, purchases: .purchasesThisWeek,
     spend: (.spendThisWeekCents/100), sales: .salesThisWeek,
     revenue: (.revenueThisWeekCents/100)}'
```

### snapshot.insights

Contains segment breakdowns. **All segments use a uniform shape** with these common fields:

| Key | Type | Notes |
|-----|------|-------|
| `label` | string | The segment name: `"PSA 9"`, `"Charizard"`, `"Gengar PSA 6"`, `"$200-500"`, etc. |
| `dimension` | string | `"grade"`, `"character"`, `"characterGrade"`, `"priceTier"`, `"era"`, `"channel"` |
| `purchaseCount` | int | |
| `soldCount` | int | |
| `sellThroughPct` | float | Decimal |
| `avgDaysToSell` | float | |
| `totalSpendCents` | int | |
| `totalRevenueCents` | int | |
| `totalFeesCents` | int | |
| `netProfitCents` | int | |
| `roi` | float | Decimal (1.34 = 134% ROI) |
| `avgBuyPctOfCL` | float | Decimal (0.50 = 50% of CL value) |
| `avgMarginPct` | float | Decimal |
| `bestChannel` | string | |
| `campaignCount` | int | How many campaigns contributed |
| `latestSaleDate` | string | YYYY-MM-DD |

**IMPORTANT:** There is no `character`, `grade`, `gradeValue`, or `tier` field — use `label` for all segment names. Parse grade from the label string when needed (e.g., `"PSA 9"` → grade 9).

Sub-keys of `insights`:
- `byCharacter` — array; includes an `"Other"` bucket for uncategorized characters
- `byGrade` — array
- `byEra` — array
- `byPriceTier` — array; labels like `"$0-50"`, `"$100-200"`, `"$500+"`
- `byChannel` — array
- `byCharacterGrade` — array; label format is `"Character PSA N"` (e.g., `"Gengar PSA 6"`)
- `coverageGaps` — array of `{segment: <segment-object>, reason: string, opportunity: string}`
- `campaignMetrics` — per-campaign metrics
- `dataSummary` — `{totalPurchases, totalSales, campaignsAnalyzed, dateRange, overallROI}`

```bash
# byCharacter — top characters by ROI, soldCount ≥ 3
jq '.insights.byCharacter | map(select(.soldCount >= 3 and .label != "Other"))
    | sort_by(-.roi) | .[]
    | {name: .label, n: .purchaseCount, sold: .soldCount, st: .sellThroughPct,
       roi, avgBuyPctCL: .avgBuyPctOfCL, bestChannel}'

# byGrade — portfolio-wide grade exposure
jq '.insights.byGrade | map({grade: .label, n: .purchaseCount, sold: .soldCount,
                    st: .sellThroughPct, roi, campaigns: .campaignCount})'

# byCharacterGrade — top standouts (n ≥ 3)
jq '.insights.byCharacterGrade | map(select(.purchaseCount >= 3))
    | sort_by(-.roi) | .[0:20]
    | .[] | {name: .label, n: .purchaseCount, sold: .soldCount, roi}'

# coverageGaps
jq '.insights.coverageGaps[] | {name: .segment.label, roi: .segment.roi,
                       sold: .segment.soldCount, reason, opportunity}'
```

### snapshot.suggestions

**Structure:** `suggestions` is a dict (NOT an array). Keys:

| Key | Type | Notes |
|-----|------|-------|
| `newCampaigns` | array | Each: `{type: "new", title, rationale, ...}` |
| `adjustments` | array | Each: `{type: "adjust", title, rationale, ...}` |
| `dataSummary` | object | `{totalPurchases, totalSales, campaignsAnalyzed, dateRange, overallROI}` |

### snapshot.creditSummary

| Key | Type | Notes |
|-----|------|-------|
| `outstandingCents` | int | Total unpaid balance |
| `weeksToCover` | float | Weeks of revenue needed to cover outstanding |
| `recoveryTrend` | string | `"improving"`, `"declining"`, `"flat"` |
| `alertLevel` | string | `"ok"`, `"warning"`, `"critical"` |

### snapshot.invoices

Array of invoice objects:

| Key | Type | Notes |
|-----|------|-------|
| `dueDate` | string | YYYY-MM-DD (may be empty for legacy rows) |
| `totalCents` | int | Invoice amount |
| `paid` | bool | |

### snapshot.channelVelocity

Array of `{channel, avgDaysToSell, count}`.

## Key JSON field names (Purchase / Campaign objects)

| Field | JSON key | Notes |
|-------|----------|-------|
| Buy cost | `buyCostCents` | Use this; `purchasePriceCents` is a common mis-guess and will silently return null |
| Grade | `gradeValue` | Float — supports half-grades like 8.5 |
| CL value | `clValueCents` | Card Ladder value at time of purchase |
| Card name | `cardName` | Cleaned name from cert lookup |
| PSA title | `psaListingTitle` | Full PSA label text |
| Cert number | `certNumber` | PSA cert number |
| Purchase ID | `id` (on Purchase) | UUID — use this for API operations, NOT the cert number |
| Campaign ID | `id` (on Campaign) | String UUID, NOT an integer |
| Campaign last update | `updatedAt` (on Campaign) | RFC3339 — used by the stale-suggestion filter |

## /campaigns/{id}/tuning

Returns tuning data with a `byGrade` array. Each grade row:

| JSON key | Type | Notes |
|----------|------|-------|
| `grade` | string | String like `"8"`, `"8.5"`, `"9"`, `"10"` (NOT a float, NOT `gradeValue`) |
| `count` | int | Total purchases (NOT `purchaseCount`) |
| `avgBuyPctOfCL` | float | Decimal — **the** margin-leak signal |
| `roi` | float | Decimal (NOT `avgRoi`) |
| `sellThroughPct` | float | Decimal |

```bash
# Margin-leak rows: avgBuyPctOfCL ≥ 0.93 with n ≥ 10
jq '.byGrade[] | select(.avgBuyPctOfCL >= 0.93 and .count >= 10)
    | {grade, n: .count, st: .sellThroughPct, roi, avgBuyPctCL: .avgBuyPctOfCL}'
```

May also contain `byPriceTier` with rows having `tier`, `count`, `avgBuyPctOfCL`, `roi`.

## /campaigns/{id}/fill-rate

**Returns a flat array** of daily objects (NOT `{daily: [...]}`). Each entry:

| JSON key | Type | Notes |
|----------|------|-------|
| `date` | string | YYYY-MM-DD |
| `spendUSD` | float | Daily spend in dollars (NOT cents, NOT `dailySpendCents`) |
| `capUSD` | float | Daily cap in dollars (NOT cents, NOT `capCents`) |
| `fillRatePct` | float | Decimal — spend/cap ratio (NOT `fillPct`) |
| `purchaseCount` | int | Number of fills that day |

```bash
# Last fill date per campaign
jq 'map(select(.purchaseCount > 0)) | last | .date'

# Average fill utilization
jq '[.[].fillRatePct] | add / length'
```

Pegged-at-cap (median fillRatePct > 0.95) = ramp candidate. Well below cap (median < 0.4) = supply-constrained, not a tuning lever.

## /dh/status

Returns a flat object:

| Key | Notes |
|-----|-------|
| `dh_inventory_count` | Total items mapped to DH |
| `dh_listings_count` | Items currently listed |
| `pending_count` | Items awaiting push |
| `intelligence_count` | DH intelligence records |
| `suggestions_count` | DH-derived suggestions |

## /dh/pending

Returns `{items: DHPendingItem[], count: int}`. When count=0, items is empty or null.

## /intelligence/niches

Returns `{opportunities: [...]}` (NOT a flat array). Each opportunity has nested `demand` and `market` objects.

Query params: `window` (`7d` or `30d`), `limit` (1–200).

## /intelligence/campaign-signals

Returns `{computed_at, data_quality, signals: [...]}`. When empty: `signals: []`, `data_quality: "empty"`.

## /opportunities/crack and /opportunities/acquisition

Both return **flat arrays** (empty `[]` when no opportunities exist). NOT `{candidates: [...]}` or `{opportunities: [...]}`.
