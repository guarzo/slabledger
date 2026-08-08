package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*DHEventCleanupScheduler)(nil)

// DHEventCleanupScheduler prunes old rows from dh_state_events.
//
// The table is append-only: every push, poll, reconcile, and manual match
// writes a row, and nothing else in the system ever deletes one. Without this
// job it grows for the life of the deployment.
//
// RETENTION STRATEGY:
//   - Default retention: 90 days (DH_EVENT_RETENTION_DAYS)
//   - Default interval: 24 hours (DH_EVENT_CLEANUP_INTERVAL)
//   - Runs DELETE FROM dh_state_events WHERE event_at < now - N days
//
// 90 days is deliberately longer than card_access_log's 30. This is a
// diagnostic trail for a pipeline whose failures surface on the scale of weeks
// — a purchase stuck in pending is noticed at the next inventory review, not
// within the hour — so the history has to outlive the gap between the failure
// and someone looking for it.
type DHEventCleanupScheduler struct {
	StopHandle
	pruner        dhevents.Pruner
	logger        observability.Logger
	interval      time.Duration
	retentionDays int
	enabled       bool
}

// DHEventCleanupConfig holds configuration for dh_state_events cleanup.
type DHEventCleanupConfig struct {
	Enabled       bool          // Whether cleanup is enabled (default: true)
	Interval      time.Duration // How often to run cleanup (default: 24 hours)
	RetentionDays int           // How many days of events to keep (default: 90)
}

// DefaultDHEventCleanupConfig returns sensible defaults.
func DefaultDHEventCleanupConfig() DHEventCleanupConfig {
	return DHEventCleanupConfig{
		Enabled:       true,
		Interval:      24 * time.Hour,
		RetentionDays: 90,
	}
}

// NewDHEventCleanupScheduler creates a new dh_state_events cleanup scheduler.
func NewDHEventCleanupScheduler(
	pruner dhevents.Pruner,
	logger observability.Logger,
	config DHEventCleanupConfig,
) *DHEventCleanupScheduler {
	interval := config.Interval
	if interval == 0 {
		interval = 24 * time.Hour
	}

	retentionDays := config.RetentionDays
	if retentionDays == 0 {
		retentionDays = 90
	}

	return &DHEventCleanupScheduler{
		StopHandle:    NewStopHandle(),
		pruner:        pruner,
		logger:        logger.With(context.Background(), observability.String("component", "dh-event-cleanup")),
		interval:      interval,
		retentionDays: retentionDays,
		enabled:       config.Enabled,
	}
}

// Start begins the background cleanup scheduler.
func (s *DHEventCleanupScheduler) Start(ctx context.Context) {
	if !s.enabled {
		s.logger.Info(ctx, "dh event cleanup scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-event-cleanup",
		Interval: s.interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
		LogFields: []observability.Field{
			observability.Int("retention_days", s.retentionDays),
		},
	}, s.cleanup)
}

// cleanup performs the actual prune.
func (s *DHEventCleanupScheduler) cleanup(ctx context.Context) {
	count, err := s.RunOnce(ctx)
	if err != nil {
		s.logger.Error(ctx, "dh event cleanup failed", observability.Err(err))
		return
	}

	if count > 0 {
		s.logger.Info(ctx, "old dh state events pruned",
			observability.Int("count", int(count)),
			observability.Int("retention_days", s.retentionDays))
	} else {
		s.logger.Debug(ctx, "no dh state events to prune")
	}
}

// RunOnce performs a single prune and returns the number of rows deleted.
// Useful for testing or a manual trigger.
func (s *DHEventCleanupScheduler) RunOnce(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -s.retentionDays)
	return s.pruner.DeleteOlderThan(ctx, cutoff)
}
