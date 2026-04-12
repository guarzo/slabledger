package scheduler

import (
	"context"
	"fmt"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// PriceRefreshScheduler refreshes prices for unsold inventory cards.
type PriceRefreshScheduler struct {
	StopHandle
	candidates          pricing.RefreshCandidateProvider
	apiTracker          pricing.APITracker
	healthChecker       pricing.HealthChecker
	priceProvider       pricing.PriceProvider
	logger              observability.Logger
	config              Config
	consecutiveFailures int
	lastFailureAt       time.Time // zero if no failure since startup
}

// NewPriceRefreshScheduler creates a new price refresh scheduler.
func NewPriceRefreshScheduler(
	candidates pricing.RefreshCandidateProvider,
	apiTracker pricing.APITracker,
	healthChecker pricing.HealthChecker,
	priceProvider pricing.PriceProvider,
	logger observability.Logger,
	config Config,
) *PriceRefreshScheduler {
	return &PriceRefreshScheduler{
		StopHandle:    NewStopHandle(),
		candidates:    candidates,
		apiTracker:    apiTracker,
		healthChecker: healthChecker,
		priceProvider: priceProvider,
		logger:        logger.With(context.Background(), observability.String("component", "scheduler")),
		config:        config,
	}
}

// Start begins the background refresh scheduler.
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

// refreshBatch processes one batch of inventory cards needing price refresh.
func (s *PriceRefreshScheduler) refreshBatch(ctx context.Context) {
	// Guard: skip entire batch if provider is unavailable (e.g. missing DH credentials).
	if !s.priceProvider.Available() {
		s.logger.Warn(ctx, "price provider unavailable, skipping refresh batch")
		return
	}

	start := time.Now()

	cards, err := s.candidates.GetRefreshCandidates(ctx, s.config.BatchSize)
	if err != nil {
		s.consecutiveFailures++
		s.lastFailureAt = time.Now()
		s.logger.Error(ctx, "failed to get refresh candidates",
			observability.Err(err),
			observability.Int("consecutive_failures", s.consecutiveFailures))
		return
	}
	s.consecutiveFailures = 0

	if len(cards) == 0 {
		s.logger.Debug(ctx, "no cards to refresh")
		return
	}

	s.logger.Info(ctx, "refreshing prices",
		observability.Int("count", len(cards)))

	// Check if DH provider is blocked
	provider := pricing.SourceDH
	blocked, until, err := s.apiTracker.IsProviderBlocked(ctx, provider)
	if err != nil {
		s.logger.Warn(ctx, "failed to check provider block status",
			observability.String("provider", provider),
			observability.Err(err))
		return
	}
	if blocked {
		s.logger.Warn(ctx, "provider blocked, skipping",
			observability.String("provider", provider),
			observability.Time("blocked_until", until))
		return
	}

	// Check hourly rate limit and compute remaining budget.
	usage, err := s.apiTracker.GetAPIUsage(ctx, provider)
	if err != nil {
		s.logger.Warn(ctx, "failed to get API usage, skipping to avoid rate-limit risk",
			observability.String("provider", provider),
			observability.Err(err))
		return
	}
	if s.config.MaxCallsPerHour > 0 && usage.CallsLastHour >= int64(s.config.MaxCallsPerHour) {
		s.logger.Warn(ctx, "hourly rate limit reached, skipping",
			observability.String("provider", provider),
			observability.Int("calls_last_hour", int(usage.CallsLastHour)),
			observability.Int("max_calls_per_hour", s.config.MaxCallsPerHour))
		return
	}

	remainingBudget := -1 // unlimited when MaxCallsPerHour is 0
	if s.config.MaxCallsPerHour > 0 {
		remainingBudget = s.config.MaxCallsPerHour - int(usage.CallsLastHour)
	}

	successCount := 0
	noDataCount := 0
	errorCount := 0
	skippedCount := 0
	apiCalls := 0

	seen := make(map[string]bool)
	for _, card := range cards {
		select {
		case <-ctx.Done():
			s.logger.Info(ctx, "refresh batch cancelled")
			return
		default:
		}

		cleanName := cardutil.NormalizePurchaseName(card.CardName)
		cardNumber := card.CardNumber
		if cardutil.IsInvalidCardNumber(cardNumber) {
			cardNumber = ""
		}
		if cardNumber == "" {
			if extracted := cardutil.ExtractCollectorNumber(card.CardName); extracted != "" {
				cardNumber = extracted
			}
		}

		if cleanName == "" {
			s.logger.Warn(ctx, "skipping refresh: normalized card name is empty",
				observability.String("original_name", card.CardName))
			skippedCount++
			continue
		}

		dedupeKey := cleanName + "|" + card.SetName + "|" + cardNumber
		if seen[dedupeKey] {
			skippedCount++
			continue
		}
		seen[dedupeKey] = true

		// Enforce hourly rate limit within the batch.
		if remainingBudget >= 0 && apiCalls >= remainingBudget {
			s.logger.Info(ctx, "hourly rate limit exhausted mid-batch, stopping",
				observability.Int("api_calls", apiCalls),
				observability.Int("max_calls_per_hour", s.config.MaxCallsPerHour))
			break
		}

		if apiCalls > 0 && s.config.MaxBurstCalls > 0 && apiCalls%s.config.MaxBurstCalls == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.config.BurstPauseDuration):
			}
		}

		if apiCalls > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.config.BatchDelay):
			}
		}

		pc := pricing.Card{
			Name:            cleanName,
			Set:             card.SetName,
			Number:          cardNumber,
			PSAListingTitle: card.PSAListingTitle,
		}

		result, err := s.priceProvider.GetPrice(ctx, pc)
		apiCalls++
		if err != nil {
			s.logger.Warn(ctx, "failed to refresh price",
				observability.String("card", cleanName),
				observability.Err(err))
			errorCount++
			continue
		}

		if result == nil {
			noDataCount++
			continue
		}

		successCount++
	}

	duration := time.Since(start)
	s.logAPIUsageSummary(ctx)

	if errorCount > 0 {
		s.consecutiveFailures++
		s.lastFailureAt = time.Now()
	} else {
		s.consecutiveFailures = 0
		// lastFailureAt intentionally preserved — shows when the last failure was, even after recovery
	}

	s.logger.Info(ctx, "refresh batch completed",
		observability.Int("total", len(cards)),
		observability.Int("success", successCount),
		observability.Int("no_data", noDataCount),
		observability.Int("errors", errorCount),
		observability.Int("skipped", skippedCount),
		observability.Int("consecutive_failures", s.consecutiveFailures),
		observability.Duration("duration", duration))
}

// LastFailureAt returns the time of the last refresh failure, or zero if no failure has occurred.
func (s *PriceRefreshScheduler) LastFailureAt() time.Time {
	return s.lastFailureAt
}

// logAPIUsageSummary logs daily API usage for each provider after a refresh cycle.
func (s *PriceRefreshScheduler) logAPIUsageSummary(ctx context.Context) {
	if s.apiTracker == nil {
		return
	}

	usage, err := s.apiTracker.GetAPIUsage(ctx, pricing.SourceDH)
	if err != nil {
		s.logger.Warn(ctx, "failed to get API usage",
			observability.Err(err),
			observability.String("provider", pricing.SourceDH))
		return
	}
	if usage == nil || usage.TotalCalls == 0 {
		return
	}

	successRate := float64(usage.TotalCalls-usage.ErrorCalls) / float64(usage.TotalCalls) * 100.0

	s.logger.Info(ctx, "API daily usage",
		observability.String("provider", pricing.SourceDH),
		observability.Int("calls", int(usage.TotalCalls)),
		observability.Float64("success_rate_pct", successRate),
		observability.Float64("avg_latency_ms", usage.AvgLatencyMS),
	)
}

// Health returns the health status of the scheduler.
func (s *PriceRefreshScheduler) Health(ctx context.Context) error {
	if !s.config.Enabled {
		s.logger.Debug(ctx, "scheduler disabled, skipping health checks")
		return nil
	}

	if err := s.healthChecker.Ping(ctx); err != nil {
		return apperrors.StorageError("repository health check", err)
	}

	if !s.priceProvider.Available() {
		return apperrors.ProviderUnavailable("price_provider", fmt.Errorf("provider not configured or not available"))
	}

	return nil
}
