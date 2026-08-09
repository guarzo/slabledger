package inventory

// PriceSourceUnknown/Itemized/Estimated/Manual classify HOW a sale's price was
// arrived at, so suspect rows (guessed from CL value rather than a real
// transaction) can be filtered out of profit analytics rather than silently
// averaged in alongside itemized sales.
const (
	PriceSourceUnknown   = ""
	PriceSourceItemized  = "itemized"
	PriceSourceEstimated = "estimated"
	PriceSourceManual    = "manual"
)

var validPriceSources = map[string]bool{
	PriceSourceItemized:  true,
	PriceSourceEstimated: true,
	PriceSourceManual:    true,
}

// ValidPriceSource allows the three named sources plus "" (unknown/legacy).
func ValidPriceSource(s string) bool { return s == "" || validPriceSources[s] }
