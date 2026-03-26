# AI Success Rate Improvement

**Date:** 2026-03-26
**Status:** Approved
**Problem:** Azure AI Foundry gpt-5.4-pro on pay-as-you-go tier returns `no_capacity` errors and connection resets during advisor and social content analysis. 0% success rate across all AI operations.

## Root Cause

Azure's error: *"Your request exceeds the maximum usage size allowed during peak load."*

The advisor's tool-calling loop sends 5-6 tool outputs simultaneously (up to 90KB combined), exceeding the pay-as-you-go tier's capacity ceiling during peak hours. Connection resets follow when Azure drops overloaded TCP connections.

## Approach

Two complementary strategies: reduce payload size (A) and shift to off-peak hours (B).

---

## A. Payload Diet

### A1. Reduce tool output limit (15KB -> 8KB)

**File:** `internal/adapters/advisortool/executor.go`

Change `maxLen` constant from 15000 to 8000. With 6 parallel tools, worst case drops from 90KB to 48KB. The existing `truncateJSON` function handles graceful array halving — smaller limits just mean fewer items in inventory/sell-sheet arrays.

### A2. New `get_dashboard_summary` composite tool

**File:** `internal/adapters/advisortool/executor.go`

A single tool that returns compact top-line metrics from four commonly-requested sources:

- **Weekly review:** spend, revenue, profit, deltas (no per-card breakdown)
- **Credit summary:** balance, utilization %, alert level, days to next invoice
- **Portfolio health:** per-campaign status and capital at risk (no per-card details)
- **Channel velocity:** avg days to sell and sale count by channel

Target output: 2-3KB total. Replaces 4 separate tool calls (`get_weekly_review`, `get_credit_summary`, `get_portfolio_health`, `get_channel_velocity`) for initial data gathering.

**Implementation:**

This is adapter-level orchestration (calling four existing service methods and projecting results), not domain business logic. Per the hexagonal architecture, it belongs entirely in the adapter layer.

- New `registerGetDashboardSummary()` method in `executor.go`
- Calls `svc.GetWeeklyReviewSummary()`, `svc.GetCreditSummary()`, `svc.GetPortfolioHealth()`, and `svc.GetPortfolioChannelVelocity()` internally
- Projects results into a local `dashboardSummary` struct in `executor.go` with only top-level fields:

```go
type dashboardSummary struct {
    WeeklyReview struct {
        PurchaseCount    int   `json:"purchaseCount"`
        PurchaseSpend    int64 `json:"purchaseSpendCents"`
        SaleCount        int   `json:"saleCount"`
        SaleRevenue      int64 `json:"saleRevenueCents"`
        NetProfit        int64 `json:"netProfitCents"`
        PurchaseCountWoW int   `json:"purchaseCountWoW"`
        SaleCountWoW     int   `json:"saleCountWoW"`
        ProfitWoW        int64 `json:"profitWoWCents"`
    } `json:"weeklyReview"`
    Credit struct {
        BalanceCents   int64   `json:"balanceCents"`
        LimitCents     int64   `json:"limitCents"`
        UtilizationPct float64 `json:"utilizationPct"`
        AlertLevel     string  `json:"alertLevel"`
        DaysToInvoice  int     `json:"daysToInvoice"`
    } `json:"credit"`
    PortfolioHealth []struct {
        CampaignName string `json:"campaignName"`
        Status       string `json:"status"`
        Reason       string `json:"reason"`
        CapitalAtRisk int64 `json:"capitalAtRiskCents"`
    } `json:"portfolioHealth"`
    ChannelVelocity []struct {
        Channel     string  `json:"channel"`
        AvgDays     float64 `json:"avgDaysToSell"`
        SaleCount   int     `json:"saleCount"`
    } `json:"channelVelocity"`
}
```

- No changes to domain `Service` interface, `service_impl.go`, `types.go`, or mocks
- Individual tools remain available for deep-dives

**Add to operation tool sets:**
- `digest`: add `get_dashboard_summary`
- `liquidation`: add `get_dashboard_summary`
- `campaign_analysis`: excluded — this operation needs per-campaign drill-down from the start, not portfolio-level summaries

### A3. Update advisor prompts

**File:** `internal/domain/advisor/prompts.go`

Update digest and liquidation system prompts to guide the model:
- "Start with `get_dashboard_summary` for an overview before drilling into specific tools."
- "Only call specific tools (get_weekly_review, get_inventory_aging, etc.) when you need per-card or per-campaign detail beyond what the summary provides."

This steers the model toward the composite tool first, reducing the number of parallel tool calls in early rounds.

### A4. Reduce maxToolRounds (5 -> 3)

**File:** `internal/domain/advisor/service_impl.go`

Change `defaultMaxToolRounds` from 5 to 3. The model typically uses 3 rounds (list campaigns -> gather data -> write analysis). Capping at 3 prevents runaway loops that accumulate context.

### A5. Trim prior context (4000 -> 2000 chars)

**File:** `internal/domain/advisor/service_impl.go`

Change `maxPriorContextLen` from 4000 to 2000. Currently a no-op (no successful analyses exist to cache), but sets a sane limit for when analyses start succeeding.

---

## B. Off-Peak Scheduling

### B1. Time-of-day scheduling for advisor

**Files:** `internal/platform/config/types.go`, `internal/platform/config/loader.go`, `internal/platform/config/defaults.go`, `internal/adapters/scheduler/advisor_refresh.go`

New env var `ADVISOR_REFRESH_HOUR` (default: `4` for 4 AM UTC). The scheduler calculates the duration until the next occurrence of that hour and uses it as `InitialDelay`. After each run, the 24-hour `Interval` handles subsequent runs.

Replaces the current `InitialDelay: 2 * time.Minute` which fires 2 minutes after every deploy — often during peak hours.

**Implementation:**
- Add `RefreshHour int` field to `AdvisorRefreshConfig` in `types.go` (default 4)
- Parse `ADVISOR_REFRESH_HOUR` env var in `loader.go`
- Validate range [-1, 23] in config validation; reject out-of-range values
- In `NewAdvisorRefreshScheduler`, compute `InitialDelay` as time until next `RefreshHour` in UTC
- If `RefreshHour` is -1, use `InitialDelay` directly (preserves override capability)

### B2. Increase inter-analysis pause (10s -> 5min)

**File:** `internal/adapters/scheduler/advisor_refresh.go`

Change the pause between digest and liquidation from 10 seconds to 5 minutes. Lets Azure capacity recover between the two analyses.

Extract the pause duration as a constant for clarity:
```go
const interAnalysisPause = 5 * time.Minute
```

### B3. Time-of-day scheduling for social content

**Files:** `internal/platform/config/types.go`, `internal/platform/config/loader.go`, `internal/adapters/scheduler/social_content.go`

New env var `SOCIAL_CONTENT_HOUR` (default: `5` for 5 AM UTC). Same mechanism as B1 — compute `InitialDelay` to the next occurrence of that hour.

Ensures social content never overlaps with advisor runs (default: 1 hour gap).

Validate range [-1, 23] same as B1.

### B4. Config summary

New env vars:
```
ADVISOR_REFRESH_HOUR=4       # Hour (UTC) to run advisor (default: 4, -1 to use InitialDelay)
SOCIAL_CONTENT_HOUR=5        # Hour (UTC) to run social content (default: 5, -1 to use InitialDelay)
```

---

## Testing

### Unit tests
- `internal/adapters/advisortool/executor_test.go`: verify `toJSON` truncation at 8KB limit; update `TestDefinitions_Count` to expect 22 tools
- `internal/adapters/scheduler/advisor_refresh_test.go`: verify `InitialDelay` computation for various hours/current times, including edge cases (current hour == target hour, past target today)
- `internal/adapters/scheduler/social_content_test.go` (new file): delay computation tests for `SOCIAL_CONTENT_HOUR`
- `internal/adapters/clients/azureai/client_test.go`: existing retry tests still pass

### Manual verification
- Deploy and monitor AI status dashboard for improved success rate
- Check logs for reduced payload sizes in `responses api request` log lines
- Verify advisor runs at configured hour (check `starting advisor analysis` log timestamps)

---

## Files Changed

| File | Change |
|------|--------|
| `internal/adapters/advisortool/executor.go` | maxLen 15K->8K, add `get_dashboard_summary` with local `dashboardSummary` struct |
| `internal/adapters/advisortool/executor_test.go` | Update tool count assertion (21->22) |
| `internal/domain/advisor/service_impl.go` | maxToolRounds 5->3, maxPriorContextLen 4000->2000, add `get_dashboard_summary` to operationTools |
| `internal/domain/advisor/prompts.go` | Update digest/liquidation prompts to prefer dashboard summary |
| `internal/adapters/scheduler/advisor_refresh.go` | Time-of-day scheduling, 5min inter-analysis pause |
| `internal/adapters/scheduler/advisor_refresh_test.go` | InitialDelay computation tests |
| `internal/adapters/scheduler/social_content.go` | Time-of-day scheduling |
| `internal/adapters/scheduler/social_content_test.go` | New file: delay computation tests |
| `internal/platform/config/types.go` | Add `RefreshHour` / `ContentHour` fields to config structs |
| `internal/platform/config/loader.go` | Parse new env vars |
| `internal/platform/config/defaults.go` | Default values for new fields |
| `CLAUDE.md` | Document new env vars |

## Out of Scope

- Provisioned Throughput (cost constraint)
- Limiting parallel tool calls at the API level (Approach C — adds complexity, revisit if A+B insufficient)
- Changing the Azure model or endpoint
- Quality tuning of analysis output (do this after success rate improves)
