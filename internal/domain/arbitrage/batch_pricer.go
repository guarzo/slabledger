package arbitrage

import "context"

// BatchPricer provides batch price distribution lookups for arbitrage analysis.
// When injected via WithBatchPricer, GetCrackOpportunities and
// GetAcquisitionTargets use batch DH API calls (2-3 HTTP requests total)
// instead of per-card lookups (~400+ requests).
type BatchPricer interface {
	// ResolveDHCardID maps a card identity to its DH card ID.
	// Returns 0 if the card has no DH mapping.
	ResolveDHCardID(ctx context.Context, cardName, setName, cardNumber string) (int, error)

	// BatchPriceDistribution returns per-grade price distribution for the given
	// DH card IDs. The returned map is keyed by card ID. Cards with no data or
	// errors are omitted from the map (not treated as errors).
	BatchPriceDistribution(ctx context.Context, cardIDs []int) (map[int]GradedDistribution, error)
}

// GradedDistribution holds per-grade price statistics from a batch analytics call.
type GradedDistribution struct {
	// ByGrade maps DH grade keys (e.g. "PSA 10", "PSA 9", "Raw") to price stats.
	ByGrade map[string]PriceBucket
}

// PriceBucket holds min/median/max/avg price stats for a single grade.
type PriceBucket struct {
	MinCents    int
	MedianCents int
	MaxCents    int
	AvgCents    int
	SampleSize  int
}

// gradeKeyForValue maps a numeric grade to the DH price_distribution key.
// Grade 0 means raw/ungraded.
func gradeKeyForValue(grade float64) string {
	switch {
	case grade >= 9.5:
		return "PSA 10"
	case grade >= 8.5:
		return "PSA 9"
	case grade >= 7.5:
		return "PSA 8"
	case grade >= 6.5:
		return "PSA 7"
	case grade >= 5.5:
		return "PSA 6"
	case grade == 0:
		return "Raw"
	default:
		return "Raw"
	}
}
