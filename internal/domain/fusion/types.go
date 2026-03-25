package fusion

import (
	"time"
)

// DataSource represents metadata about a price data source
type DataSource struct {
	Name       string        // Source name (e.g., "TCGPlayer", "eBay", "PriceCharting")
	Freshness  time.Duration // How old the data is
	Volume     int           // Number of data points (listings, sales, etc.)
	Confidence float64       // Source-specific confidence (0.0 to 1.0)
}

// PriceData represents a single price data point from a source
type PriceData struct {
	Value    float64    // Price value
	Currency string     // Currency code (USD, EUR, etc.)
	Source   DataSource // Source metadata
}

// FusedPrice represents the result of fusing multiple price data points
type FusedPrice struct {
	Value         float64      // Fused price value
	Confidence    float64      // Overall confidence score (0.0 to 1.0)
	SourceCount   int          // Number of sources used
	Sources       []SourceData // Individual source contributions
	OutliersFound int          // Number of outliers detected and removed
	Method        string       // Fusion method used (e.g., "weighted_median")
	Currency      string       // Currency of the fused price
}

// SourceData represents a source's contribution to a fused price
type SourceData struct {
	Source string  // Source name
	Price  float64 // Price from this source
	Weight float64 // Weight assigned to this source
}
