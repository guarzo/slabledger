package advisortool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/scoring"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// toolHandler is a function that executes a tool and returns JSON.
type toolHandler func(ctx context.Context, args string) (string, error)

// CampaignToolExecutor implements ai.ToolExecutor by calling inventory.Service methods.
type CampaignToolExecutor struct {
	svc            inventory.Service
	financeService finance.Service
	exportService  export.Service
	arbSvc         arbitrage.Service
	portSvc        portfolio.Service
	tuningSvc      tuning.Service
	intelRepo      intelligence.Repository
	suggestRepo    intelligence.SuggestionsRepository
	gapStore       scoring.GapStore
	handlers       map[string]toolHandler
	defs           []ai.ToolDefinition
}

// ExecutorOption configures optional dependencies on CampaignToolExecutor.
type ExecutorOption func(*CampaignToolExecutor)

// WithIntelligenceRepo injects the market intelligence repository.
func WithIntelligenceRepo(repo intelligence.Repository) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.intelRepo = repo }
}

// WithSuggestionsRepo injects the DH suggestions repository.
func WithSuggestionsRepo(repo intelligence.SuggestionsRepository) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.suggestRepo = repo }
}

// WithGapStore injects the scoring gap store for data gap reports.
func WithGapStore(gs scoring.GapStore) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.gapStore = gs }
}

// WithArbitrageService injects the arbitrage service.
func WithArbitrageService(svc arbitrage.Service) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.arbSvc = svc }
}

// WithPortfolioService injects the portfolio service.
func WithPortfolioService(svc portfolio.Service) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.portSvc = svc }
}

// WithTuningService injects the tuning service.
func WithTuningService(svc tuning.Service) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.tuningSvc = svc }
}

// WithFinanceService injects the finance service.
func WithFinanceService(svc finance.Service) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.financeService = svc }
}

// WithExportService injects the export service.
func WithExportService(svc export.Service) ExecutorOption {
	return func(e *CampaignToolExecutor) { e.exportService = svc }
}

// NewCampaignToolExecutor creates a ToolExecutor backed by the campaigns service.
func NewCampaignToolExecutor(svc inventory.Service, opts ...ExecutorOption) *CampaignToolExecutor {
	e := &CampaignToolExecutor{
		svc:      svc,
		handlers: make(map[string]toolHandler),
	}
	for _, opt := range opts {
		opt(e)
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
	const maxLen = 15000
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
// Tool registration order affects tool numbering in AI streaming responses.
// Tools are registered in this order:
//
//  1. list_campaigns               — list all campaigns
//  2. get_campaign_pnl             — single-campaign P&L
//  3. get_pnl_by_channel           — P&L by sale channel
//  4. get_campaign_tuning          — tuning suggestions
//  5. get_inventory_aging          — aging report with market signals
//  6. get_global_inventory         — unsold inventory across campaigns
//  7. get_flagged_inventory        — flagged (revocation/review) items
//  8. get_sell_sheet               — exportable sell sheet
//  9. get_portfolio_health         — campaign health scores
//  10. get_portfolio_insights      — portfolio segmentation + coverage gaps
//  11. get_capital_summary         — capital exposure (requires financeService)
//  12. get_weekly_review           — week-over-week comparison
//  13. get_capital_timeline        — daily capital deployment chart
//  14. get_expected_values         — EV for a single campaign
//  15. get_deslab_candidates       — crack/deslab recommendations
//  16. get_campaign_suggestions    — DH-sourced buy suggestions
//  17. run_projection              — Monte Carlo cashflow projection
//  18. get_channel_velocity        — avg days-to-sell per channel
//  19. get_cert_lookup             — PSA cert lookup
//  20. evaluate_purchase           — evaluate a specific purchase
//  21. suggest_price               — AI price suggestion for one item
//  22. get_suggestion_stats        — AI price suggestion statistics
//  23. get_dashboard_summary       — combined dashboard view
//  24. get_acquisition_targets     — potential acquisition targets
//  25. get_deslab_opportunities    — DH deslab/crack opportunities
//  26. get_market_intelligence     — market intelligence report
//  27. get_dh_suggestions          — DH-specific suggestions
//  28. get_inventory_alerts        — inventory alert report
//  29. get_data_gap_report         — scoring data gap report (requires gapStore)
//  30. get_expected_values_batch   — EV for multiple campaigns
//  31. suggest_price_batch         — AI price suggestions in bulk
//
// Optional tools (only useful when the relevant service is injected):
//   - get_capital_summary: requires WithFinanceService
//   - get_data_gap_report: requires WithGapStore (returns error JSON if absent)
//   - get_market_intelligence: requires WithIntelligenceRepo
//   - get_dh_suggestions: requires WithSuggestionsRepo
func (e *CampaignToolExecutor) registerTools() {
	e.registerListCampaigns()
	e.registerGetCampaignPNL()
	e.registerGetPNLByChannel()
	e.registerGetCampaignTuning()
	e.registerGetInventoryAging()
	e.registerGetGlobalInventory()
	e.registerGetFlaggedInventory()
	e.registerGetSellSheet()
	e.registerGetPortfolioHealth()
	e.registerGetPortfolioInsights()
	e.registerGetCapitalSummary()
	e.registerGetWeeklyReview()
	e.registerGetCapitalTimeline()
	e.registerGetExpectedValues()
	e.registerGetDeslabCandidates()
	e.registerGetCampaignSuggestions()
	e.registerRunProjection()
	e.registerGetChannelVelocity()
	e.registerGetCertLookup()
	e.registerEvaluatePurchase()
	e.registerSuggestPrice()
	e.registerGetSuggestionStats()
	e.registerGetDashboardSummary()
	e.registerGetAcquisitionTargets()
	e.registerGetDeslabOpportunities()
	e.registerGetMarketIntelligence()
	e.registerGetDHSuggestions()
	e.registerGetInventoryAlerts()
	e.registerGetDataGapReport()
	e.registerGetExpectedValuesBatch()
	e.registerSuggestPriceBatch()
}

// registerGetDataGapReport registers the get_data_gap_report tool.
func (e *CampaignToolExecutor) registerGetDataGapReport() {
	e.register(ai.ToolDefinition{
		Name:        "get_data_gap_report",
		Description: "Get a report of scoring data gaps over the last 7 days. Shows which factors are missing most often and which card sets are most affected. Use this in the digest to surface data quality issues.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, args string) (string, error) {
		if e.gapStore == nil {
			return `{"error": "gap store not configured"}`, nil
		}
		since := time.Now().AddDate(0, 0, -7)
		report, err := e.gapStore.GetGapReport(ctx, since)
		if err != nil {
			return "", err
		}
		return toJSON(report), nil
	})
}
