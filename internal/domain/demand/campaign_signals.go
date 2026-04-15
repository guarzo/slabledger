package demand

import "time"

// Acceleration thresholds used by CampaignSignals. Tune by code edit, not
// config — these are calibration constants, not user preferences.
const (
	// AccelerationThresholdPct is the minimum velocity_change_pct a character
	// must exhibit (over DH's 7d window) to count as "accelerating".
	AccelerationThresholdPct = 5.0
	// DecelerationThresholdPct is the maximum velocity_change_pct a character
	// may exhibit before being counted as "decelerating". Negative value.
	DecelerationThresholdPct = -5.0
	// TopContributorsLimit caps the TopAccelerating / TopDecelerating arrays
	// per campaign. Keeps response size bounded.
	TopContributorsLimit = 5
)

// DataQuality values returned by CampaignSignals.
//
// Market analytics either have velocity data or they don't — there's no
// "proxy" tier for this surface (that concept is specific to demand signals).
const (
	DataQualityFull  = "full"
	DataQualityEmpty = "empty"
)

// CampaignSignalsResponse is the return of Service.CampaignSignals.
type CampaignSignalsResponse struct {
	// ComputedAt is the min analytics_computed_at across all contributing rows,
	// or nil when the response is empty.
	ComputedAt *time.Time
	// DataQuality is "full" when at least one signal has contributors, else "empty".
	DataQuality string
	// Signals contains one entry per active campaign with at least one tracked
	// character. Campaigns whose character set doesn't intersect the cache are
	// omitted entirely — not included with TrackedCharacters=0.
	Signals []CampaignSignal
}

// CampaignSignal summarises DH market acceleration for a single campaign's
// character slice.
type CampaignSignal struct {
	CampaignID              int64
	CampaignName            string
	TrackedCharacters       int // Contributing characters (those with parseable velocity_change_pct).
	AcceleratingCount       int // Subset where velocity_change_pct >= AccelerationThresholdPct.
	DeceleratingCount       int // Subset where velocity_change_pct <= DecelerationThresholdPct.
	MedianVelocityChangePct float64
	DataQuality             string // Always "full" when TrackedCharacters > 0.
	ComputedAt              *time.Time
	// TopAccelerating is sorted by velocity_change_pct desc, capped at TopContributorsLimit.
	TopAccelerating []CampaignSignalContributor
	// TopDecelerating is sorted by velocity_change_pct asc, capped at TopContributorsLimit.
	TopDecelerating []CampaignSignalContributor
}

// CampaignSignalContributor is a single character's contribution to a signal.
type CampaignSignalContributor struct {
	Character         string   // Display name (original casing from the cache row).
	VelocityChangePct float64
	MedianDaysToSell  *float64 // Nil if DH's string value failed to parse.
	SampleSize        int
}
