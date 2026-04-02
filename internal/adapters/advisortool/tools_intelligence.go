package advisortool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

func (e *CampaignToolExecutor) registerGetMarketIntelligence() {
	e.register(ai.ToolDefinition{
		Name:        "get_market_intelligence",
		Description: "Get DH market intelligence for a card: sentiment, price forecast, grading ROI by grade, recent sales, population data, and AI insights.",
		Parameters: jsonSchema{
			Type: "object",
			Properties: map[string]jsonSchema{
				"cardName":   {Type: "string", Description: "Card name"},
				"setName":    {Type: "string", Description: "Set name"},
				"cardNumber": {Type: "string", Description: "Card number (optional)"},
			},
			Required: []string{"cardName", "setName"},
		},
	}, func(ctx context.Context, args string) (string, error) {
		if e.intelRepo == nil {
			return `{"error":"market intelligence not available (DH not configured)"}`, nil
		}
		var p struct {
			CardName   string `json:"cardName"`
			SetName    string `json:"setName"`
			CardNumber string `json:"cardNumber"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if p.CardName == "" {
			return "", fmt.Errorf("cardName is required")
		}
		if p.SetName == "" {
			return "", fmt.Errorf("setName is required")
		}
		intel, err := e.intelRepo.GetByCard(ctx, p.CardName, p.SetName, p.CardNumber)
		if err != nil {
			return "", err
		}
		if intel == nil {
			return `{"message":"no market intelligence found for this card"}`, nil
		}
		return toJSON(intel), nil
	})
}

func (e *CampaignToolExecutor) registerGetDHSuggestions() {
	e.register(ai.ToolDefinition{
		Name:        "get_dh_suggestions",
		Description: "Get the latest DH daily buy/sell suggestions: hottest cards to buy and cards to consider selling, with confidence scores and reasoning.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		if e.suggestRepo == nil {
			return `{"error":"DH suggestions not available (DH not configured)"}`, nil
		}
		suggestions, err := e.suggestRepo.GetLatest(ctx)
		if err != nil {
			return "", err
		}
		result := struct {
			Count       int `json:"count"`
			Suggestions any `json:"suggestions"`
		}{
			Count:       len(suggestions),
			Suggestions: suggestions,
		}
		return toJSON(result), nil
	})
}

// inventoryAlert flags a DH suggestion that matches a card in inventory.
type inventoryAlert struct {
	CardName        string  `json:"card_name"`
	SetName         string  `json:"set_name"`
	Category        string  `json:"category"`
	Reasoning       string  `json:"reasoning"`
	ConfidenceScore float64 `json:"confidence_score"`
}

func (e *CampaignToolExecutor) registerGetInventoryAlerts() {
	e.register(ai.ToolDefinition{
		Name:        "get_inventory_alerts",
		Description: "Cross-reference DH buy/sell suggestions with current inventory to find actionable alerts: cards you hold that DH recommends selling, or cards DH recommends buying that you already own.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		if e.suggestRepo == nil {
			return `{"error":"inventory alerts not available (DH not configured)"}`, nil
		}

		suggestions, err := e.suggestRepo.GetLatest(ctx)
		if err != nil {
			return "", err
		}
		if len(suggestions) == 0 {
			return `{"alerts":[],"count":0,"message":"no DH suggestions available"}`, nil
		}

		// Build inventory lookup from global inventory aging.
		aging, err := e.svc.GetGlobalInventoryAging(ctx)
		if err != nil {
			return "", fmt.Errorf("fetch inventory: %w", err)
		}

		type invKey struct{ name, set string }
		invSet := make(map[invKey]bool, len(aging))
		for _, item := range aging {
			k := invKey{
				name: strings.ToLower(item.Purchase.CardName),
				set:  strings.ToLower(item.Purchase.SetName),
			}
			invSet[k] = true
		}

		var alerts []inventoryAlert
		for _, s := range suggestions {
			k := invKey{
				name: strings.ToLower(s.CardName),
				set:  strings.ToLower(s.SetName),
			}
			if invSet[k] {
				alerts = append(alerts, inventoryAlert{
					CardName:        s.CardName,
					SetName:         s.SetName,
					Category:        s.Category,
					Reasoning:       s.Reasoning,
					ConfidenceScore: s.ConfidenceScore,
				})
			}
		}

		result := struct {
			Count  int              `json:"count"`
			Alerts []inventoryAlert `json:"alerts"`
		}{
			Count:  len(alerts),
			Alerts: alerts,
		}
		return toJSON(result), nil
	})
}
