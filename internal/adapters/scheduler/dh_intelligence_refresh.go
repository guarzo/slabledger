package scheduler

import (
	"context"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*DHIntelligenceRefreshScheduler)(nil)

// DHIntelligenceRefreshConfig holds configuration for the DH intelligence refresh scheduler.
type DHIntelligenceRefreshConfig struct {
	Enabled   bool
	Interval  time.Duration // default 1h
	CacheTTL  time.Duration // from config.DH.CacheTTLHours
	MaxPerRun int           // default 50
}

// IntelligenceSeedLister returns cards we own that should have a market_intelligence
// row. The scheduler diffs this list against existing rows and seeds the missing ones.
type IntelligenceSeedLister interface {
	ListUnsoldDHCardSeeds(ctx context.Context) ([]intelligence.SeedCandidate, error)
}

// DHIntelligenceRefreshScheduler periodically seeds market_intelligence with cards
// we own and refreshes stale entries by re-fetching from the DH API.
type DHIntelligenceRefreshScheduler struct {
	StopHandle
	dhClient   *dh.Client
	intelRepo  intelligence.Repository
	seedLister IntelligenceSeedLister // optional; when nil, only the refresh pass runs
	logger     observability.Logger
	config     DHIntelligenceRefreshConfig
}

// IntelligenceRefreshOption configures optional dependencies on the scheduler.
type IntelligenceRefreshOption func(*DHIntelligenceRefreshScheduler)

// WithIntelligenceSeedLister enables the seed pass that creates initial
// market_intelligence rows for cards in unsold inventory. Without this,
// the scheduler only refreshes rows that already exist — meaning an empty
// table stays empty.
func WithIntelligenceSeedLister(l IntelligenceSeedLister) IntelligenceRefreshOption {
	return func(s *DHIntelligenceRefreshScheduler) { s.seedLister = l }
}

// NewDHIntelligenceRefreshScheduler creates a new DH intelligence refresh scheduler.
func NewDHIntelligenceRefreshScheduler(
	dhClient *dh.Client,
	intelRepo intelligence.Repository,
	logger observability.Logger,
	config DHIntelligenceRefreshConfig,
	opts ...IntelligenceRefreshOption,
) *DHIntelligenceRefreshScheduler {
	if config.Interval == 0 {
		config.Interval = 1 * time.Hour
	}
	if config.MaxPerRun == 0 {
		config.MaxPerRun = 50
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 24 * time.Hour
	}

	s := &DHIntelligenceRefreshScheduler{
		StopHandle: NewStopHandle(),
		dhClient:   dhClient,
		intelRepo:  intelRepo,
		logger:     logger.With(context.Background(), observability.String("component", "dh-intelligence-refresh")),
		config:     config,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the background intelligence refresh scheduler.
func (s *DHIntelligenceRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "DH intelligence refresh scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-intelligence-refresh",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.refresh)
}

// refresh seeds intelligence for cards we own that don't have a row yet, then
// re-fetches stale entries. Both phases share the per-run budget so a
// long-tail seed can't starve refreshes.
func (s *DHIntelligenceRefreshScheduler) refresh(ctx context.Context) {
	s.logger.Debug(ctx, "running DH intelligence refresh")

	budget := s.seed(ctx, s.config.MaxPerRun)
	if budget <= 0 {
		return
	}

	stale, err := s.intelRepo.GetStale(ctx, s.config.CacheTTL, budget)
	if err != nil {
		s.logger.Error(ctx, "failed to get stale intelligence entries", observability.Err(err))
		return
	}

	if len(stale) == 0 {
		s.logger.Debug(ctx, "no stale intelligence entries to refresh")
		return
	}

	s.logger.Info(ctx, "refreshing stale DH intelligence",
		observability.Int("count", len(stale)))

	var refreshed, failed int
	for _, entry := range stale {
		cardIDInt, convErr := strconv.Atoi(entry.DHCardID)
		if convErr != nil {
			s.logger.Warn(ctx, "skipping non-numeric DH card ID (legacy or malformed)",
				observability.String("dh_card_id", entry.DHCardID),
				observability.String("card_name", entry.CardName),
				observability.Err(convErr))
			failed++
			continue
		}
		resp, fetchErr := s.dhClient.MarketDataEnterprise(ctx, cardIDInt)
		if fetchErr != nil {
			s.logger.Warn(ctx, "failed to fetch DH market data",
				observability.String("dh_card_id", entry.DHCardID),
				observability.Err(fetchErr))
			failed++
			continue
		}

		if !resp.HasData {
			s.logger.Debug(ctx, "DH market data has no data",
				observability.String("dh_card_id", entry.DHCardID))
			// Update fetched_at so this entry isn't re-selected every run
			entry.FetchedAt = time.Now()
			if storeErr := s.intelRepo.Store(ctx, &entry); storeErr != nil {
				s.logger.Warn(ctx, "failed to update fetched_at for empty DH entry",
					observability.String("dh_card_id", entry.DHCardID),
					observability.Err(storeErr))
			}
			continue
		}

		intel := dh.ConvertToIntelligence(resp, entry.CardName, entry.SetName, entry.CardNumber, entry.DHCardID)

		if storeErr := s.intelRepo.Store(ctx, intel); storeErr != nil {
			s.logger.Warn(ctx, "failed to store refreshed intelligence",
				observability.String("dh_card_id", entry.DHCardID),
				observability.Err(storeErr))
			failed++
			continue
		}

		refreshed++
	}

	s.logger.Info(ctx, "DH intelligence refresh complete",
		observability.Int("refreshed", refreshed),
		observability.Int("failed", failed))
}

// seed creates initial market_intelligence rows for unsold inventory cards
// that don't yet have one. Returns the remaining per-run budget for the
// subsequent refresh phase.
func (s *DHIntelligenceRefreshScheduler) seed(ctx context.Context, budget int) int {
	if s.seedLister == nil || budget <= 0 {
		return budget
	}

	candidates, err := s.seedLister.ListUnsoldDHCardSeeds(ctx)
	if err != nil {
		s.logger.Error(ctx, "failed to list intelligence seed candidates", observability.Err(err))
		return budget
	}
	if len(candidates) == 0 {
		return budget
	}

	var seeded, skipped, failed int
	for _, c := range candidates {
		if budget <= 0 {
			break
		}
		existing, err := s.intelRepo.GetByDHCardID(ctx, c.DHCardID)
		if err != nil {
			s.logger.Warn(ctx, "failed to check existing intelligence",
				observability.String("dh_card_id", c.DHCardID),
				observability.Err(err))
			failed++
			continue
		}
		if existing != nil {
			skipped++
			continue
		}

		cardIDInt, convErr := strconv.Atoi(c.DHCardID)
		if convErr != nil {
			s.logger.Warn(ctx, "skipping non-numeric DH card ID seed",
				observability.String("dh_card_id", c.DHCardID),
				observability.String("card_name", c.CardName),
				observability.Err(convErr))
			failed++
			continue
		}

		resp, fetchErr := s.dhClient.MarketDataEnterprise(ctx, cardIDInt)
		if fetchErr != nil {
			s.logger.Warn(ctx, "failed to fetch DH market data for seed",
				observability.String("dh_card_id", c.DHCardID),
				observability.Err(fetchErr))
			failed++
			budget--
			continue
		}
		budget--

		intel := dh.ConvertToIntelligence(resp, c.CardName, c.SetName, c.CardNumber, c.DHCardID)
		if storeErr := s.intelRepo.Store(ctx, intel); storeErr != nil {
			s.logger.Warn(ctx, "failed to store seeded intelligence",
				observability.String("dh_card_id", c.DHCardID),
				observability.Err(storeErr))
			failed++
			continue
		}
		seeded++
	}

	if seeded > 0 || failed > 0 {
		s.logger.Info(ctx, "DH intelligence seed complete",
			observability.Int("seeded", seeded),
			observability.Int("skipped_existing", skipped),
			observability.Int("failed", failed),
			observability.Int("remaining_budget", budget))
	}
	return budget
}
