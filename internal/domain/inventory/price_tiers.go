package inventory

import "math"

// PriceTier is a fixed cost-basis price tier boundary. MaxCents is exclusive.
type PriceTier struct {
	Label    string
	MinCents int
	MaxCents int
}

// FixedPriceTiers are the fixed cost-basis tier boundaries, in cents.
//
// Lives in the hub because two packages read it: ClassifyPriceTier here, and
// internal/domain/tuning's ComputePriceTierPerformance. A second copy in tuning
// would let the two tier taxonomies drift apart silently.
var FixedPriceTiers = []PriceTier{
	{"$0-50", 0, 5000},
	{"$50-100", 5000, 10000},
	{"$100-200", 10000, 20000},
	{"$200-500", 20000, 50000},
	{"$500+", 50000, math.MaxInt},
}
