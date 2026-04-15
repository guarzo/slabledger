// Package demand defines the domain types, repository contract, and service
// that compute niche-opportunity leaderboards from DoubleHolo (DH) market
// analytics and demand signals. It is a flat sibling under internal/domain/
// and does not import any adapter or other domain-sibling packages — DH JSON
// payloads are parsed here into domain-local structs so the scoring logic
// stays decoupled from the wire format.
package demand

import "time"

// --- Cache row types (domain-level mirrors of the SQLite rows) ---

// CardCache is the domain view of a dh_card_cache row. Nullable SQL columns
// map to pointer fields so callers can distinguish NULL from the zero value.
type CardCache struct {
	CardID                string
	Window                string // "7d" or "30d"
	DemandScore           *float64
	DemandDataQuality     *string // "proxy" | "full"
	DemandJSON            *string
	VelocityJSON          *string
	TrendJSON             *string
	SaturationJSON        *string
	PriceDistributionJSON *string
	AnalyticsComputedAt   *time.Time
	DemandComputedAt      *time.Time
	FetchedAt             time.Time
}

// CharacterCache is the domain view of a dh_character_cache row.
type CharacterCache struct {
	Character           string
	Window              string
	DemandJSON          *string
	VelocityJSON        *string
	SaturationJSON      *string
	DemandComputedAt    *time.Time
	AnalyticsComputedAt *time.Time
	FetchedAt           time.Time
}

// QualityStats summarises demand_data_quality distribution for a given
// window, used by the refresh scheduler for observability.
type QualityStats struct {
	ProxyCount       int
	FullCount        int
	NullQualityCount int // rows without demand_data_quality (analytics-only or 404'd)
	TotalRows        int
}

// --- Higher-level domain views (extracted from the cached JSON blobs) ---

// CardDemand is the numerically normalised per-card demand view.
type CardDemand struct {
	CardID       string
	Score        float64
	DataQuality  string // "proxy" | "full"
	Views        int
	WishlistAdds int
	ComputedAt   time.Time
}

// CardAnalytics is the per-card velocity/trend/saturation/price-distribution
// view, numerically normalised (DH card-level velocity returns numbers as
// strings on the wire).
type CardAnalytics struct {
	MedianDaysToSell   *float64
	VelocityChangePct  *float64
	ActiveListingCount int
	SampleSize         int
	ComputedAt         *time.Time
}

// CharacterDemand is the per-character demand aggregate, with an optional
// per-era breakdown.
type CharacterDemand struct {
	Character      string
	CardCount      int
	AvgDemandScore float64
	TotalViews     int
	TotalWishlist  int
	ByEra          map[string]ByEraDemand
	ComputedAt     time.Time
}

// ByEraDemand is the per-era slice within a CharacterDemand.
type ByEraDemand struct {
	CardCount      int
	AvgDemandScore float64
	TotalViews     int
	TotalWishlist  int
}

// CharacterAnalytics is the per-character velocity + saturation view.
type CharacterAnalytics struct {
	MedianDaysToSell   *float64
	VelocityChangePct  *float64
	ActiveListingCount int
	SampleSize         int
	ComputedAt         *time.Time
}

// --- Leaderboard output types ---

// NicheOpportunity is a single bucket on the niche leaderboard, scored on
// three axes: demand, market, and coverage.
type NicheOpportunity struct {
	Character        string
	Era              string // DH enum value, e.g. "sword_shield"; empty for "all eras"
	Grade            int    // PSA grade, or 0 for raw / unspecified
	Demand           *NicheDemand
	Market           *NicheMarket
	Coverage         NicheCoverage
	OpportunityScore float64
}

// NicheDemand is the demand-axis summary for a niche bucket.
type NicheDemand struct {
	Score        float64
	Views        int
	WishlistAdds int
	DataQuality  string // "proxy" | "full"
	ComputedAt   time.Time
}

// NicheMarket is the market-axis summary for a niche bucket. If DH hasn't
// computed analytics for this slice yet, AnalyticsNotComputed is true and
// the numeric fields are zero/nil.
type NicheMarket struct {
	MedianDaysToSell     *float64
	VelocityChangePct    *float64
	ActiveListingCount   int
	SampleSize           int
	AnalyticsNotComputed bool
	ComputedAt           *time.Time
}

// NicheCoverage describes which existing campaigns cover this bucket.
type NicheCoverage struct {
	OurUnsoldCount    int
	ActiveCampaignIDs []int64
	Covered           bool
}

// CampaignCoverage is a helper type that restates NicheCoverage for callers
// that want a standalone value (e.g. coverage lookups in tests).
type CampaignCoverage = NicheCoverage
