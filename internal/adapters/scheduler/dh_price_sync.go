package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHPriceSyncConfig controls the DH price-sync scheduler.
type DHPriceSyncConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHPriceSyncScheduler runs periodic drift reconciliation: it finds
// purchases whose ReviewedPriceCents has diverged from
// DHListingPriceCents and PATCHes DH to catch them up. Safety net for
// inline goroutines that failed or were missed.
type DHPriceSyncScheduler struct {
	StopHandle
	svc    dhpricing.Service
	logger observability.Logger
	config DHPriceSyncConfig
}

// NewDHPriceSyncScheduler wires the scheduler.
func NewDHPriceSyncScheduler(svc dhpricing.Service, logger observability.Logger, config DHPriceSyncConfig) *DHPriceSyncScheduler {
	if config.Interval <= 0 {
		config.Interval = 15 * time.Minute
	}
	return &DHPriceSyncScheduler{
		StopHandle: NewStopHandle(),
		svc:        svc,
		logger:     logger.With(context.Background(), observability.String("component", "dh-price-sync")),
		config:     config,
	}
}

// Start begins the sync loop. Honors config.Enabled.
func (s *DHPriceSyncScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "dh price sync scheduler disabled")
		return
	}
	RunLoop(ctx, LoopConfig{
		Name:     "dh-price-sync",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.tick)
}

func (s *DHPriceSyncScheduler) tick(ctx context.Context) {
	_ = s.svc.SyncDriftedPurchases(ctx)
}
