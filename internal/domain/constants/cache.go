package constants

import "time"

const (
	// Dynamic TTL for high-value cards
	HighValueThresholdCents = 10000 // $100
	HighValueCacheTTL       = 4 * time.Hour

	ActiveTradingCacheTTL = 8 * time.Hour
	StableCacheTTL        = 20 * time.Hour

	// ActiveSalesThreshold is the minimum number of recent sales for a card
	// to be considered actively traded (shorter cache TTL).
	ActiveSalesThreshold = 5
)
