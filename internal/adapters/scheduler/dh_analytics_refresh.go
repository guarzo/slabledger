package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

var _ Scheduler = (*DHAnalyticsRefreshScheduler)(nil)

// dhAnalyticsClient is the subset of *dh.Client used by the refresh scheduler.
// Extracted to an interface so tests can supply a fake without standing up an
// HTTP server.
type dhAnalyticsClient interface {
	EnterpriseAvailable() bool
	TopCharacters(ctx context.Context, limit int, era string) (*dh.TopCharactersResponse, error)
	CharacterVelocity(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterVelocityResponse, error)
	CharacterSaturation(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterSaturationResponse, error)
	CharacterDemand(ctx context.Context, cardIDs []int, window string, byEra bool) (*dh.CharacterDemandResponse, error)
	BatchAnalytics(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error)
	DemandSignals(ctx context.Context, cardIDs []int, window string) (*dh.DemandSignalsResponse, error)
}

// UnsoldDHCardLister returns the distinct dh_card_id values for our unsold
// inventory. Implemented by *postgres.PurchaseStore.ListUnsoldDHCardIDs.
type UnsoldDHCardLister interface {
	ListUnsoldDHCardIDs(ctx context.Context) ([]int, error)
}

// Phase-1 era allowlist. Confirmed against enterprise-api.yaml `era` enum
// (leaderboard endpoint). Expanded cautiously — DH computes these nightly.
var defaultAnalyticsEras = []string{
	"wotc",
	"ex",
	"dp",
	"platinum",
	"hgss",
	"bw",
	"xy",
	"sun_moon",
	"sword_shield",
	"scarlet_violet",
}

const (
	topCharactersOverallLimit = 50
	topCharactersPerEraLimit  = 20
	characterListPerPage      = 50
	maxCharactersPerRun       = 200
	maxSeedCardIDs            = 1000
)

// DHAnalyticsRefreshScheduler pulls DH demand + analytics signals once per day
// and caches them in dh_card_cache / dh_character_cache via demand.Repository.
// See /home/vscode/.claude/plans/buzzing-knitting-seahorse.md for the full
// design (T4). Step implementations live in dh_analytics_refresh_steps.go.
type DHAnalyticsRefreshScheduler struct {
	StopHandle
	dhClient   dhAnalyticsClient
	repo       demand.Repository
	cardLister UnsoldDHCardLister
	logger     observability.Logger
	config     config.DHAnalyticsRefreshConfig
}

// NewDHAnalyticsRefreshScheduler constructs the scheduler. cardLister may be
// nil; callers that omit it lose the "seed with unsold inventory" step but the
// hot-lists from step 1 still drive step 2.
func NewDHAnalyticsRefreshScheduler(
	dhClient dhAnalyticsClient,
	repo demand.Repository,
	cardLister UnsoldDHCardLister,
	logger observability.Logger,
	cfg config.DHAnalyticsRefreshConfig,
) *DHAnalyticsRefreshScheduler {
	if cfg.Window == "" {
		cfg.Window = "30d"
	}
	return &DHAnalyticsRefreshScheduler{
		StopHandle: NewStopHandle(),
		dhClient:   dhClient,
		repo:       repo,
		cardLister: cardLister,
		logger:     logger.With(context.Background(), observability.String("component", "dh-analytics-refresh")),
		config:     cfg,
	}
}

// Start begins the background loop. Each tick runs the full pipeline.
func (s *DHAnalyticsRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "DH analytics refresh scheduler disabled")
		return
	}
	initialDelay := timeUntilHour(time.Now(), s.config.RefreshHour)
	RunLoop(ctx, LoopConfig{
		Name:         "dh-analytics-refresh",
		Interval:     24 * time.Hour,
		InitialDelay: initialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.refresh)
}

// refresh runs the 3-step pipeline. Errors in any single step are logged and
// allowed to fall through so later steps still attempt to produce data.
func (s *DHAnalyticsRefreshScheduler) refresh(ctx context.Context) {
	if !s.dhClient.EnterpriseAvailable() {
		s.logger.Info(ctx, "DH enterprise key not configured; skipping analytics refresh")
		return
	}
	start := time.Now()
	s.logger.Info(ctx, "DH analytics refresh starting",
		observability.String("window", s.config.Window))

	// Step 1: characters.
	characters, topCardIDs, charCalls := s.refreshCharacters(ctx)

	// Step 2: cards (inventory ∪ hot-list top cards).
	cardIDs := s.buildCardSeed(ctx, topCardIDs)
	notComputed, cardCalls := s.refreshCards(ctx, cardIDs)

	// Step 3: metrics.
	stats, statsErr := s.repo.CardDataQualityStats(ctx, s.config.Window)
	if statsErr != nil {
		s.logger.Warn(ctx, "failed to read card data-quality stats",
			observability.Err(statsErr))
	}
	s.logger.Info(ctx, "DH analytics refresh complete",
		observability.Int("characters_upserted", len(characters)),
		observability.Int("cards_seeded", len(cardIDs)),
		observability.Int("analytics_not_computed", notComputed),
		observability.Int("data_quality_proxy", stats.ProxyCount),
		observability.Int("data_quality_full", stats.FullCount),
		observability.Int("data_quality_null", stats.NullQualityCount),
		observability.Int("data_quality_total_rows", stats.TotalRows),
		observability.Int("character_api_calls", charCalls),
		observability.Int("card_api_calls", cardCalls),
		observability.Duration("duration", time.Since(start)))
}
