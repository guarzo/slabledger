package fusionprice

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// buildResponseMeta converts HTTP status code and headers into transport-agnostic ResponseMeta.
// Rate limit headers are only parsed on 429 responses.
func buildResponseMeta(statusCode int, headers http.Header) *fusion.ResponseMeta {
	meta := &fusion.ResponseMeta{StatusCode: statusCode}
	if statusCode == 429 {
		if reset := computeRateLimitReset(headers); !reset.IsZero() {
			meta.RateLimitReset = &reset
		}
	}
	return meta
}

// detailsCacheKey returns the cache key for per-card grade detail data.
func detailsCacheKey(card pricing.Card) string {
	return "details:" + card.Set + ":" + card.Name + ":" + card.Number
}

// observabilitySourceName maps adapter source names to short codes used by
// fetchFromAvailableSources for metrics labels, log fields, and collector/telemetry tags.
// Update this map when adding a new secondary source or changing instrumentation.
var observabilitySourceName = map[string]string{}
