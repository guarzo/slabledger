package scheduler

import (
	"context"

	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*SessionCleanupScheduler)(nil)

// SessionJanitor is the slice of auth.Service this scheduler actually uses:
// the two expiry sweeps. Declared here, on the consumer side, so the scheduler
// does not depend on the twenty-method auth.Service composite (SLA-95).
type SessionJanitor interface {
	CleanupExpiredSessions(ctx context.Context) (int, error)
	CleanupExpiredOAuthStates(ctx context.Context) (int, error)
}

var _ SessionJanitor = auth.Service(nil)

// SessionCleanupScheduler handles periodic cleanup of expired sessions and
// expired OAuth state tokens
type SessionCleanupScheduler struct {
	StopHandle
	authService SessionJanitor
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
	authService SessionJanitor,
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

// cleanup sweeps expired sessions and expired OAuth state tokens
func (s *SessionCleanupScheduler) cleanup(ctx context.Context) {
	s.logger.Debug(ctx, "running session cleanup")

	count, err := s.authService.CleanupExpiredSessions(ctx)
	if err != nil {
		s.logger.Error(ctx, "session cleanup failed", observability.Err(err))
	} else if count > 0 {
		s.logger.Info(ctx, "expired sessions cleaned up",
			observability.Int("count", count))
	} else {
		s.logger.Debug(ctx, "no expired sessions to clean up")
	}

	// OAuth state rows are only deleted when a login completes, so abandoned
	// logins leave them behind permanently unless swept here. Run this even if
	// session cleanup failed — the two tables are independent.
	stateCount, err := s.authService.CleanupExpiredOAuthStates(ctx)
	if err != nil {
		s.logger.Error(ctx, "oauth state cleanup failed", observability.Err(err))
		return
	}

	if stateCount > 0 {
		s.logger.Info(ctx, "expired oauth states cleaned up",
			observability.Int("count", stateCount))
	} else {
		s.logger.Debug(ctx, "no expired oauth states to clean up")
	}
}
