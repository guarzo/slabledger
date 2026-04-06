package scheduler

import (
	"context"
	"fmt"

	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PriceRefreshScheduler handles background price refresh with rate limiting
type PriceRefreshScheduler struct {
	StopHandle
	priceRepo           pricing.PriceRepository
	apiTracker          pricing.APITracker
	healthChecker       pricing.HealthChecker
	priceProvider       pricing.PriceProvider
	logger              observability.Logger
	config              Config
	consecutiveFailures int
}

// NewPriceRefreshScheduler creates a new price refresh scheduler
func NewPriceRefreshScheduler(
	priceRepo pricing.PriceRepository,
	apiTracker pricing.APITracker,
	healthChecker pricing.HealthChecker,
	priceProvider pricing.PriceProvider,
	logger observability.Logger,
	config Config,
) *PriceRefreshScheduler {
	return &PriceRefreshScheduler{
		StopHandle:    NewStopHandle(),
		priceRepo:     priceRepo,
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

// refreshBatch processes one batch of stale prices
func (s *PriceRefreshScheduler) refreshBatch(ctx context.Context) {
	start := time.Now()

	// Get stale prices from database (prioritized by value-based staleness thresholds)
	stalePrices, err := s.priceRepo.GetStalePrices(ctx, "", s.config.BatchSize)
	if err != nil {
		s.consecutiveFailures++
		s.logger.Error(ctx, "failed to get stale prices",
			observability.Err(err),
			observability.Int("consecutive_failures", s.consecutiveFailures))
		return
	}
	s.consecutiveFailures = 0

	if len(stalePrices) == 0 {
		s.logger.Debug(ctx, "no stale prices to refresh")
		return
	}

	s.logger.Info(ctx, "refreshing stale prices",
		observability.Int("count", len(stalePrices)))

	// Group by provider to respect rate limits
	byProvider := s.groupByProvider(stalePrices)

	successCount := 0
	errorCount := 0
	skippedCount := 0

	for provider, prices := range byProvider {
		// Check if provider is blocked
		blocked, until, err := s.apiTracker.IsProviderBlocked(ctx, provider)
		if err != nil {
			s.logger.Warn(ctx, "failed to check provider block status",
				observability.String("provider", provider),
				observability.Err(err))
			continue
		}
		if blocked {
			s.logger.Warn(ctx, "provider blocked, skipping",
				observability.String("provider", provider),
				observability.Time("blocked_until", until))
			skippedCount += len(prices)
			continue
		}

		// Check hourly rate limit
		usage, err := s.apiTracker.GetAPIUsage(ctx, provider)
		if err != nil {
			s.logger.Warn(ctx, "failed to get API usage, skipping provider to avoid rate-limit risk",
				observability.String("provider", provider),
				observability.Int("prices_skipped", len(prices)),
				observability.Err(err))
			skippedCount += len(prices)
			continue
		}
		// Check hourly rate limit (treat MaxCallsPerHour == 0 as "no limit")
		if s.config.MaxCallsPerHour > 0 && usage.CallsLastHour >= int64(s.config.MaxCallsPerHour) {
			s.logger.Warn(ctx, "hourly rate limit reached, skipping provider",
				observability.String("provider", provider),
				observability.Int("calls_last_hour", int(usage.CallsLastHour)),
				observability.Int("max_calls_per_hour", s.config.MaxCallsPerHour))
			skippedCount += len(prices)
			continue
		}

		// Refresh prices for this provider with rate limiting
		seen := make(map[string]bool)
		apiCalls := 0
		for _, price := range prices {
			// Context cancellation check
			select {
			case <-ctx.Done():
				s.logger.Info(ctx, "refresh batch cancelled")
				return
			default:
			}

			// Normalize card name to expand PSA abbreviations and strip embedded card numbers.
			cleanName := cardutil.NormalizePurchaseName(price.CardName)
			cardNumber := price.CardNumber
			// Discard card numbers that are actually variant keywords or years
			// (legacy bad data from PSA-title parsing, e.g. "1ST" from "1ST EDITION").
			if cardutil.IsInvalidCardNumber(cardNumber) {
				cardNumber = ""
			}
			if cardNumber == "" {
				if extracted := cardutil.ExtractCollectorNumber(price.CardName); extracted != "" {
					cardNumber = extracted
				}
			}

			// Guard: skip if normalization produced an empty name
			if cleanName == "" {
				s.logger.Warn(ctx, "skipping refresh: normalized card name is empty",
					observability.String("original_name", price.CardName),
					observability.String("card_number", price.CardNumber))
				skippedCount++
				continue
			}

			// Deduplicate: skip cards already refreshed in this batch
			dedupeKey := cleanName + "|" + price.SetName + "|" + cardNumber
			if seen[dedupeKey] {
				s.logger.Debug(ctx, "skipping duplicate card in refresh batch",
					observability.String("card", cleanName),
					observability.String("set", price.SetName),
					observability.String("number", cardNumber))
				skippedCount++
				continue
			}
			seen[dedupeKey] = true

			// Burst limit based on actual API calls (not loop index)
			if apiCalls > 0 && apiCalls%s.config.MaxBurstCalls == 0 {
				s.logger.Debug(ctx, "burst limit reached, pausing",
					observability.String("provider", provider),
					observability.Int("api_calls", apiCalls),
					observability.Duration("pause_duration", s.config.BurstPauseDuration))
				select {
				case <-ctx.Done():
					s.logger.Info(ctx, "refresh batch cancelled during burst pause")
					return
				case <-time.After(s.config.BurstPauseDuration):
				}
			}

			// Rate limit: delay between actual API calls
			if apiCalls > 0 {
				select {
				case <-ctx.Done():
					s.logger.Info(ctx, "refresh batch cancelled during rate limit delay")
					return
				case <-time.After(s.config.BatchDelay):
				}
			}

			// Fetch fresh price under clean name
			card := pricing.Card{
				Name:            cleanName,
				Set:             price.SetName,
				Number:          cardNumber,
				PSAListingTitle: price.PSAListingTitle,
			}

			_, err := s.priceProvider.GetPrice(ctx, card)
			apiCalls++
			if err != nil {
				s.logger.Warn(ctx, "failed to refresh price",
					observability.String("card", cleanName),
					observability.String("provider", provider),
					observability.Err(err))
				errorCount++
				continue
			}

			// Delete old entries after successful refresh so data is not lost on failure
			if cleanName != price.CardName {
				if deleted, delErr := s.priceRepo.DeletePricesByCard(ctx, price.CardName, price.SetName, price.CardNumber); delErr != nil {
					s.logger.Warn(ctx, "failed to clean up old card name entries",
						observability.String("old_name", price.CardName),
						observability.String("new_name", cleanName),
						observability.Err(delErr))
				} else if deleted > 0 {
					s.logger.Info(ctx, "cleaned up corrupted card name in price history",
						observability.String("old_name", price.CardName),
						observability.String("new_name", cleanName),
						observability.Int("deleted", int(deleted)))
				}
			}

			successCount++
		}
	}

	duration := time.Since(start)

	// Log API daily usage summaries
	s.logAPIUsageSummary(ctx)

	if errorCount > 0 {
		s.consecutiveFailures++
	} else {
		s.consecutiveFailures = 0
	}

	s.logger.Info(ctx, "refresh batch completed",
		observability.Int("total", len(stalePrices)),
		observability.Int("success", successCount),
		observability.Int("errors", errorCount),
		observability.Int("skipped", skippedCount),
		observability.Int("consecutive_failures", s.consecutiveFailures),
		observability.Duration("duration", duration))

}

// groupByProvider groups stale prices by provider
func (s *PriceRefreshScheduler) groupByProvider(stalePrices []pricing.StalePrice) map[string][]pricing.StalePrice {
	byProvider := make(map[string][]pricing.StalePrice)
	for _, price := range stalePrices {
		byProvider[price.Source] = append(byProvider[price.Source], price)
	}
	return byProvider
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
