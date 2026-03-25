package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// SocialContentDetector is the subset of social.Service needed by the scheduler.
type SocialContentDetector interface {
	DetectAndGenerate(ctx context.Context) (int, error)
}

var _ Scheduler = (*SocialContentScheduler)(nil)

// SocialContentScheduler generates social media post drafts on a schedule.
type SocialContentScheduler struct {
	StopHandle
	detector       SocialContentDetector
	tokenRefresher InstagramTokenRefresher
	logger         observability.Logger
	config         config.SocialContentConfig
}

// SocialContentOption configures the social content scheduler.
type SocialContentOption func(*SocialContentScheduler)

// WithTokenRefresher sets an Instagram token refresher.
func WithTokenRefresher(r InstagramTokenRefresher) SocialContentOption {
	return func(s *SocialContentScheduler) { s.tokenRefresher = r }
}

// NewSocialContentScheduler creates a new social content scheduler.
func NewSocialContentScheduler(
	detector SocialContentDetector,
	logger observability.Logger,
	cfg config.SocialContentConfig,
	opts ...SocialContentOption,
) *SocialContentScheduler {
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 5 * time.Minute
	}
	s := &SocialContentScheduler{
		StopHandle: NewStopHandle(),
		detector:   detector,
		logger:     logger.With(context.Background(), observability.String("component", "social-content")),
		config:     cfg,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the background scheduler.
func (s *SocialContentScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "social content scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "social-content",
		Interval:     s.config.Interval,
		InitialDelay: s.config.InitialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.tick)
}

// InstagramTokenRefresher is called by the scheduler to refresh the Instagram token.
type InstagramTokenRefresher interface {
	RefreshIfNeeded(ctx context.Context) error
}

func (s *SocialContentScheduler) tick(ctx context.Context) {
	// Refresh Instagram token if needed
	if s.tokenRefresher != nil {
		if err := s.tokenRefresher.RefreshIfNeeded(ctx); err != nil {
			s.logger.Error(ctx, "instagram token refresh failed", observability.Err(err))
		}
	}

	s.logger.Info(ctx, "running social content detection")

	genCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	created, err := s.detector.DetectAndGenerate(genCtx)
	if err != nil {
		s.logger.Error(ctx, "social content detection failed", observability.Err(err))
		return
	}

	s.logger.Info(ctx, "social content detection completed",
		observability.Int("posts_created", created))
}
