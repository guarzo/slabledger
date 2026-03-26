# AI Success Rate Improvement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the 0% AI success rate on Azure AI Foundry pay-as-you-go tier by reducing API payload sizes and shifting workloads to off-peak hours.

**Architecture:** Two changes: (A) Payload diet — reduce tool output limit, add a composite `get_dashboard_summary` tool, reduce tool rounds and prior context. (B) Off-peak scheduling — add time-of-day config for advisor and social content schedulers with longer inter-analysis pauses.

**Tech Stack:** Go 1.26, SQLite, Azure AI Foundry Responses API

**Spec:** `docs/superpowers/specs/2026-03-26-ai-success-rate-design.md`

---

### Task 1: Reduce tool output limit (15KB -> 8KB)

**Files:**
- Modify: `internal/adapters/advisortool/executor.go:112`
- Modify: `internal/adapters/advisortool/executor_test.go`

- [ ] **Step 1: Update the truncation test to expect 8KB limit**

In `internal/adapters/advisortool/executor_test.go`, find the `TestToJSON_Truncation` test (or add one if missing). Write a test that verifies `toJSON` truncates at 8000 bytes:

```go
func TestToJSON_TruncatesAt8KB(t *testing.T) {
	// Build a slice that marshals to >8000 bytes
	items := make([]map[string]string, 200)
	for i := range items {
		items[i] = map[string]string{"id": fmt.Sprintf("item-%04d", i), "data": "padding-value-here"}
	}
	result := toJSON(items)
	if len(result) > 8000 {
		t.Errorf("toJSON output = %d bytes, want <= 8000", len(result))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/advisortool/ -run TestToJSON_TruncatesAt8KB -v`
Expected: FAIL (current limit is 15000)

- [ ] **Step 3: Change maxLen from 15000 to 8000**

In `internal/adapters/advisortool/executor.go:112`, change:
```go
const maxLen = 8000
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/advisortool/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/advisortool/executor.go internal/adapters/advisortool/executor_test.go
git commit -m "perf(ai): reduce tool output limit from 15KB to 8KB

Reduces worst-case combined payload for 6 parallel tools from
90KB to 48KB, staying under Azure pay-as-you-go capacity ceiling."
```

---

### Task 2: Add `get_dashboard_summary` composite tool

**Files:**
- Modify: `internal/adapters/advisortool/executor.go`
- Modify: `internal/adapters/advisortool/executor_test.go:23` (tool count 21 -> 22)

- [ ] **Step 1: Update tool count test**

In `internal/adapters/advisortool/executor_test.go:23`, change:
```go
const want = 22
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/advisortool/ -run TestDefinitions_Count -v`
Expected: FAIL — "returned 21 tools, want 22"

- [ ] **Step 3: Add dashboardSummary struct and registerGetDashboardSummary**

After the last `register*` method in `internal/adapters/advisortool/executor.go` (after `registerGetSuggestionStats`), add:

```go
// dashboardSummary is a compact aggregate of the four most commonly requested
// portfolio-level data sources. Adapter-level orchestration — not domain logic.
type dashboardSummary struct {
	WeeklyReview struct {
		PurchaseCount    int `json:"purchaseCount"`
		PurchaseSpend    int `json:"purchaseSpendCents"`
		SaleCount        int `json:"saleCount"`
		SaleRevenue      int `json:"saleRevenueCents"`
		NetProfit        int `json:"netProfitCents"`
		PurchaseCountWoW int `json:"purchaseCountWoW"`
		SaleCountWoW     int `json:"saleCountWoW"`
		ProfitWoW        int `json:"profitWoWCents"`
	} `json:"weeklyReview"`
	Credit struct {
		BalanceCents   int     `json:"balanceCents"`
		LimitCents     int     `json:"limitCents"`
		UtilizationPct float64 `json:"utilizationPct"`
		AlertLevel     string  `json:"alertLevel"`
		DaysToInvoice  int     `json:"daysToInvoice"`
	} `json:"credit"`
	PortfolioHealth []struct {
		CampaignName  string `json:"campaignName"`
		Status        string `json:"status"`
		Reason        string `json:"reason"`
		CapitalAtRisk int    `json:"capitalAtRiskCents"`
	} `json:"portfolioHealth"`
	ChannelVelocity []struct {
		Channel   string  `json:"channel"`
		AvgDays   float64 `json:"avgDaysToSell"`
		SaleCount int     `json:"saleCount"`
	} `json:"channelVelocity"`
}

func (e *CampaignToolExecutor) registerGetDashboardSummary() {
	e.register(ai.ToolDefinition{
		Name:        "get_dashboard_summary",
		Description: "Get a compact portfolio overview: weekly performance, credit health, campaign statuses, and channel velocity. Start here before drilling into specific tools.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		var ds dashboardSummary

		if wr, err := e.svc.GetWeeklyReviewSummary(ctx); err == nil && wr != nil {
			ds.WeeklyReview.PurchaseCount = wr.PurchasesThisWeek
			ds.WeeklyReview.PurchaseSpend = wr.SpendThisWeekCents
			ds.WeeklyReview.SaleCount = wr.SalesThisWeek
			ds.WeeklyReview.SaleRevenue = wr.RevenueThisWeekCents
			ds.WeeklyReview.NetProfit = wr.ProfitThisWeekCents
			ds.WeeklyReview.PurchaseCountWoW = wr.PurchasesThisWeek - wr.PurchasesLastWeek
			ds.WeeklyReview.SaleCountWoW = wr.SalesThisWeek - wr.SalesLastWeek
			ds.WeeklyReview.ProfitWoW = wr.ProfitThisWeekCents - wr.ProfitLastWeekCents
		}

		if cs, err := e.svc.GetCreditSummary(ctx); err == nil && cs != nil {
			ds.Credit.BalanceCents = cs.OutstandingCents
			ds.Credit.LimitCents = cs.CreditLimitCents
			ds.Credit.UtilizationPct = cs.UtilizationPct
			ds.Credit.AlertLevel = cs.AlertLevel
			ds.Credit.DaysToInvoice = cs.DaysToNextInvoice
		}

		if ph, err := e.svc.GetPortfolioHealth(ctx); err == nil && ph != nil {
			for _, ch := range ph.Campaigns {
				ds.PortfolioHealth = append(ds.PortfolioHealth, struct {
					CampaignName  string `json:"campaignName"`
					Status        string `json:"status"`
					Reason        string `json:"reason"`
					CapitalAtRisk int    `json:"capitalAtRiskCents"`
				}{
					CampaignName:  ch.CampaignName,
					Status:        ch.HealthStatus,
					Reason:        ch.HealthReason,
					CapitalAtRisk: ch.CapitalAtRisk,
				})
			}
		}

		if cv, err := e.svc.GetPortfolioChannelVelocity(ctx); err == nil {
			for _, v := range cv {
				ds.ChannelVelocity = append(ds.ChannelVelocity, struct {
					Channel   string  `json:"channel"`
					AvgDays   float64 `json:"avgDaysToSell"`
					SaleCount int     `json:"saleCount"`
				}{
					Channel:   string(v.Channel),
					AvgDays:   v.AvgDaysToSell,
					SaleCount: v.SaleCount,
				})
			}
		}

		return toJSON(ds), nil
	})
}
```

- [ ] **Step 4: Register the new tool in registerTools()**

In `internal/adapters/advisortool/executor.go`, in the `registerTools()` method, add after `registerGetChannelVelocity()`:
```go
e.registerGetDashboardSummary()
```

- [ ] **Step 5: Add handler test for get_dashboard_summary**

In `internal/adapters/advisortool/executor_test.go`, add a test that verifies the tool calls the four service methods and maps fields correctly:

```go
func TestExecute_GetDashboardSummary(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetWeeklyReviewSummaryFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
			return &campaigns.WeeklyReviewSummary{
				PurchasesThisWeek:    10,
				PurchasesLastWeek:    8,
				SpendThisWeekCents:   50000,
				SalesThisWeek:        5,
				SalesLastWeek:        3,
				RevenueThisWeekCents: 30000,
				ProfitThisWeekCents:  5000,
				ProfitLastWeekCents:  3000,
			}, nil
		},
		GetCreditSummaryFn: func(_ context.Context) (*campaigns.CreditSummary, error) {
			return &campaigns.CreditSummary{
				CreditLimitCents:  5000000,
				OutstandingCents:  2500000,
				UtilizationPct:    50.0,
				AlertLevel:        "ok",
				DaysToNextInvoice: 7,
			}, nil
		},
		GetPortfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
			return &campaigns.PortfolioHealth{
				Campaigns: []campaigns.CampaignHealth{
					{CampaignName: "Test", HealthStatus: "healthy", HealthReason: "good", CapitalAtRisk: 1000},
				},
			}, nil
		},
		GetPortfolioChannelVelocityFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
			return []campaigns.ChannelVelocity{
				{Channel: "ebay", AvgDaysToSell: 14.5, SaleCount: 5},
			}, nil
		},
	}
	e := newTestExecutor(svc)

	result, err := e.Execute(context.Background(), "get_dashboard_summary", "{}")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// Verify key fields are present in the JSON output
	if !strings.Contains(result, `"purchaseCount":10`) {
		t.Errorf("missing purchaseCount in result: %s", result)
	}
	if !strings.Contains(result, `"alertLevel":"ok"`) {
		t.Errorf("missing alertLevel in result: %s", result)
	}
	if !strings.Contains(result, `"purchaseCountWoW":2`) {
		t.Errorf("missing WoW delta in result: %s", result)
	}
}
```

Make sure `"strings"` is imported in the test file.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/adapters/advisortool/ -v`
Expected: All PASS (including tool count = 22 and handler test)

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/advisortool/executor.go internal/adapters/advisortool/executor_test.go
git commit -m "feat(ai): add get_dashboard_summary composite tool

Aggregates weekly review, credit summary, portfolio health, and
channel velocity into a single ~2-3KB response. Replaces 4
separate tool calls for initial data gathering, reducing payload
size and Azure capacity pressure."
```

---

### Task 3: Update advisor constants and operation tool sets

**Files:**
- Modify: `internal/domain/advisor/service_impl.go:14-18` (constants)
- Modify: `internal/domain/advisor/service_impl.go:23-48` (operationTools)
- Modify: `internal/domain/advisor/service_impl.go:291` (maxPriorContextLen)

- [ ] **Step 1: Change defaultMaxToolRounds from 5 to 3**

In `internal/domain/advisor/service_impl.go:15`:
```go
defaultMaxToolRounds = 3
```

- [ ] **Step 2: Change maxPriorContextLen from 4000 to 2000**

In `internal/domain/advisor/service_impl.go:291`:
```go
const maxPriorContextLen = 2000
```

- [ ] **Step 3: Add get_dashboard_summary to digest and liquidation tool sets**

In `internal/domain/advisor/service_impl.go`, update `operationTools`:

For `"digest"`, add `"get_dashboard_summary"` to the list:
```go
"digest": {
    "list_campaigns", "get_campaign_pnl", "get_pnl_by_channel",
    "get_campaign_tuning", "get_inventory_aging", "get_global_inventory",
    "get_sell_sheet", "get_portfolio_health", "get_portfolio_insights",
    "get_credit_summary", "get_weekly_review", "get_capital_timeline",
    "get_channel_velocity", "get_dashboard_summary",
},
```

For `"liquidation"`, add `"get_dashboard_summary"` to the list:
```go
"liquidation": {
    "list_campaigns", "get_global_inventory", "get_sell_sheet",
    "get_credit_summary", "get_expected_values", "get_inventory_aging",
    "get_portfolio_health", "suggest_price", "get_cert_lookup",
    "get_channel_velocity", "get_capital_timeline", "get_suggestion_stats",
    "get_dashboard_summary",
},
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/advisor/... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/advisor/service_impl.go
git commit -m "perf(ai): reduce maxToolRounds to 3, trim prior context, add dashboard tool to operations

Limits tool-calling rounds from 5 to 3 to prevent context
accumulation. Trims prior context from 4000 to 2000 chars.
Adds get_dashboard_summary to digest and liquidation tool sets."
```

---

### Task 4: Update advisor prompts to prefer dashboard summary

**Files:**
- Modify: `internal/domain/advisor/prompts.go`

- [ ] **Step 1: Update digestSystemPrompt**

In `internal/domain/advisor/prompts.go`, change `digestSystemPrompt` to:
```go
const digestSystemPrompt = baseSystemPrompt + `

## Your Task: Weekly Intelligence Digest
Generate a comprehensive weekly business review. Fetch all relevant data using tools before writing.
Focus on actionable insights, not data recitation. Lead with what matters most this week.

## Tool Strategy
Start with get_dashboard_summary for a portfolio overview. Only call individual tools
(get_weekly_review, get_inventory_aging, etc.) if you need per-card or per-campaign detail
beyond what the summary provides.`
```

- [ ] **Step 2: Update liquidationSystemPrompt**

In `internal/domain/advisor/prompts.go`, change `liquidationSystemPrompt` to:
```go
const liquidationSystemPrompt = baseSystemPrompt + `

## Your Task: Liquidation Analysis
Identify cards where selling now (even below market) is better than holding.
Consider: credit pressure, carrying costs (5% annual), days held, market trend, liquidity, and EV.
A card with negative EV or declining market that ties up capital should be liquidated.
Prioritize by capital freed relative to markdown cost.

When you identify cards that should be repriced, use the suggest_price tool
to save your recommended price. The user will review your suggestions
in the inventory UI and can accept or dismiss each one.

Before making new suggestions, call get_suggestion_stats to see how your
previous recommendations performed. If acceptance rate is low, adjust your
pricing strategy — you may be suggesting prices that are too aggressive.

## Tool Strategy
Start with get_dashboard_summary for a portfolio overview and credit health check.
Only call individual tools (get_global_inventory, get_sell_sheet, etc.) if you need
per-card detail beyond what the summary provides.`
```

- [ ] **Step 3: Run build to verify compilation**

Run: `go build ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/domain/advisor/prompts.go
git commit -m "perf(ai): update prompts to prefer dashboard summary tool

Guides the model to start with get_dashboard_summary for overview
data, reducing the number of parallel tool calls in early rounds."
```

---

### Task 5: Add time-of-day config fields

**Files:**
- Modify: `internal/platform/config/types.go:192-196`
- Modify: `internal/platform/config/defaults.go:96-105`
- Modify: `internal/platform/config/loader.go:274-302`

- [ ] **Step 1: Add RefreshHour to AdvisorRefreshConfig**

In `internal/platform/config/types.go`, update `AdvisorRefreshConfig`:
```go
// AdvisorRefreshConfig controls the background AI advisor analysis scheduler.
type AdvisorRefreshConfig struct {
	Enabled      bool
	Interval     time.Duration // how often to run analysis (default: 24h)
	InitialDelay time.Duration // delay before first run (default: 2m)
	RefreshHour  int           // hour (0-23 UTC) to schedule runs; -1 = use InitialDelay (default: 4)
}
```

- [ ] **Step 2: Add ContentHour to SocialContentConfig**

In `internal/platform/config/types.go`, update `SocialContentConfig`:
```go
// SocialContentConfig controls the background social content generation scheduler.
type SocialContentConfig struct {
	Enabled      bool
	Interval     time.Duration // how often to run detection (default: 24h)
	InitialDelay time.Duration // delay before first run (default: 5m)
	ContentHour  int           // hour (0-23 UTC) to schedule runs; -1 = use InitialDelay (default: 5)
}
```

- [ ] **Step 3: Update defaults**

In `internal/platform/config/defaults.go`, update the AdvisorRefresh and SocialContent defaults:
```go
AdvisorRefresh: AdvisorRefreshConfig{
    Enabled:      true,
    Interval:     24 * time.Hour,
    InitialDelay: 2 * time.Minute,
    RefreshHour:  4, // 4 AM UTC
},
SocialContent: SocialContentConfig{
    Enabled:      false, // disabled by default
    Interval:     24 * time.Hour,
    InitialDelay: 5 * time.Minute,
    ContentHour:  5, // 5 AM UTC
},
```

- [ ] **Step 4: Parse new env vars in loader**

In `internal/platform/config/loader.go`, after the existing `ADVISOR_REFRESH_INITIAL_DELAY` parsing (~line 287), add:
```go
if v := os.Getenv("ADVISOR_REFRESH_HOUR"); v != "" {
    if h, err := strconv.Atoi(v); err == nil && h >= -1 && h <= 23 {
        cfg.AdvisorRefresh.RefreshHour = h
    }
}
```

After the existing `SOCIAL_CONTENT_INITIAL_DELAY` parsing (~line 302), add:
```go
if v := os.Getenv("SOCIAL_CONTENT_HOUR"); v != "" {
    if h, err := strconv.Atoi(v); err == nil && h >= -1 && h <= 23 {
        cfg.SocialContent.ContentHour = h
    }
}
```

Make sure `"strconv"` is in the import block of `loader.go`.

- [ ] **Step 5: Run build**

Run: `go build ./...`
Expected: Success

- [ ] **Step 6: Commit**

```bash
git add internal/platform/config/types.go internal/platform/config/defaults.go internal/platform/config/loader.go
git commit -m "feat(config): add ADVISOR_REFRESH_HOUR and SOCIAL_CONTENT_HOUR env vars

Configurable hour-of-day (UTC) for scheduling advisor and social
content runs. Default: 4 AM and 5 AM UTC respectively. Set to -1
to use InitialDelay directly."
```

---

### Task 6: Add timeUntilHour helper and wire into advisor scheduler

**Files:**
- Modify: `internal/adapters/scheduler/advisor_refresh.go`
- Modify: `internal/adapters/scheduler/advisor_refresh_test.go`

- [ ] **Step 1: Write failing test for timeUntilHour**

Add to `internal/adapters/scheduler/advisor_refresh_test.go`:

```go
func TestTimeUntilHour(t *testing.T) {
	tests := []struct {
		name    string
		now     time.Time
		hour    int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "target hour is later today",
			now:     time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC),
			hour:    4,
			wantMin: 1*time.Hour + 59*time.Minute,
			wantMax: 2*time.Hour + 1*time.Minute,
		},
		{
			name:    "target hour already passed today",
			now:     time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC),
			hour:    4,
			wantMin: 17*time.Hour + 59*time.Minute,
			wantMax: 18*time.Hour + 1*time.Minute,
		},
		{
			name:    "target hour is current hour",
			now:     time.Date(2026, 3, 26, 4, 30, 0, 0, time.UTC),
			hour:    4,
			wantMin: 23*time.Hour + 29*time.Minute,
			wantMax: 23*time.Hour + 31*time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeUntilHour(tt.now, tt.hour)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("timeUntilHour(%v, %d) = %v, want between %v and %v",
					tt.now, tt.hour, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/scheduler/ -run TestTimeUntilHour -v`
Expected: FAIL — `undefined: timeUntilHour`

- [ ] **Step 3: Implement timeUntilHour and wire it into advisor scheduler**

In `internal/adapters/scheduler/advisor_refresh.go`, add the helper function:

```go
// timeUntilHour returns the duration from now until the next occurrence
// of the given hour (0-23) in UTC.
func timeUntilHour(now time.Time, hour int) time.Duration {
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}
```

Update `NewAdvisorRefreshScheduler` to compute InitialDelay from RefreshHour. Replace the existing function body with:

```go
func NewAdvisorRefreshScheduler(
	collector AdvisorCollector,
	cache advisor.CacheStore,
	tracker ai.AICallTracker,
	logger observability.Logger,
	cfg config.AdvisorRefreshConfig,
) *AdvisorRefreshScheduler {
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.InitialDelay < 0 {
		cfg.InitialDelay = 0
	}
	// Compute initial delay from RefreshHour if set (>= 0).
	if cfg.RefreshHour >= 0 {
		cfg.InitialDelay = timeUntilHour(time.Now(), cfg.RefreshHour)
	}
	return &AdvisorRefreshScheduler{
		StopHandle: NewStopHandle(),
		collector:  collector,
		cache:      cache,
		tracker:    tracker,
		logger:     logger.With(context.Background(), observability.String("component", "advisor-refresh")),
		config:     cfg,
	}
}
```

- [ ] **Step 4: Increase inter-analysis pause from 10s to 5min**

In the `tick` method of `advisor_refresh.go`, change the pause:

Add a constant at the top of the file (near the other constants):
```go
const interAnalysisPause = 5 * time.Minute
```

Replace `time.After(10 * time.Second)` with `time.After(interAnalysisPause)`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/scheduler/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/scheduler/advisor_refresh.go internal/adapters/scheduler/advisor_refresh_test.go
git commit -m "feat(scheduler): time-of-day scheduling for advisor, 5min inter-analysis pause

Computes InitialDelay from ADVISOR_REFRESH_HOUR (default 4 AM UTC)
so advisor analyses run during off-peak Azure hours. Increases
inter-analysis pause from 10s to 5min to let Azure capacity recover."
```

---

### Task 7: Wire time-of-day scheduling into social content scheduler

**Files:**
- Modify: `internal/adapters/scheduler/social_content.go`
- Create: `internal/adapters/scheduler/social_content_test.go`

- [ ] **Step 1: Write failing test for social content delay**

Create `internal/adapters/scheduler/social_content_test.go`:

```go
package scheduler

import (
	"testing"
	"time"
)

func TestSocialContentScheduler_UsesContentHour(t *testing.T) {
	// timeUntilHour is already tested in advisor_refresh_test.go.
	// Just verify it's reachable from this package (it's package-level).
	now := time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC)
	d := timeUntilHour(now, 5)
	if d < 2*time.Hour || d > 4*time.Hour {
		t.Errorf("timeUntilHour(%v, 5) = %v, want ~3h", now, d)
	}
}
```

- [ ] **Step 2: Run test to verify it passes** (timeUntilHour already exists from Task 6)

Run: `go test ./internal/adapters/scheduler/ -run TestSocialContentScheduler_UsesContentHour -v`
Expected: PASS

- [ ] **Step 3: Update NewSocialContentScheduler to use ContentHour**

In `internal/adapters/scheduler/social_content.go`, update `NewSocialContentScheduler`:

After the existing `if cfg.InitialDelay <= 0 { ... }` block, add:
```go
// Compute initial delay from ContentHour if set (>= 0).
if cfg.ContentHour >= 0 {
    cfg.InitialDelay = timeUntilHour(time.Now(), cfg.ContentHour)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/scheduler/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/scheduler/social_content.go internal/adapters/scheduler/social_content_test.go
git commit -m "feat(scheduler): time-of-day scheduling for social content

Uses SOCIAL_CONTENT_HOUR (default 5 AM UTC) to schedule social
content generation during off-peak hours, 1 hour after advisor."
```

---

### Task 8: Update CLAUDE.md and final verification

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add new env vars to CLAUDE.md**

In the `## Environment Variables` section of `CLAUDE.md`, add under the `# Optional` section (near the other scheduler vars):

```
ADVISOR_REFRESH_HOUR="4"     # Hour (0-23 UTC) to run advisor; -1 = use InitialDelay
SOCIAL_CONTENT_HOUR="5"      # Hour (0-23 UTC) to run social content; -1 = use InitialDelay
```

Note: When `*_HOUR` is set (default: >= 0), it takes precedence over `*_INITIAL_DELAY`. Set `*_HOUR=-1` to use `*_INITIAL_DELAY` instead.

- [ ] **Step 2: Run full test suite**

Run: `go test -race -timeout 5m ./...`
Expected: All PASS

- [ ] **Step 3: Run linter**

Run: `golangci-lint run ./...`
Expected: 0 issues

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add ADVISOR_REFRESH_HOUR and SOCIAL_CONTENT_HOUR to CLAUDE.md"
```
