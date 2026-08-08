package demand

// CharacterDemand is the character-aggregated demand payload persisted on
// CharacterCache.
type CharacterDemand struct {
	CharacterName     string  `json:"character_name"`
	CardCount         int     `json:"card_count"`
	AvgDemandScore    float64 `json:"avg_demand_score"`
	TotalViews        int     `json:"total_views"`
	TotalSearchClicks int     `json:"total_search_clicks"`
	TotalWishlistAdds int     `json:"total_wishlist_adds"`
	// DataQuality is read by qualityAllowed but has no writer: DH's
	// character demand endpoint does not report it. Character niches
	// therefore always carry "". Tracked as SLA-61.
	DataQuality string                 `json:"data_quality"`
	ComputedAt  string                 `json:"computed_at"`
	ByEra       map[string]ByEraDemand `json:"by_era,omitempty"`
}

type ByEraDemand struct {
	CardCount         int     `json:"card_count"`
	AvgDemandScore    float64 `json:"avg_demand_score"`
	TotalViews        int     `json:"total_views"`
	TotalSearchClicks int     `json:"total_search_clicks"`
	TotalWishlistAdds int     `json:"total_wishlist_adds"`
}

type CharacterVelocity struct {
	MedianDaysToSell   *float64                    `json:"median_days_to_sell,omitempty"`
	SampleSize         int                         `json:"sample_size"`
	VelocityChangePct  *float64                    `json:"velocity_change_pct,omitempty"`
	AvgDailySales      *float64                    `json:"avg_daily_sales,omitempty"`
	SellThroughRate30d *float64                    `json:"sell_through_rate_30d,omitempty"`
	SalesVolume7d      *int                        `json:"sales_volume_7d,omitempty"`
	SalesVolume30d     *int                        `json:"sales_volume_30d,omitempty"`
	SupplyCount        *int                        `json:"supply_count,omitempty"`
	ByGrade            map[string]VelocityTierStat `json:"by_grade,omitempty"`
	ByPriceTier        map[string]VelocityTierStat `json:"by_price_tier,omitempty"`
}

type VelocityTierStat struct {
	MedianDays float64 `json:"median_days"`
	SampleSize int     `json:"sample_size"`
}

type CharacterSaturation struct {
	ActiveListingCount int    `json:"active_listing_count"`
	ComputedAt         string `json:"computed_at"`
}
