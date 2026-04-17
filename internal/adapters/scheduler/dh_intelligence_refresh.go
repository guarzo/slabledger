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

	analyticsByID := s.fetchBatchAnalytics(ctx, dhCardIDsFromStale(stale))

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
			dh.MergeAnalyticsIntoIntelligence(&entry, analyticsByID[cardIDInt])
			if storeErr := s.intelRepo.Store(ctx, &entry); storeErr != nil {
				s.logger.Warn(ctx, "failed to update fetched_at for empty DH entry",
					observability.String("dh_card_id", entry.DHCardID),
					observability.Err(storeErr))
			}
			continue
		}

		intel := dh.ConvertToIntelligence(resp, entry.CardName, entry.SetName, entry.CardNumber, entry.DHCardID)
		dh.MergeAnalyticsIntoIntelligence(intel, analyticsByID[cardIDInt])

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

// fetchBatchAnalytics calls DH's batch_analytics for the given card IDs and
// returns a per-card-id map. Failures degrade gracefully to an empty map so
// the refresh loop keeps running on the market-data path.
func (s *DHIntelligenceRefreshScheduler) fetchBatchAnalytics(ctx context.Context, cardIDs []int) map[int]*dh.CardAnalytics {
	if len(cardIDs) == 0 {
		return nil
	}
	resp, err := s.dhClient.BatchAnalytics(ctx, cardIDs, []string{"velocity", "trend"})
	if err != nil {
		s.logger.Warn(ctx, "batch_analytics failed; refresh will continue without velocity/volume",
			observability.Int("card_ids", len(cardIDs)),
			observability.Err(err))
		return nil
	}
	out := make(map[int]*dh.CardAnalytics, len(resp.Results))
	for i := range resp.Results {
		r := &resp.Results[i]
		out[r.CardID] = r
	}
	return out
}

// dhCardIDsFromStale returns the numeric card IDs from a slice of stale
// intelligence entries, skipping any with non-numeric IDs (same skip rule
// used in the main refresh loop).
func dhCardIDsFromStale(stale []intelligence.MarketIntelligence) []int {
	out := make([]int, 0, len(stale))
	for _, e := range stale {
		if id, err := strconv.Atoi(e.DHCardID); err == nil {
			out = append(out, id)
		}
	}
	return out
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

	// Filter to candidates we'll actually fetch (not already stored, numeric ID),
	// so batch_analytics is called once for the whole seed set.
	fetchables := make([]intelligence.SeedCandidate, 0, len(candidates))
	cardIDs := make([]int, 0, len(candidates))
	var skipped, failed int
	for _, c := range candidates {
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
		id, convErr := strconv.Atoi(c.DHCardID)
		if convErr != nil {
			s.logger.Warn(ctx, "skipping non-numeric DH card ID seed",
				observability.String("dh_card_id", c.DHCardID),
				observability.String("card_name", c.CardName),
				observability.Err(convErr))
			failed++
			continue
		}
		fetchables = append(fetchables, c)
		cardIDs = append(cardIDs, id)
	}

	// Trim to budget before the batch_analytics call.
	if len(fetchables) > budget {
		fetchables = fetchables[:budget]
		cardIDs = cardIDs[:budget]
	}
	analyticsByID := s.fetchBatchAnalytics(ctx, cardIDs)

	var seeded int
	for i, c := range fetchables {
		cardIDInt := cardIDs[i]
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
		dh.MergeAnalyticsIntoIntelligence(intel, analyticsByID[cardIDInt])
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
