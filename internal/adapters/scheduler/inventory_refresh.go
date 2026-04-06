package scheduler

import (
	"context"
	"sort"

	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// InventoryPurchase holds the data the scheduler needs to refresh a purchase's snapshot.
type InventoryPurchase struct {
	ID              string
	CardName        string
	CardNumber      string
	SetName         string
	GradeValue      float64
	Grader          string
	BuyCostCents    int
	CLValueCents    int
	PSAListingTitle string // Raw PSA listing title for pricing fallback
	SnapshotDate    string // "" means never captured
}

// InventoryLister provides unsold inventory purchases for snapshot refresh.
type InventoryLister interface {
	ListUnsoldInventory(ctx context.Context) ([]InventoryPurchase, error)
}

// SnapshotRefresher refreshes the market snapshot for a single purchase.
type SnapshotRefresher interface {
	RefreshSnapshot(ctx context.Context, p InventoryPurchase) bool
}

var _ Scheduler = (*InventoryRefreshScheduler)(nil)

// InventoryRefreshScheduler periodically refreshes market snapshots on unsold purchases.
type InventoryRefreshScheduler struct {
	StopHandle
	lister    InventoryLister
	refresher SnapshotRefresher
	logger    observability.Logger
	config    config.InventoryRefreshConfig
}

// NewInventoryRefreshScheduler creates a new inventory refresh scheduler.
func NewInventoryRefreshScheduler(
	lister InventoryLister,
	refresher SnapshotRefresher,
	logger observability.Logger,
	cfg config.InventoryRefreshConfig,
) *InventoryRefreshScheduler {
	cfg.ApplyDefaults()
	return &InventoryRefreshScheduler{
		StopHandle: NewStopHandle(),
		lister:     lister,
		refresher:  refresher,
		logger:     logger.With(context.Background(), observability.String("component", "inventory-refresh")),
		config:     cfg,
	}
}

// Start begins the background refresh scheduler.
func (s *InventoryRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "inventory refresh scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "inventory-refresh",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
		LogFields: []observability.Field{
			observability.Duration("stale_threshold", s.config.StaleThreshold),
			observability.Int("batch_size", s.config.BatchSize),
		},
	}, s.refreshBatch)
}

// refreshBatch processes one batch of stale inventory snapshots.
func (s *InventoryRefreshScheduler) refreshBatch(ctx context.Context) {
	start := time.Now()

	purchases, err := s.lister.ListUnsoldInventory(ctx)
	if err != nil {
		s.logger.Error(ctx, "failed to list unsold inventory", observability.Err(err))
		return
	}

	// Filter to stale purchases and prioritize by value (highest first)
	stale := s.filterStale(ctx, purchases)
	if len(stale) == 0 {
		s.logger.Debug(ctx, "no stale inventory snapshots to refresh")
		return
	}

	// Sort by value descending — refresh high-value cards first
	sort.Slice(stale, func(i, j int) bool {
		return stale[i].BuyCostCents > stale[j].BuyCostCents
	})

	// No dedup here — RefreshSnapshot must run per purchase (each has its
	// own snapshot record keyed by purchase ID). Identical price lookups
	// are already coalesced at the DH provider layer via singleflight
	// and in-memory cache, so duplicate API calls don't happen.

	// Cap at batch size
	if len(stale) > s.config.BatchSize {
		stale = stale[:s.config.BatchSize]
	}

	s.logger.Info(ctx, "refreshing inventory snapshots",
		observability.Int("stale_count", len(stale)),
		observability.Int("total_unsold", len(purchases)))

	successCount := 0
	errorCount := 0

	for i, p := range stale {
		select {
		case <-ctx.Done():
			s.logger.Info(ctx, "inventory refresh cancelled")
			return
		case <-s.Done():
			s.logger.Info(ctx, "inventory refresh stopped")
			return
		default:
		}

		// Rate limit between calls
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.Done():
				return
			case <-time.After(s.config.BatchDelay):
			}
		}

		if s.refresher.RefreshSnapshot(ctx, p) {
			successCount++
		} else {
			errorCount++
		}
	}

	s.logger.Info(ctx, "inventory refresh batch completed",
		observability.Int("success", successCount),
		observability.Int("errors", errorCount),
		observability.Int("total", len(stale)),
		observability.Duration("duration", time.Since(start)))
}

// filterStale returns purchases whose snapshot is missing or older than the threshold.
func (s *InventoryRefreshScheduler) filterStale(ctx context.Context, purchases []InventoryPurchase) []InventoryPurchase {
	cutoff := time.Now().Add(-s.config.StaleThreshold)
	var result []InventoryPurchase
	for i, p := range purchases {
		if p.CardName == "" || p.GradeValue <= 0 {
			s.logger.Debug(ctx, "inventory refresh: filtering stale purchase",
				observability.Int("index", i),
				observability.String("id", p.ID),
				observability.String("cardName", p.CardName),
				observability.Float64("gradeValue", p.GradeValue))
			continue
		}
		// No snapshot yet — treat as stale
		if p.SnapshotDate == "" {
			result = append(result, p)
			continue
		}
		// Parse snapshot date and compare to cutoff
		snapshotTime, err := time.Parse("2006-01-02", p.SnapshotDate)
		if err != nil {
			// Unparseable date — treat as stale
			result = append(result, p)
			continue
		}
		if snapshotTime.Before(cutoff) {
			result = append(result, p)
		}
	}
	return result
}
