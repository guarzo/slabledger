package advisortool

import (
	"context"
	"encoding/json"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

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
		Description: "Get all unsold cards across all campaigns with aging, market signals, recommended channels, and compDigest with recent sales comp analytics when available.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetGlobalInventoryAging(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(struct {
			Items    any      `json:"items"`
			Warnings []string `json:"warnings,omitempty"`
		}{
			Items:    withCompDigests(result.Items),
			Warnings: result.Warnings,
		}), nil
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
