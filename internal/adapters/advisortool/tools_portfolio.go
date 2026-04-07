package advisortool

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

func (e *CampaignToolExecutor) registerGetPortfolioHealth() {
	e.register(ai.ToolDefinition{
		Name:        "get_portfolio_health",
		Description: "Get health scores for all campaigns: status (healthy/warning/critical), reason, and capital at risk.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetPortfolioHealth(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetPortfolioInsights() {
	e.register(ai.ToolDefinition{
		Name:        "get_portfolio_insights",
		Description: "Get cross-campaign portfolio segmentation: by character, grade, era, price tier, channel. Includes coverage gaps (profitable segments with no active campaigns).",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetPortfolioInsights(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetCapitalSummary() {
	e.register(ai.ToolDefinition{
		Name:        "get_capital_summary",
		Description: "Get capital exposure: outstanding balance, 30-day recovery rate, weeks to cover, recovery trend, and alert level.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetCapitalSummary(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetWeeklyReview() {
	e.register(ai.ToolDefinition{
		Name:        "get_weekly_review",
		Description: "Get week-over-week comparison: purchases, spend, sales, revenue, profit with deltas. Includes top/bottom performers and channel breakdown.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetWeeklyReviewSummary(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetCapitalTimeline() {
	e.register(ai.ToolDefinition{
		Name:        "get_capital_timeline",
		Description: "Get daily capital deployment: cumulative spend, cumulative recovery, outstanding balance over time, with invoice date markers.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetCapitalTimeline(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetSuggestionStats() {
	e.register(ai.ToolDefinition{
		Name:        "get_suggestion_stats",
		Description: "Get statistics on AI price suggestions: how many were accepted, dismissed, or still pending. Use this to calibrate your pricing recommendations.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetPriceOverrideStats(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

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
	Capital struct {
		BalanceCents         int     `json:"balanceCents"`
		RecoveryRate30dCents int     `json:"recoveryRate30dCents"`
		WeeksToCover         float64 `json:"weeksToCover"`
		RecoveryTrend        string  `json:"recoveryTrend"`
		AlertLevel           string  `json:"alertLevel"`
	} `json:"capital"`
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
	Errors []string `json:"errors,omitempty"`
}

func (e *CampaignToolExecutor) registerGetDashboardSummary() {
	e.register(ai.ToolDefinition{
		Name:        "get_dashboard_summary",
		Description: "Get a compact portfolio overview: weekly performance, capital velocity, campaign statuses, and channel velocity. Start here before drilling into specific tools.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		var ds dashboardSummary

		if wr, err := e.svc.GetWeeklyReviewSummary(ctx); err != nil {
			ds.Errors = append(ds.Errors, "weeklyReview: "+err.Error())
		} else if wr != nil {
			ds.WeeklyReview.PurchaseCount = wr.PurchasesThisWeek
			ds.WeeklyReview.PurchaseSpend = wr.SpendThisWeekCents
			ds.WeeklyReview.SaleCount = wr.SalesThisWeek
			ds.WeeklyReview.SaleRevenue = wr.RevenueThisWeekCents
			ds.WeeklyReview.NetProfit = wr.ProfitThisWeekCents
			ds.WeeklyReview.PurchaseCountWoW = wr.PurchasesThisWeek - wr.PurchasesLastWeek
			ds.WeeklyReview.SaleCountWoW = wr.SalesThisWeek - wr.SalesLastWeek
			ds.WeeklyReview.ProfitWoW = wr.ProfitThisWeekCents - wr.ProfitLastWeekCents
		}

		if cs, err := e.svc.GetCapitalSummary(ctx); err != nil {
			ds.Errors = append(ds.Errors, "capitalSummary: "+err.Error())
		} else if cs != nil {
			ds.Capital.BalanceCents = cs.OutstandingCents
			ds.Capital.RecoveryRate30dCents = cs.RecoveryRate30dCents
			ds.Capital.WeeksToCover = cs.WeeksToCover
			ds.Capital.RecoveryTrend = cs.RecoveryTrend
			ds.Capital.AlertLevel = cs.AlertLevel
		}

		if ph, err := e.svc.GetPortfolioHealth(ctx); err != nil {
			ds.Errors = append(ds.Errors, "portfolioHealth: "+err.Error())
		} else if ph != nil {
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

		if cv, err := e.svc.GetPortfolioChannelVelocity(ctx); err != nil {
			ds.Errors = append(ds.Errors, "channelVelocity: "+err.Error())
		} else {
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
