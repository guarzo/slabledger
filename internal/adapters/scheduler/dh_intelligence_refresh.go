package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/doubleholo"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*DHIntelligenceRefreshScheduler)(nil)

// DHIntelligenceRefreshConfig holds configuration for the DH intelligence refresh scheduler.
type DHIntelligenceRefreshConfig struct {
	Enabled   bool
	Interval  time.Duration // default 1h
	CacheTTL  time.Duration // from config.DoubleHolo.CacheTTLHours
	MaxPerRun int           // default 50
}

// DHIntelligenceRefreshScheduler periodically refreshes stale market intelligence
// by re-fetching data from the DoubleHolo API.
type DHIntelligenceRefreshScheduler struct {
	StopHandle
	dhClient  *doubleholo.Client
	intelRepo intelligence.Repository
	logger    observability.Logger
	config    DHIntelligenceRefreshConfig
}

// NewDHIntelligenceRefreshScheduler creates a new DH intelligence refresh scheduler.
func NewDHIntelligenceRefreshScheduler(
	dhClient *doubleholo.Client,
	intelRepo intelligence.Repository,
	logger observability.Logger,
	config DHIntelligenceRefreshConfig,
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

	return &DHIntelligenceRefreshScheduler{
		StopHandle: NewStopHandle(),
		dhClient:   dhClient,
		intelRepo:  intelRepo,
		logger:     logger.With(context.Background(), observability.String("component", "dh-intelligence-refresh")),
		config:     config,
	}
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

// refresh fetches stale intelligence entries and re-fetches market data from DH.
func (s *DHIntelligenceRefreshScheduler) refresh(ctx context.Context) {
	s.logger.Debug(ctx, "running DH intelligence refresh")

	stale, err := s.intelRepo.GetStale(ctx, s.config.CacheTTL, s.config.MaxPerRun)
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
		resp, fetchErr := s.dhClient.MarketData(ctx, entry.DHCardID)
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

		intel := doubleholo.ConvertToIntelligence(resp, entry.CardName, entry.SetName, entry.CardNumber, entry.DHCardID)

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
