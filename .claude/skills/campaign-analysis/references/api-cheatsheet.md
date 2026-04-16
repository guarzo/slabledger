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
| Campaign last update | `updatedAt` (on Campaign) | RFC3339 — used by the stale-suggestion filter (drop suggestions when `now - updatedAt < 72h` and suggestion targets a changed field) |

## Parsing /portfolio/insights segments

The `/api/portfolio/insights` response is large (often 75KB+). Don't pipe it raw — project the segments you'll cite.

```bash
# byCharacter — top characters by ROI, soldCount ≥ 3
jq '.byCharacter | map(select(.soldCount >= 3)) | sort_by(-.roi) | .[]
    | {character, n: .purchaseCount, sold: .soldCount, st: .sellThroughPct,
       roi, avgBuyPctCL: .avgBuyPctOfCL, bestChannel}'

# byGrade — portfolio-wide grade exposure
jq '.byGrade | map({grade: .gradeValue, n: .purchaseCount, sold: .soldCount,
                    st: .sellThroughPct, roi, campaigns: .activeCampaignCount})'

# byPriceTier — drag tiers
jq '.byPriceTier | map({tier: .label, n: .purchaseCount, st: .sellThroughPct,
                        roi, avgBuyPctCL: .avgBuyPctOfCL})'

# byCharacterGrade — top character × grade standouts (n ≥ 3)
jq '.byCharacterGrade | map(select(.purchaseCount >= 3))
    | sort_by(-.roi) | .[0:20]
    | .[] | {character, grade: .gradeValue, n: .purchaseCount,
             sold: .soldCount, roi}'

# coverageGaps — segments with no active campaign but historical ROI > 0
jq '.coverageGaps[] | {segment: .segmentLabel, roi: .historicalRoi,
                       sold: .soldCount, reason, opportunity}'
```

## /campaigns/{id}/tuning byGrade

The grade-level rows are the single highest-resolution tuning signal. Cite specific `(campaign, grade)` rows in opener and Playbook A — never just say "tuning suggests".

```bash
# Margin-leak rows: avgBuyPctOfCL ≥ 0.93 with n ≥ 10
jq '.byGrade[] | select(.avgBuyPctOfCL >= 0.93 and .purchaseCount >= 10)
    | {grade: .gradeValue, n: .purchaseCount, st: .sellThroughPct,
       roi: .avgRoi, avgBuyPctCL: .avgBuyPctOfCL, cv}'
```

| JSON key (byGrade row) | Notes |
|------------------------|-------|
| `gradeValue` | Float (8, 8.5, 9, 10) |
| `purchaseCount` | Total purchases at this grade in this campaign |
| `soldCount` | Sales completed |
| `sellThroughPct` | Decimal (0.32 = 32%) |
| `avgBuyPctOfCL` | Decimal (0.97 = 97% of CL value) — **the** margin-leak signal |
| `avgRoi` | Decimal — average ROI on sold purchases |
| `roiStddev`, `cv` | Variance — feeds the Confidence bands rule |

## /campaigns/{id}/fill-rate

```bash
# Daily spend vs cap, last 30 days
jq '.daily[] | {date, spend: (.dailySpendCents/100), cap: (.capCents/100),
                pct: .fillPct}'
```

Pegged-at-cap (median fillPct > 0.95) = ramp candidate. Well below cap (median < 0.4) = supply-constrained, not a tuning lever.

## /portfolio/weekly-history

```bash
# Trailing 4-week mean for hold-verdict rule
jq '.[0:4] | (map(.profitThisWeekCents) | add / length / 100)'
```

Returns array of `WeeklyReviewSummary` newest-first. Required `weeks` query param: positive integer, max 52.

## /portfolio/channel-velocity

```bash
jq '.[] | {channel, days: .avgDaysToSell, sales: .saleCount,
           medianNet: (.medianNetProceedsCents/100)}'
```

## /intelligence/niches

```bash
# High-opportunity, zero-coverage rows
jq '.niches[] | select(.opportunity_score >= 0.7 and .current_coverage == 0)
    | {character, era, grade, demand: .demand_score,
       opportunity: .opportunity_score, velocity: .velocity_change_pct}'
```

Query params: `window` (`7d` or `30d`), `limit` (1–200), `sort`, `era`, `grade`, `min_data_quality` (`proxy` or `full`).

## /intelligence/campaign-signals

```bash
jq '.signals[] | {campaign: .campaignName, accel: .accelerationPct,
                  status, weekOverWeekVelocity}'
```

`status` values: `accelerating`, `decelerating`, `flat`. Sharp deceleration (< -25%) on a campaign with healthy n is a tuning candidate.

## /opportunities/crack and /opportunities/acquisition

```bash
# Crack: surface when total netGain across queue exceeds ~$1K
jq '[.candidates[] | .netGainCents] | add / 100'

jq '.candidates[] | {cert: .certNumber, slabbed: (.slabbedValueCents/100),
                     raw: (.rawValueCents/100),
                     net: (.netGainCents/100), confidence}'

# Acquisition: raw → graded mispricings worth >$200
jq '.opportunities[] | select(.spreadCents >= 20000)
    | {cardName, current: (.currentMarketCents/100),
       targetGrade: .targetGradeValue,
       expected: (.expectedGradedCents/100), spread: (.spreadCents/100)}'
```
