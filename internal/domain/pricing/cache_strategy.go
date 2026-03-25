package pricing

import "time"

// CachePriorityResult describes the caching behavior for a card based on its characteristics.
type CachePriorityResult struct {
	TTL      time.Duration
	Priority int  // Higher = more important to keep cached
	Volatile bool // True if prices change frequently
}

// CachePriorityStrategy determines cache TTL and priority based on card characteristics.
type CachePriorityStrategy struct {
	HighValueThresholdCents int
	HighValueTTL            time.Duration
	ActiveTradingTTL        time.Duration
	StableTTL               time.Duration
	ActiveSalesThreshold    int // Minimum recent sales to be considered "active"
}

// CalculatePriority determines cache priority based on card value and trading activity.
func (s *CachePriorityStrategy) CalculatePriority(psa10Cents, bgs10Cents int, recentSalesCount int) CachePriorityResult {
	highValue := psa10Cents > s.HighValueThresholdCents || bgs10Cents > s.HighValueThresholdCents
	hasRecentSales := recentSalesCount > s.ActiveSalesThreshold

	switch {
	case highValue:
		return CachePriorityResult{
			TTL:      s.HighValueTTL,
			Priority: 3,
			Volatile: true,
		}
	case hasRecentSales:
		return CachePriorityResult{
			TTL:      s.ActiveTradingTTL,
			Priority: 2,
			Volatile: false,
		}
	default:
		return CachePriorityResult{
			TTL:      s.StableTTL,
			Priority: 1,
			Volatile: false,
		}
	}
}
