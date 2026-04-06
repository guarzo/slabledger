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

// ProviderStats represents statistics from a price provider.
// This allows monitoring of cache hit rates, API usage, and circuit breaker state.
type ProviderStats struct {
	// Request metrics
	APIRequests    int64  // Direct API calls made
	CachedRequests int64  // Requests served from cache
	TotalRequests  int64  // Total requests (API + cached)
	CacheHitRate   string // Cache hit rate percentage (formatted: "XX.XX%")
	Reduction      string // API reduction percentage (formatted: "XX.XX%")

	// Circuit breaker state (optional)
	CircuitBreaker *CircuitBreakerStats
}

// CircuitBreakerStats represents circuit breaker state and metrics.
type CircuitBreakerStats struct {
	// State is the current circuit breaker state: "closed", "open", or "half-open".
	State string
	// Requests is the total number of requests observed.
	Requests uint32
	// Successes is the total number of successful requests.
	Successes uint32
	// Failures is the total number of failed requests.
	Failures uint32
	// ConsecutiveSuccesses is the count of back-to-back successful requests.
	ConsecutiveSuccesses uint32
	// ConsecutiveFailures is the count of back-to-back failed requests.
	ConsecutiveFailures uint32
}
