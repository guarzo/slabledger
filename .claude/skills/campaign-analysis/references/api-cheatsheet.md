# API Cheatsheet

Look-up reference for writing curls against the SlabLedger API. Read this when you need to parse a response or remember which JSON key a concept lives under.

## Parsing responses

Pipe every curl through `jq` and project only the fields you'll cite. Never paste raw JSON into the response — large endpoints (weekly-review, inventory, capital-timeline) return multi-KB payloads that bury the signal.

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

## Weekly review response fields

The `/api/portfolio/weekly-review` response (`WeeklyReviewSummary`) contains:

- `weekStart`, `weekEnd` — date range (YYYY-MM-DD)
- `purchasesThisWeek` / `purchasesLastWeek` — purchase counts
- `spendThisWeekCents` / `spendLastWeekCents` — total spend
- `salesThisWeek` / `salesLastWeek` — sale counts
- `revenueThisWeekCents` / `revenueLastWeekCents` — gross revenue
- `profitThisWeekCents` / `profitLastWeekCents` — net profit
- `byChannel` — array of `{channel, saleCount, revenueCents, feesCents, netProfitCents, avgDaysToSell}`
- `weeksToCover` — capital deployment estimate
- `topPerformers` / `bottomPerformers` — arrays of `{cardName, certNumber, grade, profitCents, channel, daysToSell}`

## Key JSON field names

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
