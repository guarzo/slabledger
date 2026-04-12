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
	UpdatedAt                    time.Time `json:"updatedAt"`
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
