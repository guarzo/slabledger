package scheduler

import (
	"context"

	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*SessionCleanupScheduler)(nil)

// SessionCleanupScheduler handles periodic cleanup of expired sessions
type SessionCleanupScheduler struct {
	StopHandle
	authService auth.Service
	logger      observability.Logger
	interval    time.Duration
	enabled     bool
}

// SessionCleanupConfig holds configuration for session cleanup
type SessionCleanupConfig struct {
	Enabled  bool
	Interval time.Duration // How often to run cleanup (default: 1 hour)
}

// DefaultSessionCleanupConfig returns sensible defaults
func DefaultSessionCleanupConfig() SessionCleanupConfig {
	return SessionCleanupConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	}
}

// NewSessionCleanupScheduler creates a new session cleanup scheduler
func NewSessionCleanupScheduler(
	authService auth.Service,
	logger observability.Logger,
	config SessionCleanupConfig,
) *SessionCleanupScheduler {
	interval := config.Interval
	if interval == 0 {
		interval = 1 * time.Hour
	}

	return &SessionCleanupScheduler{
		StopHandle:  NewStopHandle(),
		authService: authService,
		logger:      logger.With(context.Background(), observability.String("component", "session-cleanup")),
		interval:    interval,
		enabled:     config.Enabled,
	}
}

// Start begins the background cleanup scheduler
func (s *SessionCleanupScheduler) Start(ctx context.Context) {
	if !s.enabled {
		s.logger.Info(ctx, "session cleanup scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "session-cleanup",
		Interval: s.interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.cleanup)
}

// cleanup performs the actual session cleanup
func (s *SessionCleanupScheduler) cleanup(ctx context.Context) {
	s.logger.Debug(ctx, "running session cleanup")

	count, err := s.authService.CleanupExpiredSessions(ctx)
	if err != nil {
		s.logger.Error(ctx, "session cleanup failed", observability.Err(err))
		return
	}

	if count > 0 {
		s.logger.Info(ctx, "expired sessions cleaned up",
			observability.Int("count", count))
	} else {
		s.logger.Debug(ctx, "no expired sessions to clean up")
	}
}
