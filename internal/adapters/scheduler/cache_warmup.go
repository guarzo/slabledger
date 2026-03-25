package scheduler

import (
	"context"
	"errors"

	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*CacheWarmupScheduler)(nil)

// NewSetIDsProvider is an alias for the domain-level interface.
type NewSetIDsProvider = domainCards.NewSetIDsProvider

// CacheWarmupConfig holds configuration for the card cache warmup scheduler
type CacheWarmupConfig struct {
	Enabled        bool
	Interval       time.Duration // How often to run warmup (default: 24h)
	RateLimitDelay time.Duration // Delay between GetCards calls (default: 2s)
}

// CacheWarmupScheduler populates the persistent card cache by fetching all sets on startup and periodically
type CacheWarmupScheduler struct {
	StopHandle
	cardProvider    domainCards.CardProvider
	newSetsProvider NewSetIDsProvider
	logger          observability.Logger
	config          CacheWarmupConfig
}

// NewCacheWarmupScheduler creates a new cache warmup scheduler
func NewCacheWarmupScheduler(
	cardProvider domainCards.CardProvider,
	logger observability.Logger,
	config CacheWarmupConfig,
	newSetsProvider NewSetIDsProvider,
) *CacheWarmupScheduler {
	return &CacheWarmupScheduler{
		StopHandle:      NewStopHandle(),
		cardProvider:    cardProvider,
		newSetsProvider: newSetsProvider,
		logger:          logger.With(context.Background(), observability.String("component", "cache-warmup")),
		config:          config,
	}
}

// Start begins the background warmup scheduler
func (s *CacheWarmupScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "card cache warmup disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "cache-warmup",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
		LogFields: []observability.Field{
			observability.Duration("rate_limit_delay", s.config.RateLimitDelay),
		},
	}, s.warmup)
}

// Health returns the health status of the scheduler
func (s *CacheWarmupScheduler) Health(_ context.Context) error {
	return nil
}

// warmup fetches all sets and their cards to populate the persistent cache
func (s *CacheWarmupScheduler) warmup(ctx context.Context) {
	start := time.Now()

	sets, err := s.cardProvider.ListAllSets(ctx)
	if err != nil {
		s.logger.Warn(ctx, "cache warmup: failed to list sets, will retry next cycle",
			observability.Err(err))
		return
	}

	// Filter to un-finalized sets only (skip sets already cached on disk)
	setIDs := make([]string, len(sets))
	setMap := make(map[string]domainCards.Set, len(sets))
	for i, set := range sets {
		setIDs[i] = set.ID
		setMap[set.ID] = set
	}

	targetIDs := setIDs
	if s.newSetsProvider != nil {
		newIDs, err := s.newSetsProvider.GetNewSetIDs(ctx, setIDs)
		if err != nil {
			s.logger.Warn(ctx, "cache warmup: failed to filter finalized sets, fetching all",
				observability.Err(err))
		} else {
			skipped := len(setIDs) - len(newIDs)
			if skipped > 0 {
				s.logger.Info(ctx, "cache warmup: skipping finalized sets",
					observability.Int("skipped", skipped),
					observability.Int("remaining", len(newIDs)))
			}
			targetIDs = newIDs
		}
	}

	s.logger.Info(ctx, "cache warmup: fetching cards for sets",
		observability.Int("total_sets", len(sets)),
		observability.Int("sets_to_fetch", len(targetIDs)))

	fetched := 0
	errCount := 0
	skipped := 0
	consecutiveErrors := 0
	const maxConsecutiveErrors = 5

	for _, setID := range targetIDs {
		select {
		case <-ctx.Done():
			s.logger.Info(ctx, "cache warmup cancelled")
			return
		case <-s.Done():
			s.logger.Info(ctx, "cache warmup stopped")
			return
		default:
		}

		// Early abort if too many consecutive failures (API likely down)
		if consecutiveErrors >= maxConsecutiveErrors {
			s.logger.Warn(ctx, "cache warmup: aborting cycle due to consecutive errors",
				observability.Int("consecutive_errors", consecutiveErrors),
				observability.Int("fetched_before_abort", fetched))
			break
		}

		_, err := s.cardProvider.GetCards(ctx, setID)
		if err != nil {
			// 404 (not found) means the set doesn't exist in the provider —
			// skip gracefully without counting toward consecutive errors.
			var appErr *apperrors.AppError
			if errors.As(err, &appErr) && appErr.Code == apperrors.ErrCodeProviderNotFound {
				s.logger.Debug(ctx, "cache warmup: set not found, skipping",
					observability.String("set_id", setID))
				skipped++
				consecutiveErrors = 0
			} else {
				s.logger.Debug(ctx, "cache warmup: failed to fetch set",
					observability.String("set_id", setID),
					observability.Err(err))
				errCount++
				consecutiveErrors++
			}
		} else {
			fetched++
			consecutiveErrors = 0
		}

		// Rate limit between sets
		select {
		case <-ctx.Done():
			return
		case <-s.Done():
			return
		case <-time.After(s.config.RateLimitDelay):
		}
	}

	s.logger.Info(ctx, "cache warmup completed",
		observability.Int("sets_fetched", fetched),
		observability.Int("sets_not_found", skipped),
		observability.Int("errors", errCount),
		observability.Duration("duration", time.Since(start)))
}
