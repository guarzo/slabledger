package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// DHReconcileScheduler wraps the dhlisting.Reconciler and runs it on a
// daily cadence. The reconciler diffs local DH linkage against a fresh DH
// inventory snapshot and resets any local purchases whose dh_inventory_id
// is no longer on DH. The push scheduler then re-enrolls the reset rows
// on its next tick (default every 5 minutes).
type DHReconcileScheduler struct {
	StopHandle
	statsMu    sync.RWMutex
	reconciler dhlisting.Reconciler
	logger     observability.Logger
	config     config.DHReconcileConfig
	lastResult *dhlisting.ReconcileResult
}

// NewDHReconcileScheduler constructs the scheduler. The reconciler must be
// non-nil; callers that can't construct one should omit the scheduler.
func NewDHReconcileScheduler(
	reconciler dhlisting.Reconciler,
	logger observability.Logger,
	cfg config.DHReconcileConfig,
) *DHReconcileScheduler {
	cfg.ApplyDefaults()
	return &DHReconcileScheduler{
		StopHandle: NewStopHandle(),
		reconciler: reconciler,
		logger:     logger,
		config:     cfg,
	}
}

// Start begins the scheduler loop.
func (s *DHReconcileScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "DH reconcile scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "dh-reconcile",
		Interval:     s.config.Interval,
		InitialDelay: timeUntilHour(time.Now(), s.config.RefreshHour),
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
		LogFields:    []observability.Field{observability.Int("refreshHour", s.config.RefreshHour)},
	}, func(ctx context.Context) {
		// Error is logged inside RunOnce; the loop callback is fire-and-forget.
		_ = s.RunOnce(ctx)
	})
}

// RunOnce executes a single reconciliation cycle. Exported for manual trigger
// (e.g. from an admin HTTP handler).
func (s *DHReconcileScheduler) RunOnce(ctx context.Context) error {
	result, err := s.reconciler.Reconcile(ctx)

	s.statsMu.Lock()
	if err != nil {
		s.lastResult = nil
	} else {
		s.lastResult = &result
	}
	s.statsMu.Unlock()

	if err != nil {
		s.logger.Warn(ctx, "DH reconcile run failed", observability.Err(err))
		return err
	}
	s.logger.Info(ctx, "DH reconcile run completed",
		observability.Int("scanned", result.Scanned),
		observability.Int("missingOnDH", result.MissingOnDH),
		observability.Int("reset", result.Reset),
		observability.Int("errors", len(result.Errors)))
	return nil
}

// GetLastRunResult returns a copy of the most recent reconcile result, or
// nil if no successful run has completed yet.
func (s *DHReconcileScheduler) GetLastRunResult() *dhlisting.ReconcileResult {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	if s.lastResult == nil {
		return nil
	}
	cp := *s.lastResult
	return &cp
}
