package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SnapshotHistoryConfig configures the snapshot history scheduler.
type SnapshotHistoryConfig struct {
	Enabled  bool
	Interval time.Duration // default: 24h
}

// ApplyDefaults fills zero-value fields with defaults.
func (c *SnapshotHistoryConfig) ApplyDefaults() {
	if c.Interval == 0 {
		c.Interval = 24 * time.Hour
	}
}

// SnapshotHistoryLister provides unsold purchases with their snapshot data.
type SnapshotHistoryLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

var _ Scheduler = (*SnapshotHistoryScheduler)(nil)

// SnapshotHistoryScheduler archives daily market snapshots from unsold inventory.
// It reads existing snapshot_json from purchase records — no API calls.
type SnapshotHistoryScheduler struct {
	StopHandle
	lister   SnapshotHistoryLister
	recorder campaigns.SnapshotHistoryRecorder
	logger   observability.Logger
	config   SnapshotHistoryConfig
}

// NewSnapshotHistoryScheduler creates a new snapshot history scheduler.
func NewSnapshotHistoryScheduler(
	lister SnapshotHistoryLister,
	recorder campaigns.SnapshotHistoryRecorder,
	logger observability.Logger,
	config SnapshotHistoryConfig,
) *SnapshotHistoryScheduler {
	config.ApplyDefaults()
	return &SnapshotHistoryScheduler{
		StopHandle: NewStopHandle(),
		lister:     lister,
		recorder:   recorder,
		logger:     logger.With(context.Background(), observability.String("component", "snapshot-history")),
		config:     config,
	}
}

// Start begins the background scheduler.
func (s *SnapshotHistoryScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "snapshot history scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "snapshot-history",
		Interval:     s.config.Interval,
		InitialDelay: 5 * time.Minute, // let inventory refresh populate snapshots first
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.tick)
}

func (s *SnapshotHistoryScheduler) tick(ctx context.Context) {
	purchases, err := s.lister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "failed to list unsold purchases", observability.Err(err))
		return
	}

	today := time.Now().Format("2006-01-02")
	archived, skipped := 0, 0

	for _, p := range purchases {
		if ctx.Err() != nil {
			return
		}

		if p.SnapshotJSON == "" {
			skipped++
			continue
		}

		var snap campaigns.MarketSnapshot
		if err := json.Unmarshal([]byte(p.SnapshotJSON), &snap); err != nil {
			skipped++
			continue
		}

		entry := campaigns.SnapshotHistoryEntry{
			CardName:            p.CardName,
			SetName:             p.SetName,
			CardNumber:          p.CardNumber,
			GradeValue:          p.GradeValue,
			MedianCents:         snap.MedianCents,
			ConservativeCents:   snap.ConservativeCents,
			OptimisticCents:     snap.OptimisticCents,
			LastSoldCents:       snap.LastSoldCents,
			LowestListCents:     snap.LowestListCents,
			EstimatedValueCents: snap.EstimatedValueCents,
			ActiveListings:      snap.ActiveListings,
			SalesLast30d:        snap.SalesLast30d,
			SalesLast90d:        snap.SalesLast90d,
			DailyVelocity:       snap.DailyVelocity,
			WeeklyVelocity:      snap.WeeklyVelocity,
			Trend30d:            snap.Trend30d,
			Trend90d:            snap.Trend90d,
			Volatility:          snap.Volatility,
			SourceCount:         snap.SourceCount,
			Confidence:          snap.Confidence,
			SnapshotJSON:        p.SnapshotJSON,
			SnapshotDate:        today,
		}

		if err := s.recorder.RecordSnapshot(ctx, entry); err != nil {
			s.logger.Warn(ctx, "failed to archive snapshot",
				observability.String("card", p.CardName),
				observability.Err(err),
			)
			continue
		}
		archived++
	}

	s.logger.Info(ctx, "snapshot history tick complete",
		observability.Int("archived", archived),
		observability.Int("skipped", skipped),
		observability.Int("total", len(purchases)),
	)
}
