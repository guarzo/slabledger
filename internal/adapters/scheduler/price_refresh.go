package scheduler

import (
	"context"
	"fmt"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PriceRefreshScheduler handles background price refresh with rate limiting.
// TODO(Task 4): Rewrite as purchase-driven scheduler — stale_prices VIEW and
// price_history table were dropped in migration 000038.
type PriceRefreshScheduler struct {
	StopHandle
	apiTracker          pricing.APITracker
	healthChecker       pricing.HealthChecker
	priceProvider       pricing.PriceProvider
	logger              observability.Logger
	config              Config
	consecutiveFailures int
}

// NewPriceRefreshScheduler creates a new price refresh scheduler
func NewPriceRefreshScheduler(
	apiTracker pricing.APITracker,
	healthChecker pricing.HealthChecker,
	priceProvider pricing.PriceProvider,
	logger observability.Logger,
	config Config,
) *PriceRefreshScheduler {
	return &PriceRefreshScheduler{
		StopHandle:    NewStopHandle(),
		apiTracker:    apiTracker,
		healthChecker: healthChecker,
		priceProvider: priceProvider,
		logger:        logger.With(context.Background(), observability.String("component", "scheduler")),
		config:        config,
	}
}

// Start begins the background refresh scheduler.
// Callers can use Wait() to block until Start returns after Stop is called.
func (s *PriceRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "price refresh scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "price-refresh",
		Interval: s.config.RefreshInterval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
		LogFields: []observability.Field{
			observability.Int("batch_size", s.config.BatchSize),
		},
	}, s.refreshBatch)
}

// refreshBatch is a placeholder — Task 4 will rewrite this as a purchase-driven
// refresh that iterates campaign_purchases instead of the removed stale_prices VIEW.
func (s *PriceRefreshScheduler) refreshBatch(ctx context.Context) {
	s.logger.Debug(ctx, "price refresh batch: no-op pending Task 4 rewrite")
}

// logAPIUsageSummary logs daily API usage for each provider after a refresh cycle.
func (s *PriceRefreshScheduler) logAPIUsageSummary(ctx context.Context) {
	if s.apiTracker == nil {
		return
	}

	providers := []string{pricing.SourceDH}
	for _, provider := range providers {
		usage, err := s.apiTracker.GetAPIUsage(ctx, provider)
		if err != nil {
			continue
		}
		if usage.TotalCalls == 0 {
			continue
		}

		successRate := float64(usage.TotalCalls-usage.ErrorCalls) / float64(usage.TotalCalls) * 100.0

		s.logger.Info(ctx, "API daily usage",
			observability.String("provider", provider),
			observability.Int("calls", int(usage.TotalCalls)),
			observability.Float64("success_rate_pct", successRate),
			observability.Float64("avg_latency_ms", usage.AvgLatencyMS),
		)
	}
}

// Health returns the health status of the scheduler
func (s *PriceRefreshScheduler) Health(ctx context.Context) error {
	// Disabled scheduler is intentionally inactive, not unhealthy
	if !s.config.Enabled {
		s.logger.Debug(ctx, "scheduler disabled, skipping health checks")
		return nil
	}

	// Check if repository is accessible
	if err := s.healthChecker.Ping(ctx); err != nil {
		return apperrors.StorageError("repository health check", err)
	}

	// Check if provider is available
	if !s.priceProvider.Available() {
		return apperrors.ProviderUnavailable("price_provider", fmt.Errorf("provider not configured or not available"))
	}

	return nil
}
