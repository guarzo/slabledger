package inventory

import "time"

// DHPushConfig holds admin-configurable thresholds for DH push safety gates.
type DHPushConfig struct {
	SwingPctThreshold            int       `json:"swingPctThreshold"`
	SwingMinCents                int       `json:"swingMinCents"`
	DisagreementPctThreshold     int       `json:"disagreementPctThreshold"`
	UnreviewedChangePctThreshold int       `json:"unreviewedChangePctThreshold"`
	UnreviewedChangeMinCents     int       `json:"unreviewedChangeMinCents"`
	InitialPushValueFloorPct     int       `json:"initialPushValueFloorPct"`
	// ListingsPaused, when true, pauses the DH list transition globally.
	// Items still flow through psa_import (so inventory creation continues),
	// but ListPurchases skips the in_stock → listed flip and leaves items
	// unlisted on DoubleHolo. Used during card-show liquidation windows
	// where local sales should not be undercut by live DH listings.
	ListingsPaused bool      `json:"listingsPaused"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// DefaultDHPushConfig returns sensible defaults for push safety thresholds.
func DefaultDHPushConfig() DHPushConfig {
	return DHPushConfig{
		SwingPctThreshold:            20,
		SwingMinCents:                5000,
		DisagreementPctThreshold:     25,
		UnreviewedChangePctThreshold: 15,
		UnreviewedChangeMinCents:     3000,
		InitialPushValueFloorPct:     50,
	}
}
