// Package constants provides domain constants for API timeouts and cache configuration.
//
// # Cache Configuration
//
// Cache TTL values and thresholds for tiered caching based on card value
// and trading activity:
//
//   - HighValueCacheTTL: Shorter TTL for high-value cards (fresher data)
//   - ActiveTradingCacheTTL: Moderate TTL for actively traded cards
//   - StableCacheTTL: Longer TTL for stable/low-activity cards
package constants
