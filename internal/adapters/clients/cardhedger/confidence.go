package cardhedger

const (
	// ConfidenceRejectThreshold is the minimum confidence for a card-match result.
	// Matches below this are rejected entirely.
	ConfidenceRejectThreshold = 0.5

	// ConfidenceCacheThreshold is the minimum confidence to cache a card-match mapping.
	// Matches at or above this are persisted; those between Reject and Cache are used
	// transiently but not stored.
	ConfidenceCacheThreshold = 0.7
)

// ShouldRejectMatch returns true if the confidence is too low to use.
func ShouldRejectMatch(confidence float64) bool {
	return confidence < ConfidenceRejectThreshold
}

// ShouldCacheMatch returns true if the confidence is high enough to persist.
func ShouldCacheMatch(confidence float64) bool {
	return confidence >= ConfidenceCacheThreshold
}
