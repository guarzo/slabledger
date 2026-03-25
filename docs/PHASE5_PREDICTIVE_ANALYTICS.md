# Phase 5: Predictive Analytics — Implementation Plan

## Prerequisites

Phase 5 depends on **30-60 days of collected data** from the Phase 3 data pipeline:

- `market_snapshot_history` — daily price/velocity/trend snapshots per card/grade (via `SnapshotHistoryScheduler`)
- `cl_value_history` — CL value observations per cert (via CL CSV import hooks)
- `population_history` — pop count observations per card/grade (via CL CSV import hooks)
- `price_history` — per-source daily prices (already collected by `PriceRefreshScheduler`)

**Minimum viable data**: 30 days of `market_snapshot_history` for trajectory analysis. CL and population histories accumulate more slowly (one observation per CSV import cycle, typically biweekly).

---

## Feature 24: Price Trajectory Analysis

### What It Does

Computes price direction, momentum, and projected values for each unsold card using the `market_snapshot_history` time series. Replaces the current point-in-time `trend_30d` (a single number from a pricing source) with a locally computed trajectory based on actual daily observations.

### Data Source

```sql
SELECT snapshot_date, median_cents, conservative_cents, optimistic_cents,
       active_listings, sales_last_30d, daily_velocity
FROM market_snapshot_history
WHERE card_name = ? AND set_name = ? AND card_number = ? AND grade_value = ?
ORDER BY snapshot_date ASC
```

### Computations

For each unsold card with >= 14 days of history:

1. **7-day moving average** of `median_cents` — smooths daily noise
2. **Linear regression slope** over the last 30 days — direction and magnitude
3. **Acceleration** — change in slope between the first and second halves of the window (is momentum building or fading?)
4. **Projected price** — extrapolate the regression 7/14/30 days forward (with confidence interval based on residual variance)
5. **Volatility band** — rolling standard deviation of daily price changes, expressed as a percentage
6. **Liquidity trend** — linear regression of `sales_last_30d` over time (is this card getting harder or easier to sell?)
7. **Listing competition trend** — regression of `active_listings` over time (rising = more sellers = price pressure)

### Domain Types

```go
// internal/domain/campaigns/trajectory.go

type PriceTrajectory struct {
    CardName         string  `json:"cardName"`
    SetName          string  `json:"setName"`
    CardNumber       string  `json:"cardNumber"`
    GradeValue       float64 `json:"gradeValue"`
    DataPoints       int     `json:"dataPoints"`       // number of daily snapshots

    // Current state
    CurrentMedianCents int  `json:"currentMedianCents"`
    MovingAvg7d       int   `json:"movingAvg7dCents"`

    // Direction
    SlopeCentsPerDay  float64 `json:"slopeCentsPerDay"`  // positive = appreciating
    Direction         string  `json:"direction"`          // "rising", "falling", "stable"
    Acceleration      string  `json:"acceleration"`       // "accelerating", "decelerating", "steady"

    // Projections
    Projected7dCents  int     `json:"projected7dCents"`
    Projected14dCents int     `json:"projected14dCents"`
    Projected30dCents int     `json:"projected30dCents"`
    ConfidencePct     float64 `json:"confidencePct"`      // R² of the regression

    // Volatility
    VolatilityPct     float64 `json:"volatilityPct"`      // daily price change std dev as % of mean
    VolatilityBand    [2]int  `json:"volatilityBand"`     // [low, high] 1-sigma band

    // Market dynamics
    LiquidityTrend    string  `json:"liquidityTrend"`     // "improving", "declining", "stable"
    CompetitionTrend  string  `json:"competitionTrend"`   // "increasing", "decreasing", "stable"
}

type TrajectoryPortfolio struct {
    Items             []PriceTrajectory `json:"items"`
    RisingCount       int               `json:"risingCount"`
    FallingCount      int               `json:"fallingCount"`
    StableCount       int               `json:"stableCount"`
    InsufficientData  int               `json:"insufficientData"`
    AvgSlopePctPerDay float64           `json:"avgSlopePctPerDay"`
}
```

### Implementation Files

| File | Purpose |
|------|---------|
| `internal/domain/campaigns/trajectory.go` | Types + pure computation functions |
| `internal/domain/campaigns/service_trajectory.go` | `GetPriceTrajectories(ctx, campaignID) (*TrajectoryPortfolio, error)` |
| `internal/domain/campaigns/repository.go` | Add `GetSnapshotHistory(ctx, card, grade, since) ([]SnapshotHistoryEntry, error)` |
| `internal/adapters/storage/sqlite/history_repository.go` | SQLite query implementation |
| `internal/adapters/advisortool/executor.go` | Add `get_price_trajectories` tool |
| `internal/adapters/httpserver/handlers/campaigns_analytics.go` | Add `GET /api/campaigns/{id}/trajectories` endpoint |

### Frontend Integration

- **TuningTab**: New "Price Trajectories" section showing a table of cards with direction arrows, projected prices, and volatility indicators
- **Inventory page**: Add trajectory badges (↑/↓/→) next to each card's market signal
- **AI advisor**: The `get_price_trajectories` tool lets the LLM reference trajectory data in its analyses

### Thresholds

```
Direction:
  rising   = slope > +0.5% of mean per day
  falling  = slope < -0.5% of mean per day
  stable   = between -0.5% and +0.5%

Acceleration:
  accelerating  = second-half slope > first-half slope by > 20%
  decelerating  = second-half slope < first-half slope by > 20%
  steady        = within 20%

Liquidity/Competition:
  Same thresholds applied to sales_last_30d and active_listings regressions
```

---

## Feature 25: CL Accuracy Tracking

### What It Does

Answers the question: **"Is Card Ladder consistently overvaluing or undervaluing certain segments?"** This directly impacts buy terms optimization — if CL overvalues EX-era PSA 9s by 15%, then 80% buy terms is effectively 94% of true market value, leaving only ~6% margin before fees.

### Data Sources

1. `cl_value_history` — CL values per cert over time
2. `campaign_purchases` — actual buy cost (= CL × buyTermsPct + $3)
3. `campaign_sales` — actual sale price (= true market realization)
4. `market_snapshot_history` — independent market price observations

### Computations

For each segment (by era, grade, character, or campaign):

1. **CL vs Sale Price**: For sold cards, compute `(salePriceCents - clValueAtPurchase) / clValueAtPurchase`. Positive = CL undervalued, negative = CL overvalued.
2. **CL vs Market Median**: For unsold cards, compute `(currentMedianCents - clValueCents) / clValueCents` using the latest snapshot.
3. **CL Drift Rate**: Using `cl_value_history`, compute how much CL values have changed over time for a segment. Fast-rising CL = potential overvaluation building.
4. **Segment Aggregation**: Average the CL deviation across all cards in a segment, weighted by cost basis.

### Domain Types

```go
// internal/domain/campaigns/cl_accuracy.go

type CLAccuracySegment struct {
    SegmentLabel     string  `json:"segmentLabel"`     // e.g. "EX-Era PSA 9", "Vintage Charizard"
    SegmentDimension string  `json:"segmentDimension"` // "era", "grade", "character", "campaign"
    CardCount        int     `json:"cardCount"`
    SoldCount        int     `json:"soldCount"`

    // CL vs actual market
    AvgCLDeviationPct    float64 `json:"avgClDeviationPct"`    // positive = CL undervalues
    MedianCLDeviationPct float64 `json:"medianClDeviationPct"`
    WorstOvervaluePct    float64 `json:"worstOvervaluePct"`    // most CL overvalued (most negative)
    BestUndervaluePct    float64 `json:"bestUndervaluePct"`    // most CL undervalued (most positive)

    // CL trend
    CLDriftPctPerMonth float64 `json:"clDriftPctPerMonth"` // how fast CL values are changing
    CLDriftDirection   string  `json:"clDriftDirection"`   // "rising", "falling", "stable"

    // Effective margin impact
    EffectiveMarginPct float64 `json:"effectiveMarginPct"` // actual margin after CL deviation
    TheoreticalMargin  float64 `json:"theoreticalMargin"`  // margin if CL were perfectly accurate
    MarginLeakPct      float64 `json:"marginLeakPct"`      // theoretical - effective

    // Recommendation
    Signal    string `json:"signal"`    // "accurate", "overvalues", "undervalues"
    Reasoning string `json:"reasoning"`
}

type CLAccuracyReport struct {
    ByEra       []CLAccuracySegment `json:"byEra"`
    ByGrade     []CLAccuracySegment `json:"byGrade"`
    ByCampaign  []CLAccuracySegment `json:"byCampaign"`
    OverallPct  float64             `json:"overallDeviationPct"`
    DataPoints  int                 `json:"dataPoints"`
    MinHistory  int                 `json:"minHistoryDays"` // days of CL history available
}
```

### Implementation Files

| File | Purpose |
|------|---------|
| `internal/domain/campaigns/cl_accuracy.go` | Types + computation |
| `internal/domain/campaigns/service_cl_accuracy.go` | `GetCLAccuracyReport(ctx) (*CLAccuracyReport, error)` |
| `internal/domain/campaigns/repository.go` | Add `GetCLValueHistory(ctx, certNumber, since) ([]CLValueEntry, error)` |
| `internal/adapters/storage/sqlite/history_repository.go` | SQLite query |
| `internal/adapters/advisortool/executor.go` | Add `get_cl_accuracy` tool |
| `internal/adapters/httpserver/handlers/campaigns_analytics.go` | Add `GET /api/portfolio/cl-accuracy` endpoint |

### Frontend Integration

- **Insights page**: New "CL Accuracy" tab in portfolio insights, showing segment table with deviation indicators
- **Campaign Tuning tab**: CL accuracy callout for the specific campaign's segment
- **AI advisor**: The `get_cl_accuracy` tool lets the LLM recommend buy terms adjustments based on CL bias

### Business Impact

If the CL accuracy analysis reveals that EX-era PSA 9s are consistently overvalued by 10%, the AI advisor can recommend:
- Lowering buy terms from 80% to 72% for that segment (capturing the real 7.65% margin)
- Or shifting capital to segments where CL undervalues (vintage PSA 10s, for example)

---

## Feature 26: Supply Pressure Signals

### What It Does

Tracks PSA population changes over time to detect supply pressure. A card whose pop is growing rapidly (more graded copies entering the market) faces downward price pressure, while a stable pop with rising demand (velocity) indicates scarcity-driven appreciation.

### Data Source

```sql
SELECT observation_date, population, pop_higher
FROM population_history
WHERE card_name = ? AND set_name = ? AND card_number = ? AND grade_value = ?
ORDER BY observation_date ASC
```

### Computations

For each card/grade with >= 2 population observations:

1. **Pop Growth Rate**: `(latest_pop - earliest_pop) / earliest_pop / months_elapsed`
2. **Pop Higher Ratio**: `pop_higher / population` — what fraction of graded copies are higher grade (scarcity indicator)
3. **Supply-Demand Score**: Combine pop growth rate (supply) with velocity trend from `market_snapshot_history` (demand). Score = demand_growth - supply_growth.
4. **Scarcity Alert**: Flag cards where `pop_higher / population < 0.1` (fewer than 10% graded higher — true scarcity at this grade)

### Domain Types

```go
// internal/domain/campaigns/supply_pressure.go

type SupplyPressure struct {
    CardName          string  `json:"cardName"`
    SetName           string  `json:"setName"`
    CardNumber        string  `json:"cardNumber"`
    GradeValue        float64 `json:"gradeValue"`

    CurrentPop        int     `json:"currentPop"`
    PopHigher         int     `json:"popHigher"`
    PopHigherRatio    float64 `json:"popHigherRatio"`    // popHigher / currentPop

    PopGrowthPctMonth float64 `json:"popGrowthPctMonth"` // monthly pop growth rate
    PopObservations   int     `json:"popObservations"`   // number of data points

    VelocityTrend     string  `json:"velocityTrend"`     // from trajectory analysis
    SupplyDemandScore float64 `json:"supplyDemandScore"` // positive = demand outpacing supply

    Signal            string  `json:"signal"`            // "scarce", "balanced", "oversupplied"
    Reasoning         string  `json:"reasoning"`
}
```

### Implementation Files

| File | Purpose |
|------|---------|
| `internal/domain/campaigns/supply_pressure.go` | Types + computation |
| `internal/domain/campaigns/service_supply.go` | `GetSupplyPressure(ctx, campaignID) ([]SupplyPressure, error)` |
| `internal/domain/campaigns/repository.go` | Add `GetPopulationHistory(ctx, card, grade, since) ([]PopulationEntry, error)` |
| `internal/adapters/storage/sqlite/history_repository.go` | SQLite query |
| `internal/adapters/advisortool/executor.go` | Add `get_supply_pressure` tool |

### Frontend Integration

- **Inventory page**: Supply pressure badge per card (scarce/balanced/oversupplied)
- **AI advisor**: The `get_supply_pressure` tool informs hold vs. sell recommendations

### Limitations

Population data comes from CL CSV imports (typically biweekly). The analysis is coarse-grained compared to daily price data. Cards not imported via CL will have no population history. Consider adding PSA cert lookups to the `InventoryRefreshScheduler` in the future to get more frequent pop observations.

---

## Feature 27: AI-Generated Market Event Detection

### What It Does

Automatically detects abnormal price or volume movements across the portfolio and generates natural-language event descriptions. Replaces the manual `market_events` table with an automated detection system.

### Data Source

`market_snapshot_history` — the daily archive. Look for:

1. **Price spikes/drops**: Day-over-day median change exceeding 2 standard deviations from the card's historical daily change distribution
2. **Volume anomalies**: `sales_last_30d` change exceeding 2 standard deviations
3. **Listing surges**: `active_listings` jump exceeding 2 standard deviations
4. **Correlated movements**: Multiple cards in the same set or character moving in the same direction simultaneously (set-wide event)

### Approach

This feature uses the AI advisor (GPT-5.4) rather than hand-coded rules. The implementation adds a new analysis endpoint that:

1. Queries `market_snapshot_history` for all cards over the last 7 days
2. Computes z-scores for daily changes in median price, sales volume, and active listings
3. Filters to anomalies exceeding the threshold (|z| > 2.0)
4. Passes the anomaly list to the LLM with a prompt asking it to:
   - Group related anomalies (same set, same character, same era)
   - Hypothesize the cause (set release, influencer, seasonal, reprinting)
   - Assess whether the movement is likely temporary or structural
   - Recommend actions (sell into the spike, buy the dip, hold through noise)

### Implementation

This is primarily a new AI advisor prompt + a supporting analytics query. No new domain types needed beyond the anomaly detection.

**New files:**

| File | Purpose |
|------|---------|
| `internal/domain/campaigns/anomaly.go` | `AnomalyDetector` — computes z-scores from snapshot history |
| `internal/domain/campaigns/service_anomaly.go` | `DetectMarketAnomalies(ctx, days) ([]MarketAnomaly, error)` |
| `internal/domain/campaigns/repository.go` | Add `GetRecentSnapshotHistory(ctx, days) ([]SnapshotHistoryEntry, error)` |
| `internal/adapters/advisortool/executor.go` | Add `detect_anomalies` tool |
| `internal/domain/advisor/prompts.go` | Add `anomalyAnalysisSystemPrompt` and `anomalyAnalysisUserPrompt` |
| `internal/adapters/httpserver/handlers/advisor.go` | Add `POST /api/advisor/market-events` endpoint |

**New domain types:**

```go
type MarketAnomaly struct {
    CardName       string  `json:"cardName"`
    SetName        string  `json:"setName"`
    CardNumber     string  `json:"cardNumber"`
    GradeValue     float64 `json:"gradeValue"`
    AnomalyType    string  `json:"anomalyType"`    // "price_spike", "price_drop", "volume_surge", "listing_surge"
    Date           string  `json:"date"`
    ZScore         float64 `json:"zScore"`
    PreviousValue  int     `json:"previousValue"`
    CurrentValue   int     `json:"currentValue"`
    ChangePct      float64 `json:"changePct"`
}
```

### Frontend Integration

- **Dashboard**: New "Market Events" card showing AI-generated event summaries from the past 7 days
- **Inventory page**: Anomaly badge on cards with recent unusual movements

---

## Implementation Sequence

### Week 1-2: Price Trajectories (Feature 24)
- Highest immediate value — replaces heuristic trends with computed ones
- Pure SQL + Go math, no AI dependency
- Frontend: trajectory table in TuningTab + badges in inventory

### Week 3: CL Accuracy (Feature 25)
- Depends on having some `cl_value_history` data (at least 3-4 CL import cycles)
- Business-critical insight — directly informs buy terms decisions
- Frontend: new tab in insights

### Week 4: Supply Pressure (Feature 26)
- Depends on `population_history` (needs multiple CL import cycles)
- Simpler computation, lower data requirements
- Frontend: badges in inventory

### Week 5: Market Event Detection (Feature 27)
- Depends on 30+ days of `market_snapshot_history` for z-score baselines
- Leverages existing AI infrastructure (advisor service + tool calling)
- Frontend: new dashboard card + inventory badges

---

## New AI Advisor Tools Summary

Phase 5 adds 4 tools to the tool executor:

| Tool | Description |
|------|-------------|
| `get_price_trajectories` | Price direction, momentum, projections, and volatility per card in a campaign |
| `get_cl_accuracy` | CL valuation accuracy by segment — reveals where CL overvalues/undervalues |
| `get_supply_pressure` | Population growth rates and supply-demand scores per card |
| `detect_anomalies` | Recent market anomalies (price spikes/drops, volume surges) across portfolio |

These tools enhance the existing AI advisor prompts. The liquidation analysis becomes more precise when it can reference price trajectories and supply pressure. The campaign analysis can cite CL accuracy data to recommend buy terms adjustments.

---

## New API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/campaigns/{id}/trajectories` | Price trajectories for a campaign's unsold cards |
| `GET` | `/api/portfolio/cl-accuracy` | CL accuracy report across all segments |
| `POST` | `/api/advisor/market-events` | AI-generated market event analysis (SSE stream) |

Supply pressure data is accessed via the AI tool only (no dedicated endpoint) since it's primarily useful as AI context rather than a standalone view.

---

## Data Retention

As snapshot history grows (~900MB/year at 1,000 cards), implement tiered retention:

```sql
-- Keep full daily data for 90 days
-- Downsample to weekly for 91-365 days
-- Downsample to monthly for 1-3 years

-- Weekly downsampling: keep the row closest to each Monday
DELETE FROM market_snapshot_history
WHERE snapshot_date < date('now', '-90 days')
  AND snapshot_date >= date('now', '-365 days')
  AND strftime('%w', snapshot_date) != '1';  -- keep Mondays only

-- Monthly downsampling: keep the first of each month
DELETE FROM market_snapshot_history
WHERE snapshot_date < date('now', '-365 days')
  AND strftime('%d', snapshot_date) != '01';  -- keep 1st of month only
```

Implement as a new `SnapshotRetentionScheduler` that runs monthly, following the existing `AccessLogCleanupScheduler` pattern.

---

## Verification

### Feature 24 (Trajectories)
- Populate `market_snapshot_history` with 30+ rows for a test card
- Call `GET /api/campaigns/{id}/trajectories` — verify slope, direction, projections
- Ask AI advisor "What are the price trends in my vintage campaign?" — verify it uses trajectory data

### Feature 25 (CL Accuracy)
- Populate `cl_value_history` with 3+ observations per cert at different CL values
- Record some sales at different prices than CL
- Call `GET /api/portfolio/cl-accuracy` — verify deviation percentages make sense
- Ask AI advisor "Is CL overvaluing any of my segments?" — verify reasoning

### Feature 26 (Supply Pressure)
- Populate `population_history` with 2+ observations showing pop changes
- Call the `get_supply_pressure` AI tool — verify growth rates and signals
- Ask AI advisor "Which cards face supply pressure?" — verify it references pop data

### Feature 27 (Market Events)
- Inject anomalous data points into `market_snapshot_history` (e.g., 50% price spike)
- Call `POST /api/advisor/market-events` — verify anomaly detection and AI narrative
- Check that correlated movements (same set) are grouped correctly
