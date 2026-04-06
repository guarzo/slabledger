package advisortool

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

func (e *CampaignToolExecutor) registerGetFlaggedInventory() {
	e.register(ai.ToolDefinition{
		Name:        "get_flagged_inventory",
		Description: "Get unsold cards that have inventory signals: profit capture opportunities, stale listings, deslab candidates, or markdown flags. Returns only actionable cards, not the full inventory.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetFlaggedInventory(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}
