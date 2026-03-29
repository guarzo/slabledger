package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// PicksGenerator is the subset of picks.Service needed by the scheduler.
type PicksGenerator interface {
	GenerateDailyPicks(ctx context.Context) error
}

var _ Scheduler = (*PicksRefreshScheduler)(nil)

type PicksRefreshScheduler struct {
	StopHandle
	generator PicksGenerator
	logger    observability.Logger
	config    config.PicksRefreshConfig
}

func NewPicksRefreshScheduler(
	generator PicksGenerator,
	logger observability.Logger,
	cfg config.PicksRefreshConfig,
) *PicksRefreshScheduler {
	cfg.ApplyDefaults()
	return &PicksRefreshScheduler{
		StopHandle: NewStopHandle(),
		generator:  generator,
		logger:     logger.With(context.Background(), observability.String("component", "picks-refresh")),
		config:     cfg,
	}
}

func (s *PicksRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "picks refresh scheduler disabled")
		return
	}

	initialDelay := 5 * time.Minute
	if s.config.ContentHour >= 0 && s.config.ContentHour <= 23 {
		initialDelay = timeUntilHour(time.Now(), s.config.ContentHour)
	} else if s.config.ContentHour > 23 {
		s.logger.Warn(ctx, "invalid ContentHour, using default 5m delay",
			observability.Int("contentHour", s.config.ContentHour))
	}

	RunLoop(ctx, LoopConfig{
		Name:         "picks-refresh",
		Interval:     s.config.Interval,
		InitialDelay: initialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.tick)
}

func (s *PicksRefreshScheduler) tick(ctx context.Context) {
	s.logger.Info(ctx, "starting daily picks generation")
	start := time.Now()

	if err := s.generator.GenerateDailyPicks(ctx); err != nil {
		s.logger.Error(ctx, "picks generation failed",
			observability.Err(err),
			observability.Int("duration_s", int(time.Since(start).Seconds())),
		)
		return
	}

	s.logger.Info(ctx, "picks generation completed",
		observability.Int("duration_s", int(time.Since(start).Seconds())),
	)
}
