package demand

import "context"

// Repository persists and retrieves the cached DH demand and analytics rows
// that back the niche-opportunity leaderboard. The SQLite adapter
// (internal/adapters/storage/sqlite) implements this interface.
type Repository interface {
	// Card cache
	UpsertCardCache(ctx context.Context, row CardCache) error
	GetCardCache(ctx context.Context, cardID, window string) (*CardCache, error)
	ListCardCacheByDemandScore(ctx context.Context, window string, limit int) ([]CardCache, error)
	CardDataQualityStats(ctx context.Context, window string) (QualityStats, error)

	// Character cache
	UpsertCharacterCache(ctx context.Context, row CharacterCache) error
	GetCharacterCache(ctx context.Context, character, window string) (*CharacterCache, error)
	ListCharacterCache(ctx context.Context, window string) ([]CharacterCache, error)
}

// ActiveCampaign describes a single active campaign's targeting rules, used
// by the campaign-signals service to correlate per-campaign market data.
// Kept minimal — only the fields needed to filter characters and grades.
type ActiveCampaign struct {
	ID            int64 // Numeric ID only; non-numeric IDs (e.g. "external") are excluded.
	Name          string
	GradeRange    string // e.g. "9-10"; empty means no grade constraint.
	InclusionList string // Comma-separated; empty means open-net.
	ExclusionMode bool   // If true, InclusionList is an exclusion list.
}

// ActiveCampaignSource is the narrow interface used by CampaignSignals to
// enumerate active campaigns. Separating it from CampaignCoverageLookup
// makes the two access patterns explicit: per-niche indexed lookup (leaderboard)
// vs. full table scan (campaign signals).
type ActiveCampaignSource interface {
	// ActiveCampaigns returns all campaigns with Phase="active". Campaigns with
	// non-numeric IDs are omitted (the ID field only holds int64). Returns an
	// empty slice when there are no active campaigns.
	ActiveCampaigns(ctx context.Context) ([]ActiveCampaign, error)
}

// CampaignCoverageLookup answers coverage questions for a niche bucket
// (character, era, grade). The real implementation is wired in T5/T6 against
// the campaigns store; for now this interface is the seam the Service depends
// on so it can be fully unit-tested.
//
// It embeds ActiveCampaignSource so the same concrete type can satisfy both
// interfaces without two separate injection points on Service.
type CampaignCoverageLookup interface {
	ActiveCampaignSource

	// CampaignsCovering returns active campaign IDs whose inclusion rules match
	// the given (character, era, grade) triple. An empty slice means no campaign
	// currently targets this niche.
	CampaignsCovering(ctx context.Context, character, era string, grade int) ([]int64, error)

	// UnsoldCountFor returns the count of our unsold inventory matching the
	// bucket. Zero means the niche is uncovered by our holdings.
	UnsoldCountFor(ctx context.Context, character, era string, grade int) (int, error)
}
