package fusionprice

import (
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// detailsCacheKey returns the cache key for per-card grade detail data.
func detailsCacheKey(card pricing.Card) string {
	return "details:" + card.Set + ":" + card.Name + ":" + card.Number
}

// observabilitySourceName maps adapter source names to short codes used by
// fetchFromAvailableSources for metrics labels, log fields, and collector/telemetry tags.
// Update this map when adding a new secondary source or changing instrumentation.
var observabilitySourceName = map[string]string{}
