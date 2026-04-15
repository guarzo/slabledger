package dh

// BatchAnalyticsResponse is returned from POST /enterprise/cards/batch_analytics.
// DH's 200 body carries per-card results with optional per-card `error` fields
// for cards whose analytics haven't been computed yet; the method returns the
// full envelope so callers can see which specific cards failed.
type BatchAnalyticsResponse struct {
	Results []CardAnalytics `json:"results"`
}

// CardAnalytics is a single per-card entry in a batch analytics response.
// Any of velocity/trend/saturation/price_distribution may be nil depending on
// the `fields` requested and whether DH has computed that surface yet.
// If Error is non-empty the other surfaces are typically nil.
type CardAnalytics struct {
	CardID            int                       `json:"card_id"`
	CardName          string                    `json:"card_name,omitempty"`
	SetName           string                    `json:"set_name,omitempty"`
	ComputedAt        string                    `json:"computed_at,omitempty"`
	Velocity          *VelocityMetrics          `json:"velocity,omitempty"`
	Trend             *TrendMetrics             `json:"trend,omitempty"`
	Saturation        *SaturationMetrics        `json:"saturation,omitempty"`
	PriceDistribution *PriceDistributionMetrics `json:"price_distribution,omitempty"`
	Error             string                    `json:"error,omitempty"`
}

// VelocityMetrics holds days-to-sell and sell-through for a single card or character.
// Numeric fields come back as strings from DH (e.g. "12.5") so they are typed
// as strings here and left for the caller to parse. sell_through values are
// also strings keyed by window.
type VelocityMetrics struct {
	MedianDaysToSell  string                      `json:"median_days_to_sell"`
	AvgDaysToSell     string                      `json:"avg_days_to_sell"`
	SellThrough       map[string]string           `json:"sell_through"`
	SampleSize        int                         `json:"sample_size"`
	ByGrade           map[string]VelocityTierStat `json:"by_grade,omitempty"`
	ByPriceTier       map[string]VelocityTierStat `json:"by_price_tier,omitempty"`
	VelocityChangePct *float64                    `json:"velocity_change_pct,omitempty"`
}

// VelocityTierStat is a per-grade or per-price-tier velocity slice.
type VelocityTierStat struct {
	MedianDays float64 `json:"median_days"`
	SampleSize int     `json:"sample_size"`
}

// TrendMetrics holds directional price trend over rolling windows.
type TrendMetrics struct {
	Direction7d  string `json:"direction_7d"`
	ChangePct7d  string `json:"change_pct_7d"`
	Volume7d     int    `json:"volume_7d"`
	Direction30d string `json:"direction_30d"`
	ChangePct30d string `json:"change_pct_30d"`
	Volume30d    int    `json:"volume_30d"`
	Direction90d string `json:"direction_90d"`
	ChangePct90d string `json:"change_pct_90d"`
	Volume90d    int    `json:"volume_90d"`
}

// SaturationMetrics holds supply-side saturation metrics for a card or character.
type SaturationMetrics struct {
	ActiveListingCount int                            `json:"active_listing_count"`
	ActiveAskCount     int                            `json:"active_ask_count"`
	PriceSpreadPct     any                            `json:"price_spread_pct,omitempty"`
	LowestAskPrice     any                            `json:"lowest_ask_price,omitempty"`
	HighestAskPrice    any                            `json:"highest_ask_price,omitempty"`
	ByGrade            map[string]SaturationGradeStat `json:"by_grade,omitempty"`
}

// SaturationGradeStat is a per-grade saturation slice.
type SaturationGradeStat struct {
	AskCount     int `json:"ask_count"`
	ListingCount int `json:"listing_count"`
}

// PriceDistributionMetrics is keyed by grade/condition and holds
// min/median/avg/max price statistics for that slice.
type PriceDistributionMetrics map[string]PriceDistributionBucket

// PriceDistributionBucket is the per-grade distribution shape.
type PriceDistributionBucket struct {
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	Median     float64 `json:"median"`
	Avg        float64 `json:"avg"`
	SampleSize int     `json:"sample_size"`
}

// DemandSignalsResponse is returned from GET /market/demand_signals.
type DemandSignalsResponse struct {
	DemandSignals []DemandSignal `json:"demand_signals"`
}

// DemandSignal is a single per-card demand entry.
type DemandSignal struct {
	CardID                   int      `json:"card_id"`
	DemandScore              float64  `json:"demand_score"`
	DataQuality              string   `json:"data_quality"`
	Views                    *int     `json:"views,omitempty"`
	SearchClicks             *int     `json:"search_clicks,omitempty"`
	WishlistAdds             *int     `json:"wishlist_adds,omitempty"`
	AvgDailySales            *float64 `json:"avg_daily_sales,omitempty"`
	ActiveBids               *int     `json:"active_bids,omitempty"`
	HighestBidPctOfAsk       *float64 `json:"highest_bid_pct_of_ask,omitempty"`
	DemandPressure7d         *float64 `json:"demand_pressure_7d,omitempty"`
	SupplySaturationShift    *float64 `json:"supply_saturation_shift,omitempty"`
	EbayActiveListingCount   *int     `json:"ebay_active_listing_count,omitempty"`
	EbayActiveListing7dAgo   *int     `json:"ebay_active_listing_count_7d_ago,omitempty"`
	RawSales24h              int      `json:"raw_sales_24h"`
	RawAvgDailySales7d       *float64 `json:"raw_avg_daily_sales_7d,omitempty"`
}

// CharacterDemandResponse is returned from
// GET /market/demand_signals/character_demand.
type CharacterDemandResponse struct {
	CharacterDemand []CharacterDemandEntry `json:"character_demand"`
}

// CharacterDemandEntry is a single character-aggregated demand row.
type CharacterDemandEntry struct {
	CharacterName     string                 `json:"character_name"`
	CardCount         int                    `json:"card_count"`
	AvgDemandScore    float64                `json:"avg_demand_score"`
	TotalViews        int                    `json:"total_views"`
	TotalSearchClicks int                    `json:"total_search_clicks"`
	TotalWishlistAdds int                    `json:"total_wishlist_adds"`
	ByEra             map[string]ByEraEntry  `json:"by_era,omitempty"`
}

// ByEraEntry is the per-era breakdown inside a character demand entry.
type ByEraEntry struct {
	CardCount         int     `json:"card_count"`
	AvgDemandScore    float64 `json:"avg_demand_score"`
	TotalViews        int     `json:"total_views"`
	TotalSearchClicks int     `json:"total_search_clicks"`
	TotalWishlistAdds int     `json:"total_wishlist_adds"`
}

// TopCharactersResponse is returned from
// GET /market/demand_signals/top_characters.
type TopCharactersResponse struct {
	CharacterDemand []CharacterDemandEntry `json:"character_demand"`
	ComputedAt      string                 `json:"computed_at"`
	Era             *string                `json:"era"`
	Limit           int                    `json:"limit"`
}

// CharacterListOpts is the shared query shape for /characters/velocity and
// /characters/saturation. Fields not set are omitted from the request.
type CharacterListOpts struct {
	SortBy  string
	SortDir string
	Page    int
	PerPage int
	// Era is accepted by /characters/leaderboard — not by velocity/saturation —
	// but is kept on this shared opts struct for ergonomics; the list endpoints
	// simply ignore it.
	Era string
}

// CharacterVelocityResponse is returned from GET /characters/velocity.
type CharacterVelocityResponse struct {
	Characters []CharacterVelocityEntry `json:"characters"`
	Pagination PaginationMeta           `json:"pagination"`
}

// CharacterVelocityEntry is one row in CharacterVelocityResponse.
type CharacterVelocityEntry struct {
	CharacterName string                  `json:"character_name"`
	CardCount     int                     `json:"card_count"`
	ComputedAt    string                  `json:"computed_at"`
	Velocity      CharacterVelocityFields `json:"velocity"`
}

// CharacterVelocityFields is the velocity block for a character row.
// All numeric fields in character-level responses come back as float64
// (unlike card-level which are strings).
type CharacterVelocityFields struct {
	MedianDaysToSell  float64                     `json:"median_days_to_sell"`
	AvgDaysToSell     float64                     `json:"avg_days_to_sell"`
	SellThrough       map[string]float64          `json:"sell_through"`
	SampleSize        int                         `json:"sample_size"`
	ByGrade           map[string]VelocityTierStat `json:"by_grade,omitempty"`
	ByPriceTier       map[string]VelocityTierStat `json:"by_price_tier,omitempty"`
	VelocityChangePct float64                     `json:"velocity_change_pct"`
}

// CharacterSaturationResponse is returned from GET /characters/saturation.
type CharacterSaturationResponse struct {
	Characters []CharacterSaturationEntry `json:"characters"`
	Pagination PaginationMeta             `json:"pagination"`
}

// CharacterSaturationEntry is one row in CharacterSaturationResponse.
type CharacterSaturationEntry struct {
	CharacterName string                    `json:"character_name"`
	CardCount     int                       `json:"card_count"`
	ComputedAt    string                    `json:"computed_at"`
	Saturation    CharacterSaturationFields `json:"saturation"`
}

// CharacterSaturationFields is the saturation block for a character row.
type CharacterSaturationFields struct {
	ActiveListingCount int                            `json:"active_listing_count"`
	ActiveAskCount     int                            `json:"active_ask_count"`
	PriceSpreadPct     float64                        `json:"price_spread_pct"`
	LowestAskPrice     float64                        `json:"lowest_ask_price"`
	HighestAskPrice    float64                        `json:"highest_ask_price"`
	ByGrade            map[string]SaturationGradeStat `json:"by_grade,omitempty"`
}

// LeaderboardResponse is returned from GET /characters/leaderboard.
type LeaderboardResponse struct {
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
	Metric      string             `json:"metric"`
	Filters     map[string]any     `json:"filters"`
	Pagination  PaginationMeta     `json:"pagination"`
}

// LeaderboardEntry is a single ranked character.
type LeaderboardEntry struct {
	Rank          int     `json:"rank"`
	CharacterName string  `json:"character_name"`
	CardCount     int     `json:"card_count"`
	Value         float64 `json:"value"`
	Metric        string  `json:"metric"`
}
