package advisortool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func (e *CampaignToolExecutor) registerGetExpectedValues() {
	e.registerCampaignTool("get_expected_values",
		"Get expected value per unsold card in a campaign: EV in cents, EV per dollar invested, sell probability, confidence level.",
		func(ctx context.Context, id string) (any, error) {
			if e.arbSvc == nil {
				return nil, fmt.Errorf("arbitrage service not available")
			}
			return e.arbSvc.GetExpectedValues(ctx, id)
		})
}

func (e *CampaignToolExecutor) registerGetDeslabCandidates() {
	e.registerCampaignTool("get_deslab_candidates",
		"Get deslab arbitrage analysis: cards where removing from PSA slab and selling raw may be more profitable than selling graded. Shows graded vs deslab net, advantage, and ROI comparison.",
		func(ctx context.Context, id string) (any, error) {
			if e.arbSvc == nil {
				return nil, fmt.Errorf("arbitrage service not available")
			}
			return e.arbSvc.GetCrackCandidates(ctx, id)
		})
}

func (e *CampaignToolExecutor) registerGetCampaignSuggestions() {
	e.register(ai.ToolDefinition{
		Name:        "get_campaign_suggestions",
		Description: "Get data-driven suggestions for new campaigns and adjustments to existing ones, with expected ROI, margin, and confidence.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		if e.portSvc == nil {
			return "", fmt.Errorf("portfolio service not available")
		}
		result, err := e.portSvc.GetCampaignSuggestions(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerRunProjection() {
	e.registerCampaignTool("run_projection",
		"Run Monte Carlo simulation (1000 iterations) comparing current campaign parameters vs alternatives. Returns P10/P50/P90 distributions for ROI and profit.",
		func(ctx context.Context, id string) (any, error) {
			if e.arbSvc == nil {
				return nil, fmt.Errorf("arbitrage service not available")
			}
			return e.arbSvc.RunProjection(ctx, id)
		})
}

func (e *CampaignToolExecutor) registerGetChannelVelocity() {
	e.register(ai.ToolDefinition{
		Name:        "get_channel_velocity",
		Description: "Get average days to sell and sale count by channel across all inventory.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		if e.portSvc == nil {
			return "", fmt.Errorf("portfolio service not available")
		}
		result, err := e.portSvc.GetPortfolioChannelVelocity(ctx)
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
			CertInfo *inventory.CertInfo       `json:"certInfo"`
			Market   *inventory.MarketSnapshot `json:"market,omitempty"`
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
		if p.Grade <= 0 {
			return "", fmt.Errorf("grade must be positive")
		}
		if p.BuyCostCents < 0 {
			return "", fmt.Errorf("buyCostCents must be non-negative")
		}
		if e.arbSvc == nil {
			return "", fmt.Errorf("arbitrage service not available")
		}
		result, err := e.arbSvc.EvaluatePurchase(ctx, p.CampaignID, p.CardName, p.Grade, p.BuyCostCents)
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

func (e *CampaignToolExecutor) registerGetAcquisitionTargets() {
	e.register(ai.ToolDefinition{
		Name:        "get_acquisition_targets",
		Description: "Get raw-to-graded arbitrage opportunities: cards where buying raw NM and grading would yield $100+ profit. Shows raw NM price, best graded estimate, profit, and ROI.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		if e.arbSvc == nil {
			return "", fmt.Errorf("arbitrage service not available")
		}
		result, err := e.arbSvc.GetAcquisitionTargets(ctx)
		if err != nil {
			return "", err
		}
		if len(result) > 20 {
			result = result[:20]
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetDeslabOpportunities() {
	e.register(ai.ToolDefinition{
		Name:        "get_deslab_opportunities",
		Description: "Get cross-campaign deslab arbitrage candidates: graded cards where removing from slab and selling raw is more profitable than selling graded. Shows deslab vs graded net, advantage, and ROI.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		if e.arbSvc == nil {
			return "", fmt.Errorf("arbitrage service not available")
		}
		result, err := e.arbSvc.GetCrackOpportunities(ctx)
		if err != nil {
			return "", err
		}
		if len(result) > 20 {
			result = result[:20]
		}
		return toJSON(result), nil
	})
}
