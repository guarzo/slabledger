package advisortool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
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

func (e *CampaignToolExecutor) registerGetCreditSummary() {
	e.register(ai.ToolDefinition{
		Name:        "get_credit_summary",
		Description: "Get credit utilization: outstanding balance, credit limit, utilization %, alert level (ok/warning/critical), projected exposure, and days to next invoice.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetCreditSummary(ctx)
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

func (e *CampaignToolExecutor) registerGetExpectedValues() {
	e.registerCampaignTool("get_expected_values",
		"Get expected value per unsold card in a campaign: EV in cents, EV per dollar invested, sell probability, confidence level.",
		func(ctx context.Context, id string) (any, error) { return e.svc.GetExpectedValues(ctx, id) })
}

func (e *CampaignToolExecutor) registerGetCrackCandidates() {
	e.registerCampaignTool("get_crack_candidates",
		"Get crack arbitrage analysis: cards where selling raw may be more profitable than selling graded. Shows graded vs crack net, advantage, and ROI comparison.",
		func(ctx context.Context, id string) (any, error) { return e.svc.GetCrackCandidates(ctx, id) })
}

func (e *CampaignToolExecutor) registerGetCampaignSuggestions() {
	e.register(ai.ToolDefinition{
		Name:        "get_campaign_suggestions",
		Description: "Get data-driven suggestions for new campaigns and adjustments to existing ones, with expected ROI, margin, and confidence.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetCampaignSuggestions(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerRunProjection() {
	e.registerCampaignTool("run_projection",
		"Run Monte Carlo simulation (1000 iterations) comparing current campaign parameters vs alternatives. Returns P10/P50/P90 distributions for ROI and profit.",
		func(ctx context.Context, id string) (any, error) { return e.svc.RunProjection(ctx, id) })
}

func (e *CampaignToolExecutor) registerGetChannelVelocity() {
	e.register(ai.ToolDefinition{
		Name:        "get_channel_velocity",
		Description: "Get average days to sell and sale count by channel across all campaigns.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetPortfolioChannelVelocity(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetCertLookup() {
	e.register(ai.ToolDefinition{
		Name:        "get_cert_lookup",
		Description: "Look up a PSA certification number to get card details and current market data (prices, trends, velocity, listings).",
		Parameters: jsonSchema{
			Type: "object",
			Properties: map[string]jsonSchema{
				"certNumber": {Type: "string", Description: "PSA certification number"},
			},
			Required: []string{"certNumber"},
		},
	}, func(ctx context.Context, args string) (string, error) {
		var p struct {
			CertNumber string `json:"certNumber"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if p.CertNumber == "" {
			return "", fmt.Errorf("certNumber is required")
		}
		certInfo, snapshot, err := e.svc.LookupCert(ctx, p.CertNumber)
		if err != nil {
			return "", err
		}
		result := struct {
			CertInfo *campaigns.CertInfo       `json:"certInfo"`
			Market   *campaigns.MarketSnapshot `json:"market,omitempty"`
		}{
			CertInfo: certInfo,
			Market:   snapshot,
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerEvaluatePurchase() {
	e.register(ai.ToolDefinition{
		Name:        "evaluate_purchase",
		Description: "Evaluate a hypothetical purchase: compute expected value, sell probability, estimated profit, and confidence level for a card at a given grade and buy cost within a campaign.",
		Parameters: jsonSchema{
			Type: "object",
			Properties: map[string]jsonSchema{
				"campaignId":   {Type: "string", Description: "Campaign ID to evaluate within"},
				"cardName":     {Type: "string", Description: "Card name"},
				"grade":        {Type: "number", Description: "PSA grade (e.g. 9, 10)"},
				"buyCostCents": {Type: "integer", Description: "Buy cost in cents"},
			},
			Required: []string{"campaignId", "cardName", "grade", "buyCostCents"},
		},
	}, func(ctx context.Context, args string) (string, error) {
		var p struct {
			CampaignID   string  `json:"campaignId"`
			CardName     string  `json:"cardName"`
			Grade        float64 `json:"grade"`
			BuyCostCents int     `json:"buyCostCents"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if p.CampaignID == "" {
			return "", fmt.Errorf("campaignId is required")
		}
		if p.CardName == "" {
			return "", fmt.Errorf("cardName is required")
		}
		result, err := e.svc.EvaluatePurchase(ctx, p.CampaignID, p.CardName, p.Grade, p.BuyCostCents)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerSuggestPrice() {
	e.register(ai.ToolDefinition{
		Name:        "suggest_price",
		Description: "Suggest a sell price for a purchase. The suggestion is saved for user review — the user must accept or dismiss it in the UI before it takes effect.",
		Parameters: jsonSchema{
			Type: "object",
			Properties: map[string]jsonSchema{
				"purchaseId": {Type: "string", Description: "Purchase ID to suggest a price for"},
				"priceCents": {Type: "integer", Description: "Suggested price in cents"},
			},
			Required: []string{"purchaseId", "priceCents"},
		},
	}, func(ctx context.Context, args string) (string, error) {
		var p struct {
			PurchaseID string `json:"purchaseId"`
			PriceCents int    `json:"priceCents"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if p.PurchaseID == "" {
			return "", fmt.Errorf("purchaseId is required")
		}
		if p.PriceCents <= 0 {
			return "", fmt.Errorf("priceCents must be positive")
		}
		if err := e.svc.SetAISuggestedPrice(ctx, p.PurchaseID, p.PriceCents); err != nil {
			return "", err
		}
		return `{"status":"ok","note":"Suggestion saved. User will review and accept/dismiss."}`, nil
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
	Errors []string `json:"errors,omitempty"`
}

func (e *CampaignToolExecutor) registerGetDashboardSummary() {
	e.register(ai.ToolDefinition{
		Name:        "get_dashboard_summary",
		Description: "Get a compact portfolio overview: weekly performance, credit health, campaign statuses, and channel velocity. Start here before drilling into specific tools.",
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

		if cs, err := e.svc.GetCreditSummary(ctx); err != nil {
			ds.Errors = append(ds.Errors, "creditSummary: "+err.Error())
		} else if cs != nil {
			ds.Credit.BalanceCents = cs.OutstandingCents
			ds.Credit.LimitCents = cs.CreditLimitCents
			ds.Credit.UtilizationPct = cs.UtilizationPct
			ds.Credit.AlertLevel = cs.AlertLevel
			ds.Credit.DaysToInvoice = cs.DaysToNextInvoice
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

func (e *CampaignToolExecutor) registerGetAcquisitionTargets() {
	e.register(ai.ToolDefinition{
		Name:        "get_acquisition_targets",
		Description: "Get raw-to-graded arbitrage opportunities: cards where buying raw NM and grading would yield $100+ profit. Shows raw NM price, best graded estimate, profit, and ROI.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetAcquisitionTargets(ctx)
		if err != nil {
			return "", err
		}
		if len(result) > 20 {
			result = result[:20]
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetCrackOpportunities() {
	e.register(ai.ToolDefinition{
		Name:        "get_crack_opportunities",
		Description: "Get cross-campaign crack arbitrage candidates: graded cards where selling raw is more profitable than selling graded. Uses JustTCG NM-specific pricing. Shows crack vs graded net, advantage, and ROI.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetCrackOpportunities(ctx)
		if err != nil {
			return "", err
		}
		if len(result) > 20 {
			result = result[:20]
		}
		return toJSON(result), nil
	})
}

// jsonSchema is a minimal JSON Schema representation for tool parameters.
type jsonSchema struct {
	Type        string                `json:"type"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]jsonSchema `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
}
