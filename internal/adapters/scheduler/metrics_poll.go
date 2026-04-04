package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/platform/config"
)

var _ Scheduler = (*MetricsPollScheduler)(nil)

// MetricsPollScheduler polls Instagram for engagement metrics on recently published posts.
type MetricsPollScheduler struct {
	StopHandle
	lister social.MetricsPostLister
	saver  social.MetricsSaver
	poller social.InsightsPoller
	logger observability.Logger
	config config.MetricsPollConfig
}

// NewMetricsPollScheduler creates a new metrics poll scheduler.
func NewMetricsPollScheduler(
	lister social.MetricsPostLister,
	saver social.MetricsSaver,
	poller social.InsightsPoller,
	logger observability.Logger,
	cfg config.MetricsPollConfig,
) *MetricsPollScheduler {
	if cfg.Interval <= 0 {
		cfg.Interval = 6 * time.Hour
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 7 * 24 * time.Hour
	}
	return &MetricsPollScheduler{
		StopHandle: NewStopHandle(),
		lister:     lister,
		saver:      saver,
		poller:     poller,
		logger:     logger.With(context.Background(), observability.String("component", "metrics-poll")),
		config:     cfg,
	}
}

// Start begins the background scheduler.
func (s *MetricsPollScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "metrics poll scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "metrics-poll",
		Interval:     s.config.Interval,
		InitialDelay: 2 * time.Minute,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.Tick)
}

// Tick fetches and saves metrics for all recently published posts.
// Exported for testing.
func (s *MetricsPollScheduler) Tick(ctx context.Context) {
	since := time.Now().Add(-s.config.MaxAge)
	posts, err := s.lister.GetPublishedPostIDs(ctx, since)
	if err != nil {
		s.logger.Error(ctx, "metrics poll: failed to list published posts", observability.Err(err))
		return
	}

	now := time.Now()
	for _, post := range posts {
		metrics, err := s.poller.PollInsights(ctx, post.InstagramPostID)
		if err != nil {
			s.logger.Error(ctx, "metrics poll: failed to poll insights",
				observability.String("post_id", post.PostID),
				observability.String("instagram_post_id", post.InstagramPostID),
				observability.Err(err))
			continue
		}
		if metrics == nil {
			s.logger.Warn(ctx, "metrics poll: poller returned nil metrics, skipping",
				observability.String("post_id", post.PostID))
			continue
		}

		metrics.PostID = post.PostID
		metrics.PolledAt = now

		if err := s.saver.SaveMetrics(ctx, metrics); err != nil {
			s.logger.Error(ctx, "metrics poll: failed to save metrics",
				observability.String("post_id", post.PostID),
				observability.Err(err))
			continue
		}
	}

	s.logger.Info(ctx, "metrics poll completed", observability.Int("posts_polled", len(posts)))
}
