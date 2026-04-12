package advisortool

import (
	"context"
	"fmt"
	"math"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func (e *CampaignToolExecutor) registerGetFlaggedInventory() {
	e.register(ai.ToolDefinition{
		Name:        "get_flagged_inventory",
		Description: "Get unsold cards that have inventory signals: profit capture opportunities, stale listings, deslab candidates, or markdown flags. Includes compDigest with recent sales comp analytics when available.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetFlaggedInventory(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(withCompDigests(result)), nil
	})
}

// agingItemWithDigest wraps AgingItem with a compact compDigest for advisor token efficiency.
type agingItemWithDigest struct {
	inventory.AgingItem
	CompDigest string `json:"compDigest,omitempty"`
}

// withCompDigests wraps items, replacing the full CompSummary with a compact one-line digest.
func withCompDigests(items []inventory.AgingItem) []agingItemWithDigest {
	out := make([]agingItemWithDigest, len(items))
	for i := range items {
		digest := compDigest(items[i].CompSummary)
		item := items[i]
		item.CompSummary = nil // exclude full summary from advisor JSON
		out[i] = agingItemWithDigest{AgingItem: item, CompDigest: digest}
	}
	return out
}

// compDigest formats a CompSummary into a compact one-line string for the advisor.
func compDigest(cs *inventory.CompSummary) string {
	if cs == nil || cs.RecentComps == 0 {
		return ""
	}
	trendStr := fmt.Sprintf("%+.0f%%", cs.Trend90d*100)
	if math.Abs(cs.Trend90d) < 0.005 {
		trendStr = "flat"
	}
	return fmt.Sprintf("%d comps (90d), median $%.0f, high $%.0f, %d/%d above CL, %d/%d above cost, trend %s",
		cs.RecentComps,
		float64(cs.MedianCents)/100,
		float64(cs.HighestCents)/100,
		cs.CompsAboveCL, cs.RecentComps,
		cs.CompsAboveCost, cs.RecentComps,
		trendStr,
	)
}
