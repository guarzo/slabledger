package scheduler

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*CardTrajectoryRefreshScheduler)(nil)

// CardTrajectoryRefreshConfig controls the weekly trajectory refresh cadence.
// Disabled by default — flip Enabled once DH is populating
// graded-sales-analytics.recent_sales reliably (as of 2026-04-18 the field
// is returned empty even when total_sales > 0, which would cause every run
// to upsert zero buckets).
type CardTrajectoryRefreshConfig struct {
	Enabled        bool
	Interval       time.Duration // default 7 days
	MaxPerRun      int           // default 50
	GradingCompany string        // default "PSA"
	Grade          string        // default "10"
}

// CardTrajectoryRefreshScheduler pulls DH graded-sales-analytics for owned
// cards, buckets sales into weekly aggregates, and upserts into
// card_price_trajectory. Owned cards come from the same seed lister the
// intelligence refresh uses.
type CardTrajectoryRefreshScheduler struct {
	StopHandle
	dhClient      *dh.Client
	trajectoryRep intelligence.TrajectoryRepository
	seedLister    IntelligenceSeedLister
	logger        observability.Logger
	config        CardTrajectoryRefreshConfig
}

// NewCardTrajectoryRefreshScheduler constructs the scheduler.
func NewCardTrajectoryRefreshScheduler(
	dhClient *dh.Client,
	trajectoryRep intelligence.TrajectoryRepository,
	seedLister IntelligenceSeedLister,
	logger observability.Logger,
	config CardTrajectoryRefreshConfig,
) *CardTrajectoryRefreshScheduler {
	if config.Interval == 0 {
		config.Interval = 7 * 24 * time.Hour
	}
	if config.MaxPerRun == 0 {
		config.MaxPerRun = 50
	}
	if config.GradingCompany == "" {
		config.GradingCompany = "PSA"
	}
	if config.Grade == "" {
		config.Grade = "10"
	}
	return &CardTrajectoryRefreshScheduler{
		StopHandle:    NewStopHandle(),
		dhClient:      dhClient,
		trajectoryRep: trajectoryRep,
		seedLister:    seedLister,
		logger:        logger.With(context.Background(), observability.String("component", "card-trajectory-refresh")),
		config:        config,
	}
}

// Start begins the background refresh loop.
func (s *CardTrajectoryRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "card trajectory refresh scheduler disabled")
		return
	}
	RunLoop(ctx, LoopConfig{
		Name:     "card-trajectory-refresh",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.refresh)
}

func (s *CardTrajectoryRefreshScheduler) refresh(ctx context.Context) {
	if s.seedLister == nil {
		return
	}
	candidates, err := s.seedLister.ListUnsoldDHCardSeeds(ctx)
	if err != nil {
		s.logger.Error(ctx, "failed to list card seeds for trajectory refresh", observability.Err(err))
		return
	}
	if len(candidates) == 0 {
		return
	}
	budget := s.config.MaxPerRun
	var updated, skipped, failed int
	for _, c := range candidates {
		if budget <= 0 {
			break
		}
		id, convErr := strconv.Atoi(c.DHCardID)
		if convErr != nil {
			skipped++
			continue
		}
		resp, fetchErr := s.dhClient.GradedSalesAnalytics(ctx, id, s.config.GradingCompany, s.config.Grade)
		budget--
		if fetchErr != nil {
			s.logger.Warn(ctx, "graded-sales-analytics fetch failed",
				observability.String("dh_card_id", c.DHCardID),
				observability.Err(fetchErr))
			failed++
			continue
		}
		if len(resp.RecentSales) == 0 {
			// DH gap today — skip writes so we don't clobber existing buckets
			// with empty data. Once DH populates recent_sales, this branch
			// stops firing.
			skipped++
			continue
		}
		sales := convertRecentSales(resp.RecentSales)
		buckets := intelligence.BucketSalesByWeek(sales)
		if len(buckets) == 0 {
			skipped++
			continue
		}
		if err := s.trajectoryRep.Upsert(ctx, c.DHCardID, buckets); err != nil {
			s.logger.Warn(ctx, "failed to upsert trajectory",
				observability.String("dh_card_id", c.DHCardID),
				observability.Err(err))
			failed++
			continue
		}
		updated++
	}
	s.logger.Info(ctx, "card trajectory refresh complete",
		observability.Int("updated", updated),
		observability.Int("skipped", skipped),
		observability.Int("failed", failed))
}

// convertRecentSales maps DH's RecentSale rows to the domain Sale shape the
// bucketing aggregator expects. Sales with unparseable timestamps are
// dropped (BucketSalesByWeek would skip them anyway, but doing it here keeps
// the domain layer agnostic of DH's string formats).
func convertRecentSales(sales []dh.RecentSale) []intelligence.Sale {
	out := make([]intelligence.Sale, 0, len(sales))
	for _, s := range sales {
		t, err := time.Parse("2006-01-02", s.SoldAt)
		if err != nil {
			// Fall back to RFC3339 in case DH ever returns full timestamps.
			t, err = time.Parse(time.RFC3339, s.SoldAt)
			if err != nil {
				continue
			}
		}
		out = append(out, intelligence.Sale{
			SoldAt:         t,
			GradingCompany: s.GradingCompany,
			Grade:          s.Grade,
			PriceCents:     int64(math.Round(s.Price * 100)),
			Platform:       s.Platform,
		})
	}
	return out
}
