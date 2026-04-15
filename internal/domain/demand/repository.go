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

// CampaignCoverageLookup answers coverage questions for a niche bucket
// (character, era, grade). The real implementation is wired in T5/T6 against
// the campaigns store; for now this interface is the seam the Service depends
// on so it can be fully unit-tested.
type CampaignCoverageLookup interface {
	// CampaignsCovering returns active campaign IDs whose inclusion rules match
	// the given (character, era, grade) triple. An empty slice means no campaign
	// currently targets this niche.
	CampaignsCovering(ctx context.Context, character, era string, grade int) ([]int64, error)

	// UnsoldCountFor returns the count of our unsold inventory matching the
	// bucket. Zero means the niche is uncovered by our holdings.
	UnsoldCountFor(ctx context.Context, character, era string, grade int) (int, error)
}
