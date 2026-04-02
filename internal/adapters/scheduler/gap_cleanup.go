package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/scoring"
)

// GapCleanupScheduler periodically prunes old scoring data gap records
// to prevent the scoring_data_gaps table from growing unbounded.
type GapCleanupScheduler struct {
	StopHandle
	store         scoring.GapStore
	logger        observability.Logger
	interval      time.Duration
	retentionDays int
}

// NewGapCleanupScheduler creates a new gap cleanup scheduler.
// Default retention is 30 days, running every 24 hours.
func NewGapCleanupScheduler(store scoring.GapStore, logger observability.Logger) *GapCleanupScheduler {
	return &GapCleanupScheduler{
		StopHandle:    NewStopHandle(),
		store:         store,
		logger:        logger.With(context.Background(), observability.String("component", "gap-cleanup")),
		interval:      24 * time.Hour,
		retentionDays: 30,
	}
}

func (s *GapCleanupScheduler) Start(ctx context.Context) {
	RunLoop(ctx, LoopConfig{
		Name:     "gap-cleanup",
		Interval: s.interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.cleanup)
}

func (s *GapCleanupScheduler) cleanup(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays)
	count, err := s.store.PruneOldGaps(ctx, cutoff)
	if err != nil {
		s.logger.Error(ctx, "gap cleanup failed", observability.Err(err))
		return
	}
	if count > 0 {
		s.logger.Info(ctx, "old scoring data gaps cleaned up",
			observability.Int("count", int(count)),
			observability.Int("retention_days", s.retentionDays))
	}
}
