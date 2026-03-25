package scheduler

import (
	"time"
)

// Config holds configuration for the price refresh scheduler
type Config struct {
	// How often to check for stale prices
	RefreshInterval time.Duration

	// Max prices to refresh per batch
	BatchSize int

	// Delay between individual API calls
	BatchDelay time.Duration

	// Max API calls per 5-minute window per provider
	MaxBurstCalls int

	// Max API calls allowed per hour per provider (default: 50)
	MaxCallsPerHour int

	// Duration to pause after hitting burst limit (default: 30 seconds)
	// Previously hardcoded to 5 minutes, which could delay shutdown
	BurstPauseDuration time.Duration

	// Enable scheduler
	Enabled bool
}
