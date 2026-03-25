package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// SnapshotEnrichService is the subset of campaigns.Service needed by the enrichment scheduler.
type SnapshotEnrichService interface {
	ProcessPendingSnapshots(ctx context.Context, limit int) (processed, skipped, failed int)
	RetryFailedSnapshots(ctx context.Context, limit int) (processed, skipped, failed int)
}

var _ Scheduler = (*SnapshotEnrichScheduler)(nil)

// SnapshotEnrichScheduler processes purchases that need background market snapshot capture.
// It runs two loops: a fast loop for pending snapshots and a slower loop for retrying failures.
type SnapshotEnrichScheduler struct {
	StopHandle
	service SnapshotEnrichService
	logger  observability.Logger
	config  config.SnapshotEnrichConfig
}

// NewSnapshotEnrichScheduler creates a new snapshot enrichment scheduler.
func NewSnapshotEnrichScheduler(
	service SnapshotEnrichService,
	logger observability.Logger,
	cfg config.SnapshotEnrichConfig,
) *SnapshotEnrichScheduler {
	cfg.ApplyDefaults()
	return &SnapshotEnrichScheduler{
		StopHandle: NewStopHandle(),
		service:    service,
		logger:     logger.With(context.Background(), observability.String("component", "snapshot-enrich")),
		config:     cfg,
	}
}

// Start begins two background loops: one for pending snapshots and one for retries.
func (s *SnapshotEnrichScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "snapshot enrichment scheduler disabled")
		return
	}
	if s.service == nil {
		s.logger.Error(ctx, "snapshot enrichment scheduler: service is nil, refusing to start")
		return
	}

	// Primary loop: process pending snapshots quickly.
	// WG.Add before goroutine launch so Stop()+Wait() cannot return prematurely.
	s.WG().Add(1)
	go func() {
		defer s.WG().Done()
		RunLoop(ctx, LoopConfig{
			Name:         "snapshot-enrich",
			Interval:     s.config.Interval,
			InitialDelay: 5 * time.Second,
			WG:           nil, // tracked by outer Add/Done
			StopChan:     s.Done(),
			Logger:       s.logger,
			LogFields: []observability.Field{
				observability.Int("batch_size", s.config.BatchSize),
			},
		}, s.tickPending)
	}()

	// Retry loop: retry failed snapshots on a slower cadence.
	s.WG().Add(1)
	go func() {
		defer s.WG().Done()
		RunLoop(ctx, LoopConfig{
			Name:         "snapshot-retry",
			Interval:     s.config.RetryInterval,
			InitialDelay: 2 * time.Minute,
			WG:           nil, // tracked by outer Add/Done
			StopChan:     s.Done(),
			Logger:       s.logger,
		}, s.tickRetry)
	}()
}

// tickPending processes one batch of pending snapshots.
func (s *SnapshotEnrichScheduler) tickPending(ctx context.Context) {
	processed, skipped, failed := s.service.ProcessPendingSnapshots(ctx, s.config.BatchSize)
	if processed > 0 || skipped > 0 || failed > 0 {
		s.logger.Info(ctx, "snapshot enrichment",
			observability.Int("processed", processed),
			observability.Int("skipped", skipped),
			observability.Int("failed", failed))
	}
}

// tickRetry retries one batch of previously-failed snapshots.
func (s *SnapshotEnrichScheduler) tickRetry(ctx context.Context) {
	processed, skipped, failed := s.service.RetryFailedSnapshots(ctx, s.config.BatchSize)
	if processed > 0 || skipped > 0 || failed > 0 {
		s.logger.Info(ctx, "snapshot retry",
			observability.Int("processed", processed),
			observability.Int("skipped", skipped),
			observability.Int("failed", failed))
	}
}
