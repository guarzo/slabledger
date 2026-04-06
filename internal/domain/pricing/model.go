package pricing

// SalesDistribution represents the distribution of recent sales prices
// Used for conservative exit price calculations (Profit Optimization)
type SalesDistribution struct {
	P10  float64 // 10th percentile (pessimistic - 90% of sales above this)
	P25  float64 // 25th percentile (conservative - 75% of sales above this)
	P50  float64 // 50th percentile (median/typical)
	P75  float64 // 75th percentile (optimistic - 25% of sales above this)
	P90  float64 // 90th percentile (very optimistic - 10% of sales above this)
	Mean float64 // Average sale price

	SampleSize int    // Number of recent sales
	Period     int    // Days of data (30, 60, 90)
	Source     string // "doubleholo", "csv_history"
}
