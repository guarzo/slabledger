package advisortool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// toolHandler is a function that executes a tool and returns JSON.
type toolHandler func(ctx context.Context, args string) (string, error)

// CampaignToolExecutor implements ai.ToolExecutor by calling campaigns.Service methods.
type CampaignToolExecutor struct {
	svc      campaigns.Service
	handlers map[string]toolHandler
	defs     []ai.ToolDefinition
}

// NewCampaignToolExecutor creates a ToolExecutor backed by the campaigns service.
func NewCampaignToolExecutor(svc campaigns.Service) *CampaignToolExecutor {
	e := &CampaignToolExecutor{
		svc:      svc,
		handlers: make(map[string]toolHandler),
	}
	e.registerTools()
	return e
}

var _ ai.FilteredToolExecutor = (*CampaignToolExecutor)(nil)

func (e *CampaignToolExecutor) Execute(ctx context.Context, toolName string, arguments string) (string, error) {
	handler, ok := e.handlers[toolName]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
	return handler(ctx, arguments)
}

func (e *CampaignToolExecutor) Definitions() []ai.ToolDefinition {
	return e.defs
}

// DefinitionsFor returns definitions for only the named tools.
func (e *CampaignToolExecutor) DefinitionsFor(names []string) []ai.ToolDefinition {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var result []ai.ToolDefinition
	for _, d := range e.defs {
		if want[d.Name] {
			result = append(result, d)
		}
	}
	return result
}

func (e *CampaignToolExecutor) register(def ai.ToolDefinition, handler toolHandler) {
	e.defs = append(e.defs, def)
	e.handlers[def.Name] = handler
}

// emptyObjectParams is a valid JSON Schema for tools that take no parameters.
// The Responses API requires "properties" to be present even if empty.
var emptyObjectParams = jsonSchema{
	Type:       "object",
	Properties: map[string]jsonSchema{},
}

// campaignIDParams is the shared JSON Schema for tools that take a single campaignId.
var campaignIDParams = jsonSchema{
	Type: "object",
	Properties: map[string]jsonSchema{
		"campaignId": {Type: "string", Description: "Campaign ID"},
	},
	Required: []string{"campaignId"},
}

// registerCampaignTool registers a tool that takes a campaignId and calls a service method.
func (e *CampaignToolExecutor) registerCampaignTool(name, description string, fn func(ctx context.Context, id string) (any, error)) {
	e.register(ai.ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  campaignIDParams,
	}, func(ctx context.Context, args string) (string, error) {
		id, err := parseCampaignID(args)
		if err != nil {
			return "", err
		}
		result, err := fn(ctx, id)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

// toJSON marshals a value to JSON, returning an error JSON on failure.
// Large results are truncated to keep the LLM context manageable.
// When truncating, array elements are removed to ensure valid JSON output.
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		errResult, _ := json.Marshal(struct { //nolint:errcheck // struct with string always marshals
			Error string `json:"error"`
		}{Error: fmt.Sprintf("marshal failed: %s", err)})
		return string(errResult)
	}
	const maxLen = 30000
	if len(b) <= maxLen {
		return string(b)
	}

	// Try to truncate by removing array elements from the JSON.
	truncated := truncateJSON(b, maxLen)
	if truncated != nil {
		return string(truncated)
	}

	// Fallback: return metadata without broken JSON.
	result, _ := json.Marshal(struct { //nolint:errcheck
		Truncated      bool   `json:"truncated"`
		OriginalLength int    `json:"original_length"`
		Message        string `json:"message"`
	}{
		Truncated:      true,
		OriginalLength: len(b),
		Message:        "Result too large to include. Use more specific tool calls to narrow the data.",
	})
	return string(result)
}

// truncateJSON attempts to reduce JSON size by halving array elements.
// Returns nil if it can't produce valid JSON under maxBytes.
func truncateJSON(b []byte, maxBytes int) []byte {
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}

	switch val := raw.(type) {
	case []any:
		return truncateSlice(val, maxBytes)
	case map[string]any:
		return truncateMapArrays(val, maxBytes)
	default:
		return nil
	}
}

func truncateSlice(s []any, maxBytes int) []byte {
	for len(s) > 1 {
		s = s[:len(s)/2]
		b, err := json.Marshal(s)
		if err == nil && len(b) <= maxBytes {
			return b
		}
	}
	return nil
}

func truncateMapArrays(m map[string]any, maxBytes int) []byte {
	// Find the largest array field.
	var largestKey string
	var largestLen int
	for k, v := range m {
		if arr, ok := v.([]any); ok && len(arr) > largestLen {
			largestKey = k
			largestLen = len(arr)
		}
	}
	if largestKey == "" {
		return nil
	}

	arr := m[largestKey].([]any) //nolint:errcheck // type-checked above
	for len(arr) > 1 {
		arr = arr[:len(arr)/2]
		m[largestKey] = arr
		b, err := json.Marshal(m)
		if err == nil && len(b) <= maxBytes {
			return b
		}
	}
	return nil
}

// parseCampaignID extracts and validates a campaignId from JSON args.
func parseCampaignID(args string) (string, error) {
	var p struct {
		CampaignID string `json:"campaignId"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.CampaignID == "" {
		return "", fmt.Errorf("campaignId is required")
	}
	return p.CampaignID, nil
}

// registerTools registers all available tools.
func (e *CampaignToolExecutor) registerTools() {
	e.registerListCampaigns()
	e.registerGetCampaignPNL()
	e.registerGetPNLByChannel()
	e.registerGetCampaignTuning()
	e.registerGetInventoryAging()
	e.registerGetGlobalInventory()
	e.registerGetSellSheet()
	e.registerGetPortfolioHealth()
	e.registerGetPortfolioInsights()
	e.registerGetCreditSummary()
	e.registerGetWeeklyReview()
	e.registerGetCapitalTimeline()
	e.registerGetExpectedValues()
	e.registerGetCrackCandidates()
	e.registerGetCampaignSuggestions()
	e.registerRunProjection()
	e.registerGetChannelVelocity()
	e.registerGetCertLookup()
	e.registerEvaluatePurchase()
	e.registerSuggestPrice()
	e.registerGetSuggestionStats()
}

// --- Tool registrations ---

func (e *CampaignToolExecutor) registerListCampaigns() {
	e.register(ai.ToolDefinition{
		Name:        "list_campaigns",
		Description: "List all campaigns with their parameters, phase, and basic stats. Use activeOnly=false to include closed campaigns.",
		Parameters: jsonSchema{
			Type: "object",
			Properties: map[string]jsonSchema{
				"activeOnly": {Type: "boolean", Description: "If true, only return active campaigns. Default false."},
			},
		},
	}, func(ctx context.Context, args string) (string, error) {
		var p struct {
			ActiveOnly bool `json:"activeOnly"`
		}
		// activeOnly is optional — default false is correct when absent or malformed.
		_ = json.Unmarshal([]byte(args), &p) //nolint:errcheck
		result, err := e.svc.ListCampaigns(ctx, p.ActiveOnly)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetCampaignPNL() {
	e.registerCampaignTool("get_campaign_pnl",
		"Get P&L summary for a campaign: total spend, revenue, fees, net profit, ROI, sell-through rate, avg days to sell.",
		func(ctx context.Context, id string) (any, error) { return e.svc.GetCampaignPNL(ctx, id) })
}

func (e *CampaignToolExecutor) registerGetPNLByChannel() {
	e.registerCampaignTool("get_pnl_by_channel",
		"Get P&L broken down by sale channel (eBay, GameStop, etc.) for a campaign.",
		func(ctx context.Context, id string) (any, error) { return e.svc.GetPNLByChannel(ctx, id) })
}

func (e *CampaignToolExecutor) registerGetCampaignTuning() {
	e.registerCampaignTool("get_campaign_tuning",
		"Get comprehensive tuning data: performance by grade, by price tier, buy threshold analysis, market alignment, top/bottom performers, and algorithmic recommendations.",
		func(ctx context.Context, id string) (any, error) { return e.svc.GetCampaignTuning(ctx, id) })
}

func (e *CampaignToolExecutor) registerGetInventoryAging() {
	e.registerCampaignTool("get_inventory_aging",
		"Get unsold cards for a campaign with days held, current market snapshot, market signal (rising/falling/stable), and price anomaly flags.",
		func(ctx context.Context, id string) (any, error) { return e.svc.GetInventoryAging(ctx, id) })
}

func (e *CampaignToolExecutor) registerGetGlobalInventory() {
	e.register(ai.ToolDefinition{
		Name:        "get_global_inventory",
		Description: "Get all unsold cards across all campaigns with aging, market signals, and recommended channels.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetGlobalInventoryAging(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

func (e *CampaignToolExecutor) registerGetSellSheet() {
	e.register(ai.ToolDefinition{
		Name:        "get_sell_sheet",
		Description: "Get the global sell sheet: target sell price, minimum acceptable price, and recommended channel for each unsold card.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GenerateGlobalSellSheet(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}

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

// jsonSchema is a minimal JSON Schema representation for tool parameters.
type jsonSchema struct {
	Type        string                `json:"type"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]jsonSchema `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
}
