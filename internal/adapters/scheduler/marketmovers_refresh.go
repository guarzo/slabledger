package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// MMPurchaseLister lists unsold purchases for Market Movers value sync.
type MMPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

// MMValueUpdater updates Market Movers values on purchases.
type MMValueUpdater interface {
	UpdatePurchaseMMValue(ctx context.Context, purchaseID string, mmValueCents int) error
}

// MarketMoversRefreshConfig controls the scheduler behaviour.
type MarketMoversRefreshConfig struct {
	Enabled     bool
	RefreshHour int
}

// MarketMoversRefreshScheduler refreshes MM values from the Market Movers API daily.
type MarketMoversRefreshScheduler struct {
	StopHandle
	client         *marketmovers.Client
	store          *sqlite.MarketMoversStore
	purchaseLister MMPurchaseLister
	valueUpdater   MMValueUpdater
	logger         observability.Logger
	config         MarketMoversRefreshConfig
}

// NewMarketMoversRefreshScheduler creates a new Market Movers refresh scheduler.
func NewMarketMoversRefreshScheduler(
	client *marketmovers.Client,
	store *sqlite.MarketMoversStore,
	purchaseLister MMPurchaseLister,
	valueUpdater MMValueUpdater,
	logger observability.Logger,
	cfg MarketMoversRefreshConfig,
) *MarketMoversRefreshScheduler {
	return &MarketMoversRefreshScheduler{
		StopHandle:     NewStopHandle(),
		client:         client,
		store:          store,
		purchaseLister: purchaseLister,
		valueUpdater:   valueUpdater,
		logger:         logger,
		config:         cfg,
	}
}

// Start begins the daily scheduler loop.
func (s *MarketMoversRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "Market Movers refresh scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "market-movers-refresh",
		Interval:     24 * time.Hour,
		InitialDelay: timeUntilHour(time.Now(), s.config.RefreshHour),
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
		LogFields:    []observability.Field{observability.Int("refreshHour", s.config.RefreshHour)},
	}, func(ctx context.Context) {
		s.runOnce(ctx) //nolint:errcheck
	})
}

// RunOnce runs a single refresh cycle. Exported for manual trigger.
func (s *MarketMoversRefreshScheduler) RunOnce(ctx context.Context) error {
	return s.runOnce(ctx)
}

func (s *MarketMoversRefreshScheduler) runOnce(ctx context.Context) error {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		s.logger.Error(ctx, "MM refresh: failed to load config", observability.Err(err))
		return err
	}
	if cfg == nil {
		s.logger.Debug(ctx, "MM refresh: not configured, skipping")
		return nil
	}

	// List all unsold purchases
	purchases, err := s.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "MM refresh: failed to list purchases", observability.Err(err))
		return err
	}

	// Load all existing mappings keyed by cert
	existingMappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Warn(ctx, "MM refresh: failed to list mappings", observability.Err(err))
	}
	mappingByCert := make(map[string]int64, len(existingMappings))
	for _, m := range existingMappings {
		mappingByCert[m.SlabSerial] = m.MMCollectibleID
	}

	updated, mapped, skipped, searchFailed := 0, 0, 0, 0

	for i := range purchases {
		p := &purchases[i]

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Resolve collectible ID — use cached mapping or search the API
		collectibleID, ok := mappingByCert[p.CertNumber]
		if !ok {
			id, err := s.resolveCollectibleID(ctx, p)
			if err != nil {
				s.logger.Warn(ctx, "MM refresh: failed to resolve collectible ID",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
				searchFailed++
				continue
			}
			if id == 0 {
				skipped++
				continue
			}
			collectibleID = id
			if err := s.store.SaveMapping(ctx, p.CertNumber, collectibleID); err != nil {
				s.logger.Warn(ctx, "MM refresh: failed to save mapping",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			} else {
				mappingByCert[p.CertNumber] = collectibleID
				mapped++
			}
		}

		// Fetch avg recent price (last 30 days)
		avgPrice, err := s.client.AvgRecentPrice(ctx, collectibleID, 30)
		if err != nil {
			s.logger.Warn(ctx, "MM refresh: failed to fetch price",
				observability.String("cert", p.CertNumber),
				observability.Int64("collectibleId", collectibleID),
				observability.Err(err))
			continue
		}
		if avgPrice <= 0 {
			continue
		}

		mmValueCents := mathutil.ToCentsInt(avgPrice)
		if err := s.valueUpdater.UpdatePurchaseMMValue(ctx, p.ID, mmValueCents); err != nil {
			s.logger.Warn(ctx, "MM refresh: failed to update MM value",
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			continue
		}
		updated++
	}

	s.logger.Info(ctx, "MM refresh: complete",
		observability.Int("updated", updated),
		observability.Int("newMappings", mapped),
		observability.Int("skipped", skipped),
		observability.Int("searchFailed", searchFailed),
		observability.Int("totalPurchases", len(purchases)))
	return nil
}

// resolveCollectibleID searches Market Movers for the card and returns its collectible ID.
// Returns 0 if no suitable result is found.
func (s *MarketMoversRefreshScheduler) resolveCollectibleID(ctx context.Context, p *campaigns.Purchase) (int64, error) {
	if p.CardName == "" {
		return 0, nil
	}

	// Build a search query: "{CardName} {Grader} {Grade}"
	grader := p.Grader
	if grader == "" {
		grader = "PSA"
	}
	query := fmt.Sprintf("%s %s %s", p.CardName, grader, formatGrade(p.GradeValue))
	// Trim extra spaces from zero-grade edge case
	query = strings.TrimSpace(query)

	results, err := s.client.SearchCollectibles(ctx, query, 0, 5)
	if err != nil {
		return 0, fmt.Errorf("search collectibles: %w", err)
	}
	if len(results.Items) == 0 {
		return 0, nil
	}

	// Take the first result (highest relevance score from the index)
	return results.Items[0].Item.ID, nil
}
