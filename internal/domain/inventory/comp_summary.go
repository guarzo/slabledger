package inventory

import "context"

// CompSummary holds aggregated sales comp analytics for a card variant (gemRateID).
type CompSummary struct {
	GemRateID      string              `json:"gemRateId"`
	TotalComps     int                 `json:"totalComps"`
	RecentComps    int                 `json:"recentComps"`
	MedianCents    int                 `json:"medianCents"`
	HighestCents   int                 `json:"highestCents"`
	LowestCents    int                 `json:"lowestCents"`
	Trend90d       float64             `json:"trend90d"`
	CompsAboveCL   int                 `json:"compsAboveCL"`
	CompsAboveCost int                 `json:"compsAboveCost"`
	ByPlatform     []PlatformBreakdown `json:"byPlatform"`
	LastSaleDate   string              `json:"lastSaleDate"`
	PriceCentsList []int               `json:"-"`
}

// PlatformBreakdown holds per-platform comp statistics.
type PlatformBreakdown struct {
	Platform    string `json:"platform"`
	SaleCount   int    `json:"saleCount"`
	MedianCents int    `json:"medianCents"`
	HighCents   int    `json:"highCents"`
	LowCents    int    `json:"lowCents"`
}

// CompSummaryProvider computes comp analytics for a card variant at a specific grade.
type CompSummaryProvider interface {
	// GetCompSummary returns aggregated comp data for a gemRateID filtered by grade.
	// certNumber resolves the CL condition (grade) from the card mapping table so comps
	// are grade-specific (e.g., PSA 10 only, not mixed with PSA 9).
	// CompsAboveCL and CompsAboveCost are left at 0 — the caller derives them per-purchase
	// from PriceCentsList since different purchases may have different CL values and costs.
	GetCompSummary(ctx context.Context, gemRateID, certNumber string) (*CompSummary, error)
}

// CountAboveCost returns how many prices in the list exceed the given cost.
func CountAboveCost(prices []int, costCents int) int {
	count := 0
	for _, p := range prices {
		if p > costCents {
			count++
		}
	}
	return count
}
