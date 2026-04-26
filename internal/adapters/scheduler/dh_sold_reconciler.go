package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHSoldReconcilerConfig controls the sold-status reconciler scheduler.
type DHSoldReconcilerConfig struct {
	Enabled  bool
	Interval time.Duration
}

// StaleDHStatusLister finds purchases with a linked sale but stale dh_status.
type StaleDHStatusLister interface {
	ListStaleDHStatusSoldPurchases(ctx context.Context) ([]string, error)
}

// DHStatusUpdater updates the dh_status column on a purchase.
type DHStatusUpdater interface {
	UpdatePurchaseDHStatus(ctx context.Context, id string, status string) error
}

// DHSoldReconcilerScheduler periodically fixes purchases that have a linked
// sale but whose dh_status is not 'sold'. This is a safety net for the
// best-effort dh_status update in CreateSale/CreateBulkSales.
type DHSoldReconcilerScheduler struct {
	StopHandle
	lister  StaleDHStatusLister
	updater DHStatusUpdater
	logger  observability.Logger
	config  DHSoldReconcilerConfig
}

// NewDHSoldReconcilerScheduler creates a new sold-status reconciler scheduler.
func NewDHSoldReconcilerScheduler(
	lister StaleDHStatusLister,
	updater DHStatusUpdater,
	logger observability.Logger,
	config DHSoldReconcilerConfig,
) *DHSoldReconcilerScheduler {
	if config.Interval <= 0 {
		config.Interval = 1 * time.Hour
	}
	return &DHSoldReconcilerScheduler{
		StopHandle: NewStopHandle(),
		lister:     lister,
		updater:    updater,
		logger:     logger.With(context.Background(), observability.String("component", "dh-sold-reconciler")),
		config:     config,
	}
}

// Start begins the reconciler loop.
func (s *DHSoldReconcilerScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "dh sold reconciler scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-sold-reconciler",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.reconcile)
}

func (s *DHSoldReconcilerScheduler) reconcile(ctx context.Context) {
	ids, err := s.lister.ListStaleDHStatusSoldPurchases(ctx)
	if err != nil {
		s.logger.Warn(ctx, "dh sold reconciler: failed to list stale purchases",
			observability.Err(err))
		return
	}
	if len(ids) == 0 {
		return
	}

	fixed := 0
	for _, id := range ids {
		if err := s.updater.UpdatePurchaseDHStatus(ctx, id, string(inventory.DHStatusSold)); err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to update purchase",
				observability.String("purchaseID", id),
				observability.Err(err))
			continue
		}
		fixed++
	}

	s.logger.Info(ctx, "dh sold reconciler completed",
		observability.Int("fixed", fixed),
		observability.Int("total", len(ids)))
}
