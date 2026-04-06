package advisortool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/scoring"
)

// toolHandler is a function that executes a tool and returns JSON.
type toolHandler func(ctx context.Context, args string) (string, error)

// CampaignToolExecutor implements ai.ToolExecutor by calling campaigns.Service methods.
type CampaignToolExecutor struct {
	svc         campaigns.Service
	intelRepo   intelligence.Repository
	suggestRepo intelligence.SuggestionsRepository
	gapStore    scoring.GapStore
	handlers    map[string]toolHandler
	defs        []ai.ToolDefinition
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

// NewCampaignToolExecutor creates a ToolExecutor backed by the campaigns service.
func NewCampaignToolExecutor(svc campaigns.Service, opts ...ExecutorOption) *CampaignToolExecutor {
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
	e.registerGetDashboardSummary()
	e.registerGetAcquisitionTargets()
	e.registerGetCrackOpportunities()
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
