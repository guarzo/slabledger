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

// --- Leaderboard output types ---

// NicheOpportunity is a single bucket on the niche leaderboard, scored on
// three axes: demand, market, and coverage.
type NicheOpportunity struct {
	Character        string
	Era              string // DH enum value, e.g. "sword_shield"; empty for "all eras"
	Grade            int    // PSA grade, or 0 for raw / unspecified
	Demand           *NicheDemand
	Market           *NicheMarket
	Acceleration     *NicheAcceleration // populated when velocity_change_pct is available
	Coverage         NicheCoverage
	OpportunityScore float64
}

// NicheAcceleration is the market-acceleration summary for a niche bucket.
// Nil when no character cache row with a parseable velocity_change_pct
// matched the niche.
//
// When attached to a NicheOpportunity (leaderboard use), the bucket is always
// a single character/grade pair, so TotalCount is always 1 and
// AcceleratingCount is 0 or 1. These fields exist to mirror the shape of
// CampaignSignal (where the counts aggregate across many characters) so that
// callers can use the same field names across both surfaces.
type NicheAcceleration struct {
	MedianVelocityChangePct float64
	AcceleratingCount       int
	TotalCount              int
	DataQuality             string // "full" when any contributor has full data
	ComputedAt              *time.Time
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
	AvgDailySales        *float64
	SellThroughRate30d   *float64
	SalesVolume7d        *int
	SalesVolume30d       *int
	SupplyCount          *int
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
