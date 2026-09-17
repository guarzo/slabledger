package showprep

const PriceAssessmentPolicy = "recent-sales-v1"

type RecentPriceEvidence struct {
	WindowStart        string   `json:"windowStart"`
	WindowEnd          string   `json:"windowEnd"`
	SaleIDs            []string `json:"saleIds"`
	Count              int      `json:"count"`
	MedianCents        int      `json:"medianCents"`
	LatestSaleDate     string   `json:"latestSaleDate"`
	LatestSaleCount    int      `json:"latestSaleCount"`
	LatestSaleMinCents int      `json:"latestSaleMinCents"`
	LatestSaleMaxCents int      `json:"latestSaleMaxCents"`
	GapPct             *float64 `json:"gapPct"`
}
