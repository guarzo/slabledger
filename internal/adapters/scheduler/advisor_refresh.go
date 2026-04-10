package scheduler

import (
	"context"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// AdvisorCollector is the subset of advisor.Service needed by the scheduler.
type AdvisorCollector interface {
	CollectDigest(ctx context.Context) (string, error)
	CollectLiquidation(ctx context.Context) (string, error)
}

var _ Scheduler = (*AdvisorRefreshScheduler)(nil)

// AdvisorRefreshScheduler runs AI advisor analyses on a schedule and caches the results.
type AdvisorRefreshScheduler struct {
	StopHandle
	collector AdvisorCollector
	cache     advisor.CacheStore
	tracker   ai.AICallTracker
	logger    observability.Logger
	config    config.AdvisorRefreshConfig
}

// NewAdvisorRefreshScheduler creates a new advisor refresh scheduler.
func NewAdvisorRefreshScheduler(
	collector AdvisorCollector,
	cache advisor.CacheStore,
	tracker ai.AICallTracker,
	logger observability.Logger,
	cfg config.AdvisorRefreshConfig,
) *AdvisorRefreshScheduler {
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.InitialDelay < 0 {
		cfg.InitialDelay = 0
	}
	// Compute initial delay from RefreshHour only if >= 0.
	// Note: RefreshHour < 0 means scheduler is disabled (handled in Start()).
	if cfg.RefreshHour >= 0 {
		cfg.InitialDelay = timeUntilHour(time.Now(), cfg.RefreshHour)
	}
	return &AdvisorRefreshScheduler{
		StopHandle: NewStopHandle(),
		collector:  collector,
		cache:      cache,
		tracker:    tracker,
		logger:     logger.With(context.Background(), observability.String("component", "advisor-refresh")),
		config:     cfg,
	}
}

// Start begins the background scheduler.
func (s *AdvisorRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "advisor refresh scheduler disabled")
		return
	}

	// Check for disabled scheduler: RefreshHour < 0 means do not run
	if s.config.RefreshHour < 0 {
		s.logger.Info(ctx, "advisor refresh scheduler disabled (RefreshHour < 0)")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "advisor-refresh",
		Interval:     s.config.Interval,
		InitialDelay: s.config.InitialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.tick)
}

func (s *AdvisorRefreshScheduler) tick(ctx context.Context) {
	s.runWithRetry(ctx, advisor.AnalysisDigest, s.collector.CollectDigest)

	// Brief pause between analyses to avoid back-to-back rate limiting.
	select {
	case <-ctx.Done():
		return
	case <-time.After(interAnalysisPause):
	}

	s.runWithRetry(ctx, advisor.AnalysisLiquidation, s.collector.CollectLiquidation)

	s.checkAIHealth(ctx)
}

const (
	schedulerTransientRetries = 2
	schedulerRetryBackoff     = 5 * time.Minute
	interAnalysisPause        = 5 * time.Minute
	// AnalysisCollectTimeout is the max duration for an LLM analysis call.
	// Exported so tests can assert against the same value.
	AnalysisCollectTimeout = 20 * time.Minute
	// AnalysisSaveTimeout is the max duration for persisting analysis results.
	AnalysisSaveTimeout = 15 * time.Second
)

func (s *AdvisorRefreshScheduler) runWithRetry(ctx context.Context, analysisType advisor.AnalysisType, collect func(context.Context) (string, error)) {
	for attempt := range schedulerTransientRetries + 1 {
		err := s.runAnalysis(ctx, analysisType, collect)
		if err == nil {
			return
		}
		if !isTransientAIError(err) || attempt == schedulerTransientRetries {
			return
		}
		s.logger.Warn(ctx, "transient error, scheduler retry",
			observability.String("type", string(analysisType)),
			observability.Int("attempt", attempt+1),
			observability.Duration("delay", schedulerRetryBackoff),
			observability.Err(err),
		)
		select {
		case <-ctx.Done():
			return
		case <-time.After(schedulerRetryBackoff):
		}
	}
}

// isTransientAIError returns true for errors worth retrying at the scheduler level
// (rate limits and network-level failures like connection resets).
func isTransientAIError(err error) bool {
	if err == nil {
		return false
	}
	status, _ := ai.ClassifyAIError(err)
	if status == ai.AIStatusRateLimited {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "SSE stream ended without") ||
		strings.Contains(msg, "status incomplete")
}

func (s *AdvisorRefreshScheduler) runAnalysis(ctx context.Context, analysisType advisor.AnalysisType, collect func(context.Context) (string, error)) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.logger.Info(ctx, "starting advisor analysis", observability.String("type", string(analysisType)))

	lease, err := s.cache.MarkRunning(ctx, analysisType)
	if err != nil {
		s.logger.Error(ctx, "failed to mark analysis running", observability.String("type", string(analysisType)), observability.Err(err))
		return err
	}

	start := time.Now()
	// Use a generous timeout for the LLM call.
	analysisCtx, cancel := context.WithTimeout(ctx, AnalysisCollectTimeout)
	defer cancel()

	content, collectErr := collect(analysisCtx)

	duration := time.Since(start)
	errMsg := ""
	if collectErr != nil {
		errMsg = collectErr.Error()
		s.logger.Error(ctx, "advisor analysis failed",
			observability.String("type", string(analysisType)),
			observability.Err(collectErr),
			observability.Int("duration_s", int(duration.Seconds())),
		)
	} else {
		s.logger.Info(ctx, "advisor analysis completed",
			observability.String("type", string(analysisType)),
			observability.Int("duration_s", int(duration.Seconds())),
			observability.Int("content_len", len(content)),
		)
	}

	// Use a background context for the save — the parent ctx may be canceled during shutdown
	// but we still want to persist the result so the row doesn't stay "running" forever.
	saveCtx, saveCancel := context.WithTimeout(context.Background(), AnalysisSaveTimeout)
	defer saveCancel()
	if saveErr := s.cache.SaveResult(saveCtx, analysisType, lease, content, errMsg); saveErr != nil {
		s.logger.Error(saveCtx, "failed to save analysis result", observability.String("type", string(analysisType)), observability.Err(saveErr))
	}
	return collectErr
}

// timeUntilHour returns the duration from now until the next occurrence
// of the given hour (0-23) in UTC.
func timeUntilHour(now time.Time, hour int) time.Duration {
	nowUTC := now.UTC()
	target := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), hour, 0, 0, 0, time.UTC)
	if !target.After(nowUTC) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(nowUTC)
}

// checkAIHealth logs warnings if AI metrics indicate degradation.
func (s *AdvisorRefreshScheduler) checkAIHealth(ctx context.Context) {
	if s.tracker == nil {
		return
	}
	stats, err := s.tracker.GetAIUsage(ctx)
	if err != nil {
		s.logger.Warn(ctx, "failed to fetch AI usage for health check", observability.Err(err))
		return
	}
	if stats.TotalCalls == 0 {
		return
	}

	successRate := 100.0 * float64(stats.SuccessCalls) / float64(stats.TotalCalls)
	if successRate < 80 {
		s.logger.Warn(ctx, "AI success rate below 80%",
			observability.Int("totalCalls", int(stats.TotalCalls)),
			observability.Int("successRate", int(successRate)),
			observability.Int("errors", int(stats.ErrorCalls)),
		)
	}

	if stats.RateLimitHits > 5 {
		s.logger.Warn(ctx, "elevated AI rate limiting",
			observability.Int("rateLimitHits7d", int(stats.RateLimitHits)),
			observability.Int("callsLast24h", int(stats.CallsLast24h)),
		)
	}
}
