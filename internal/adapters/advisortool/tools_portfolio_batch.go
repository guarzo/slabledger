package advisortool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

// jsonSchema is a minimal JSON Schema representation for tool parameters.
type jsonSchema struct {
	Type        string                `json:"type"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]jsonSchema `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
	Items       *jsonSchema           `json:"items,omitempty"`
}

// maxBatchResultChars matches maxToolResultChars from the advisor service.
// Each campaign's share of this budget is maxBatchResultChars / len(campaignIds).
const maxBatchResultChars = 12_000

// maxConcurrentEVRequests limits how many GetExpectedValues calls run in parallel
// to avoid overwhelming downstream services.
const maxConcurrentEVRequests = 5

func (e *CampaignToolExecutor) registerSuggestPriceBatch() {
	e.register(ai.ToolDefinition{
		Name:        "suggest_price_batch",
		Description: "Suggest sell prices for multiple purchases in one call. Each suggestion is saved for user review. Returns per-item status.",
		Parameters: jsonSchema{
			Type: "object",
			Properties: map[string]jsonSchema{
				"suggestions": {
					Type:        "array",
					Description: "Array of price suggestions",
					Items: &jsonSchema{
						Type: "object",
						Properties: map[string]jsonSchema{
							"purchaseId": {Type: "string", Description: "Purchase ID to suggest a price for"},
							"priceCents": {Type: "integer", Description: "Suggested price in cents"},
						},
						Required: []string{"purchaseId", "priceCents"},
					},
				},
			},
			Required: []string{"suggestions"},
		},
	}, func(ctx context.Context, args string) (string, error) {
		var p struct {
			Suggestions []struct {
				PurchaseID string `json:"purchaseId"`
				PriceCents int    `json:"priceCents"`
			} `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if len(p.Suggestions) == 0 {
			return "", fmt.Errorf("suggestions array is required and must not be empty")
		}

		// Validate all items before executing any.
		for _, s := range p.Suggestions {
			if s.PurchaseID == "" {
				return "", fmt.Errorf("purchaseId is required for each suggestion")
			}
			if s.PriceCents <= 0 {
				return "", fmt.Errorf("priceCents must be positive for purchaseId %s", s.PurchaseID)
			}
		}

		type itemResult struct {
			PurchaseID string `json:"purchaseId"`
			Status     string `json:"status"`
			Error      string `json:"error,omitempty"`
		}
		results := make([]itemResult, len(p.Suggestions))
		for i, s := range p.Suggestions {
			if err := e.svc.SetAISuggestedPrice(ctx, s.PurchaseID, s.PriceCents); err != nil {
				results[i] = itemResult{PurchaseID: s.PurchaseID, Status: "error", Error: err.Error()}
			} else {
				results[i] = itemResult{PurchaseID: s.PurchaseID, Status: "ok"}
			}
		}

		resp := struct {
			Results []itemResult `json:"results"`
		}{Results: results}
		b, _ := json.Marshal(resp) //nolint:errcheck
		return string(b), nil
	})
}

func (e *CampaignToolExecutor) registerGetExpectedValuesBatch() {
	e.register(ai.ToolDefinition{
		Name:        "get_expected_values_batch",
		Description: "Get expected values for multiple campaigns in one call. Returns a map of campaignId to EV data. Omit campaignIds to get all active campaigns.",
		Parameters: jsonSchema{
			Type: "object",
			Properties: map[string]jsonSchema{
				"campaignIds": {
					Type:        "array",
					Description: "Campaign IDs to fetch EVs for. Omit for all active campaigns.",
					Items:       &jsonSchema{Type: "string"},
				},
			},
		},
	}, func(ctx context.Context, args string) (string, error) {
		var p struct {
			CampaignIDs []string `json:"campaignIds"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}

		// Default to all active campaigns if none specified.
		if len(p.CampaignIDs) == 0 {
			all, err := e.svc.ListCampaigns(ctx, true)
			if err != nil {
				return "", fmt.Errorf("list active campaigns: %w", err)
			}
			for _, c := range all {
				p.CampaignIDs = append(p.CampaignIDs, c.ID)
			}
		}
		if len(p.CampaignIDs) == 0 {
			return `{}`, nil
		}

		type evResult struct {
			id   string
			data json.RawMessage
		}

		perCampaignBudget := maxBatchResultChars / len(p.CampaignIDs)
		results := make([]evResult, len(p.CampaignIDs))
		sem := make(chan struct{}, maxConcurrentEVRequests)
		var wg sync.WaitGroup
		for i, id := range p.CampaignIDs {
			wg.Add(1)
			go func(idx int, campaignID string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				ev, err := e.svc.GetExpectedValues(ctx, campaignID)
				if err != nil {
					errJSON, _ := json.Marshal(struct { //nolint:errcheck
						Error string `json:"error"`
					}{Error: err.Error()})
					results[idx] = evResult{id: campaignID, data: errJSON}
					return
				}
				b, _ := json.Marshal(ev) //nolint:errcheck
				if len(b) > perCampaignBudget {
					if truncated := truncateJSON(b, perCampaignBudget); truncated != nil {
						b = truncated
					}
				}
				results[idx] = evResult{id: campaignID, data: b}
			}(i, id)
		}
		wg.Wait()

		merged := make(map[string]json.RawMessage, len(results))
		for _, r := range results {
			merged[r.id] = r.data
		}
		b, err := json.Marshal(merged)
		if err != nil {
			return "", fmt.Errorf("marshal batch result: %w", err)
		}
		return string(b), nil
	})
}
