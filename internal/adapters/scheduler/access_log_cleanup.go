package scheduler

import (
	"context"

	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// AccessLogCleanupScheduler handles periodic cleanup of old card access log entries.
// This prevents the card_access_log table from growing unbounded.
//
// RETENTION STRATEGY:
// - Default retention: 30 days (configurable via ACCESS_LOG_RETENTION_DAYS env var)
// - Default interval: 24 hours (configurable via ACCESS_LOG_CLEANUP_INTERVAL env var)
// - Runs DELETE FROM card_access_log WHERE accessed_at < DATETIME('now', '-N days')
//
// The accessed_at index (idx_access_log_recent) ensures efficient DELETE operations.
//
// For large databases, consider running VACUUM manually during maintenance windows
// after significant cleanup to reclaim disk space.
type AccessLogCleanupScheduler struct {
	StopHandle
	accessTracker pricing.AccessTracker
	logger        observability.Logger
	interval      time.Duration
	retentionDays int
	enabled       bool
}

// AccessLogCleanupConfig holds configuration for access log cleanup
type AccessLogCleanupConfig struct {
	Enabled       bool          // Whether cleanup is enabled (default: true)
	Interval      time.Duration // How often to run cleanup (default: 24 hours)
	RetentionDays int           // How many days of logs to keep (default: 30)
}

// DefaultAccessLogCleanupConfig returns sensible defaults
func DefaultAccessLogCleanupConfig() AccessLogCleanupConfig {
	return AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      24 * time.Hour,
		RetentionDays: 30,
	}
}

// NewAccessLogCleanupScheduler creates a new access log cleanup scheduler
func NewAccessLogCleanupScheduler(
	accessTracker pricing.AccessTracker,
	logger observability.Logger,
	config AccessLogCleanupConfig,
) *AccessLogCleanupScheduler {
	interval := config.Interval
	if interval == 0 {
		interval = 24 * time.Hour
	}

	retentionDays := config.RetentionDays
	if retentionDays == 0 {
		retentionDays = 30
	}

	return &AccessLogCleanupScheduler{
		StopHandle:    NewStopHandle(),
		accessTracker: accessTracker,
		logger:        logger.With(context.Background(), observability.String("component", "access-log-cleanup")),
		interval:      interval,
		retentionDays: retentionDays,
		enabled:       config.Enabled,
	}
}

// Start begins the background cleanup scheduler
func (s *AccessLogCleanupScheduler) Start(ctx context.Context) {
	if !s.enabled {
		s.logger.Info(ctx, "access log cleanup scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "access-log-cleanup",
		Interval: s.interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
		LogFields: []observability.Field{
			observability.Int("retention_days", s.retentionDays),
		},
	}, s.cleanup)
}

// cleanup performs the actual cleanup operation
func (s *AccessLogCleanupScheduler) cleanup(ctx context.Context) {
	s.logger.Debug(ctx, "running access log cleanup",
		observability.Int("retention_days", s.retentionDays))

	count, err := s.accessTracker.CleanupOldAccessLogs(ctx, s.retentionDays)
	if err != nil {
		s.logger.Error(ctx, "access log cleanup failed", observability.Err(err))
		return
	}

	if count > 0 {
		s.logger.Info(ctx, "old access log entries cleaned up",
			observability.Int("count", int(count)),
			observability.Int("retention_days", s.retentionDays))
	} else {
		s.logger.Debug(ctx, "no old access log entries to clean up")
	}
}

// RunOnce performs a single cleanup operation (useful for testing or manual triggers)
func (s *AccessLogCleanupScheduler) RunOnce(ctx context.Context) (int64, error) {
	return s.accessTracker.CleanupOldAccessLogs(ctx, s.retentionDays)
}
