# API Cheatsheet

Look-up reference for writing curls against the SlabLedger API. Read this when you need to parse a response or remember which JSON key a concept lives under.

## Pagination & full-population analysis

The old "endpoints silently truncate" warning is **obsolete** as of the 2026-07 overhaul. Corrected picture:

- `GET /api/campaigns/{id}/sales` — `?limit` is now **honored up to 10000** (default 50). Pass `?limit=10000` for full sale history; there is no hidden 50-cap anymore.
- `GET /api/inventory` — returns **UNSOLD-only rows BY DESIGN**. This was never a "176 cap" — the endpoint simply does not serve sold rows. Do not treat its row count as a portfolio total.
- **Full-population sold / channel / P&L analysis:** use `GET /api/portfolio/analysis` (the `pnl` split and `inScopeByGrade` are computed server-side over the full population, External excluded). For ad-hoc cross-tabs the endpoint doesn't shape, query Postgres directly.

**Postgres escape hatch** (ad-hoc full-population only):
```bash
# Connection string is in /workspace/.env
DBURL=$(grep -E "^SUPABASE_DB_URL=" /workspace/.env | cut -d= -f2- | tr -d '"')
psql "$DBURL" -c "select count(*) from campaign_sales"
```
Key tables: `campaign_sales` (purchase_id, sale_channel, sale_price_cents, net_profit_cents, sale_date, forced_liquidation), `campaign_purchases` (id, card_player ← clean character name, buy_cost_cents, cl_value_cents, cl_value_at_purchase_cents, card_year [text], grade_value). Join `campaign_sales.purchase_id = campaign_purchases.id`. `campaign_sales` includes External-import sales — exclude `campaign_id='external'` for standard-campaign economics.

**Before computing on any of this, apply ledger R-027** — model how the field is generated. The frozen-at-purchase `cl_value_at_purchase_cents` (and the `/analysis` `bpclAtBuy` built on it) is clean buy quality; the live `cl_value_cents` and any `avgBuyPctOfCL` derived from it are CL-drift artifacts; ROI/channel-mix are forced-sale-contaminated (R-025).

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

## /portfolio/analysis — the default-session endpoint

**This is the one required call for a default `/campaign-analysis` session.**

```bash
curl -s -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  "$BASE/api/portfolio/analysis?since=2026-06-22"
```

`since=YYYY-MM-DD` is the most-recent `campaign-state-log.md` date. With no prior date the endpoint returns full-history deltas capped to 90 days. **External is excluded everywhere** in this response. All money fields are cents.

Top-level shape:

| JSON key | Type | Notes |
|----------|------|-------|
| `generatedAt` | string | RFC3339 |
| `since` | string | Echo of the `since=` param (omitted if none) |
| `campaigns` | array | One `CampaignAnalysis` per campaign (below) |
| `deltas` | object | `SessionDeltas` — what changed since `since` (below) |

### `campaigns[]` — `CampaignAnalysis`

| JSON key | Type | Notes |
|----------|------|-------|
| `campaignId` | string UUID | |
| `campaignName` | string | Use on first reference (R-019) |
| `phase` | string | `"active"`, `"paused"`, `"pending"`, `"archived"` — ground truth (R-014) |
| `buyTermsCLPct` | float | Contract buy terms (PSA-side bid ceiling), decimal |
| `bpclAtBuy` | object | `BPCLStats` — clean CL-at-buy buy quality (below) |
| `pnl` | object | `SplitPNL` — discretionary vs forced (below) |
| `weeklyFill` | array | `WeeklyFill[]` — trailing 8 Monday-bucketed weeks (below) |
| `inScopeByGrade` | array | `GradeScopeRow[]` — server-side current-scope filter (below) |

#### `bpclAtBuy` — `BPCLStats` (buy-price ÷ CL-at-purchase)

| JSON key | Type | Notes |
|----------|------|-------|
| `n` | int | Purchases WITH a CL-at-buy snapshot (`clValueAtPurchaseCents > 0`) |
| `total` | int | All purchases in the campaign |
| `coveragePct` | float | `n / total * 100` — caveat thin coverage before citing |
| `dollarWeighted` | float | `sum(buyCost) / sum(clAtBuy)` over the `n` snapshot rows — **the clean buy-quality figure** |
| `meanDriftPct` | float | mean of `(clNow − clAtBuy) / clAtBuy * 100` over `n` — CL drift SINCE purchase, reported separately (drift is noise, not buy skill) |

`dollarWeighted` is the CL-at-buy replacement for the retired contaminated `avgBuyPctOfCL`. Pair it with `buyTermsCLPct` when you cite it (hard constraint 3).

#### `pnl` — `SplitPNL` (realized, split by the sale's `forcedLiquidation` flag)

`pnl.discretionary` and `pnl.forced` each hold a `PNLBlock`:

| JSON key | Type | Notes |
|----------|------|-------|
| `soldCount` | int | |
| `revenueCents` | int | |
| `netProfitCents` | int | |
| `roiPct` | float | `netProfit / (revenue − netProfit) * 100`; 0 when cost basis is 0 |

Forced = invoice-driven liquidation (see `forcedLiquidation` in field-semantics). Compare the two blocks to see how much "bad ROI" is really forced-sale drag, not buy quality.

#### `weeklyFill[]` — `WeeklyFill` (trailing 8 weeks)

| JSON key | Type | Notes |
|----------|------|-------|
| `weekStart` | string | Monday, YYYY-MM-DD |
| `fills` | int | Purchase count that week |
| `spendCents` | int | |
| `capCents` | int | `dailySpendCapCents × 7` |
| `utilizationPct` | float | `spend / cap * 100`; 0 when cap is 0 — **the fill-rate-vs-plan signal for the R-001 gate** |

#### `inScopeByGrade[]` — `GradeScopeRow` (current-scope filtered server-side)

| JSON key | Type | Notes |
|----------|------|-------|
| `grade` | float | PSA grade (supports half-grades) |
| `n` | int | In-scope purchases at this grade |
| `dollarWeightedBpclAtBuy` | float | CL-at-buy buy quality for the grade |
| `soldCount` | int | Discretionary sales only |
| `netProfitCents` | int | Discretionary sales only |

Rows are already filtered to the campaign's current grade / price / year / inclusion scope — no manual currentScope filter needed (that ritual is retired).

### `deltas` — `SessionDeltas`

| JSON key | Type | Notes |
|----------|------|-------|
| `newPurchases` | int | Fills since `since` |
| `newPurchaseCents` | int | |
| `newSales` | int | |
| `newSaleCents` | int | |
| `campaignsUpdated` | array | Campaign **names** with `updatedAt > since` (drives the fresh-re-enable premise check) |
| `invoices` | array | `InvoiceSummary[]` with `invoiceDate` ≥ `since` |

`invoices[]`: `{invoiceDate, dueDate, totalCents, status}` (dates YYYY-MM-DD). Two entries within a 5-day window feed the twin-invoice premise check.

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

**Note: `snapshot.invoices` vs `/api/credit/invoices`.** Playbook B fetches `/api/credit/invoices` directly and uses enriched fields (`pendingReceiptCents`, `sellThroughPct`, `soldCount`, `totalCount`). It is unverified whether `snapshot.invoices` carries those same enrichments or only the base `{dueDate, totalCents, paid}` shown above. Default behavior: use `snapshot.invoices` for the opener's "upcoming invoices" line (only `dueDate` and `totalCents` are needed there) and keep Playbook B's separate `/credit/invoices` fetch for liquidation planning. A future session with live API access should diff the two shapes and consolidate if possible.

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

## /opportunities/acquisition

Returns a **flat array** (empty `[]` when no opportunities exist). NOT `{candidates: [...]}` or `{opportunities: [...]}`.
