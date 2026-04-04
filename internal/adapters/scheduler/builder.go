package scheduler

import (
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/auth"
	domainCampaigns "github.com/guarzo/slabledger/internal/domain/campaigns"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/scoring"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// CardHedgerClient combines the interfaces used by both CardHedger schedulers.
// *cardhedger.Client satisfies this interface.
type CardHedgerClient interface {
	CardHedgerBatchClient
	CardHedgerRefreshClient
}

// BuildDeps holds the dependencies needed to build the scheduler group.
type BuildDeps struct {
	PriceRepo     pricing.PriceRepository
	APITracker    pricing.APITracker
	HealthChecker pricing.HealthChecker
	AccessTracker pricing.AccessTracker
	PriceProvider pricing.PriceProvider
	CardProvider  domainCards.CardProvider
	AuthService   auth.Service // may be nil if auth is not configured
	Logger        observability.Logger

	// CardHedger dependencies (all optional; schedulers skipped if nil)
	CardHedgerClient    CardHedgerClient
	SyncStateStore      SyncStateStore
	CardIDMappingLookup CardIDMappingLookup

	// CardHedger daily batch dependencies (optional)
	CardIDMappingLister     CardIDMappingLister
	CardIDMappingSaver      CardIDMappingSaver
	DiscoveryFailureTracker pricing.DiscoveryFailureTracker
	FavoritesLister         FavoritesLister
	CampaignCardLister      CampaignCardLister
	CertSweeper             CertSweeper

	// Cache warmup dependencies (optional)
	NewSetsProvider NewSetIDsProvider

	// Inventory snapshot refresh dependencies (optional)
	InventoryLister   InventoryLister
	SnapshotRefresher SnapshotRefresher

	// Snapshot enrichment dependencies (optional)
	SnapshotEnrichService SnapshotEnrichService

	// Snapshot history archival dependencies (optional)
	SnapshotHistoryLister   SnapshotHistoryLister
	SnapshotHistoryRecorder domainCampaigns.SnapshotHistoryRecorder

	// Advisor refresh dependencies (optional)
	AdvisorCollector AdvisorCollector
	AdvisorCache     advisor.CacheStore
	AICallTracker    ai.AICallTracker

	// Social content dependencies (optional)
	SocialContentDetector   SocialContentDetector
	InstagramTokenRefresher InstagramTokenRefresher

	// Metrics poll dependencies (optional)
	MetricsPostLister social.MetricsPostLister
	MetricsSaver      social.MetricsSaver
	InsightsPoller    social.InsightsPoller

	// Picks generation dependencies (optional)
	PicksGenerator PicksGenerator

	// JustTCG dependencies (optional)
	JustTCGClient JustTCGClient

	// DH dependencies (optional)
	DHClient           *dh.Client
	DHIntelligenceRepo intelligence.Repository
	DHSuggestionsRepo  intelligence.SuggestionsRepository

	// DH v2 dependencies (optional)
	DHOrdersClient        DHOrdersClient
	DHInventoryListClient DHInventoryListClient
	DHFieldsUpdater       DHFieldsUpdater
	PurchaseByCertLookup  PurchaseByCertLookup
	CampaignService       domainCampaigns.Service

	// Scoring gap cleanup dependencies (optional)
	GapStore scoring.GapStore

	// Card Ladder dependencies (optional)
	CardLadderClient         *cardladder.Client
	CardLadderStore          *sqlite.CardLadderStore
	CardLadderPurchaseLister CardLadderPurchaseLister
	CardLadderValueUpdater   CardLadderValueUpdater
	CardLadderCLRecorder     domainCampaigns.CLValueHistoryRecorder
	CardLadderSalesStore     *sqlite.CLSalesStore
}

// BuildResult holds the scheduler group and optional auxiliary references.
type BuildResult struct {
	Group             *Group
	CardDiscoverer    CardDiscoverer              // nil if CardHedger batch is not configured
	CardLadderRefresh *CardLadderRefreshScheduler // nil if Card Ladder is not configured
}

// BuildGroup constructs a scheduler Group from centralized configuration and dependencies.
func BuildGroup(cfg *config.Config, deps BuildDeps) BuildResult {
	var schedulers []Scheduler
	var cardDiscoverer CardDiscoverer
	var clRefresh *CardLadderRefreshScheduler

	// Price refresh scheduler
	schedulerConfig := Config{
		RefreshInterval:    cfg.PriceRefresh.RefreshInterval,
		BatchSize:          cfg.PriceRefresh.BatchSize,
		BatchDelay:         cfg.PriceRefresh.BatchDelay,
		MaxBurstCalls:      cfg.PriceRefresh.MaxBurstCalls,
		MaxCallsPerHour:    cfg.PriceRefresh.MaxCallsPerHour,
		BurstPauseDuration: cfg.PriceRefresh.BurstPauseDuration,
		Enabled:            cfg.PriceRefresh.Enabled,
	}
	priceScheduler := NewPriceRefreshScheduler(
		deps.PriceRepo, deps.APITracker, deps.HealthChecker, deps.PriceProvider,
		deps.Logger, schedulerConfig,
	)
	schedulers = append(schedulers, priceScheduler)

	// Session cleanup scheduler (if auth is enabled)
	if deps.AuthService != nil {
		cleanupConfig := SessionCleanupConfig{
			Enabled:  cfg.SessionCleanup.Enabled,
			Interval: cfg.SessionCleanup.Interval,
		}
		sessionCleanupScheduler := NewSessionCleanupScheduler(
			deps.AuthService, deps.Logger, cleanupConfig,
		)
		schedulers = append(schedulers, sessionCleanupScheduler)
	}

	// Access log cleanup scheduler (if enabled)
	if cfg.Maintenance.AccessLogCleanupEnabled && cfg.Maintenance.AccessLogRetentionDays > 0 {
		accessLogConfig := AccessLogCleanupConfig{
			Enabled:       cfg.Maintenance.AccessLogCleanupEnabled,
			Interval:      cfg.Maintenance.AccessLogCleanupInterval,
			RetentionDays: cfg.Maintenance.AccessLogRetentionDays,
		}
		accessLogCleanupScheduler := NewAccessLogCleanupScheduler(
			deps.AccessTracker, deps.Logger, accessLogConfig,
		)
		schedulers = append(schedulers, accessLogCleanupScheduler)
	}

	// Scoring data gap cleanup scheduler (if gap store is provided)
	if deps.GapStore != nil {
		schedulers = append(schedulers, NewGapCleanupScheduler(deps.GapStore, deps.Logger))
	}

	// Card cache warmup scheduler (if enabled)
	if cfg.CacheWarmup.Enabled {
		warmupConfig := CacheWarmupConfig{
			Enabled:        cfg.CacheWarmup.Enabled,
			Interval:       cfg.CacheWarmup.Interval,
			RateLimitDelay: cfg.CacheWarmup.RateLimitDelay,
		}
		warmupScheduler := NewCacheWarmupScheduler(
			deps.CardProvider, deps.Logger, warmupConfig, deps.NewSetsProvider,
		)
		schedulers = append(schedulers, warmupScheduler)
	}

	// Inventory snapshot refresh scheduler (if dependencies are provided)
	if deps.InventoryLister != nil && deps.SnapshotRefresher != nil {
		invConfig := config.InventoryRefreshConfig{
			Enabled:        cfg.InventoryRefresh.Enabled,
			Interval:       cfg.InventoryRefresh.Interval,
			StaleThreshold: cfg.InventoryRefresh.StaleThreshold,
			BatchSize:      cfg.InventoryRefresh.BatchSize,
			BatchDelay:     cfg.InventoryRefresh.BatchDelay,
		}
		inventoryScheduler := NewInventoryRefreshScheduler(
			deps.InventoryLister, deps.SnapshotRefresher,
			deps.Logger, invConfig,
		)
		schedulers = append(schedulers, inventoryScheduler)
	}

	// Snapshot enrichment scheduler (processes pending snapshots from async imports)
	if deps.SnapshotEnrichService != nil {
		enrichScheduler := NewSnapshotEnrichScheduler(
			deps.SnapshotEnrichService, deps.Logger, cfg.SnapshotEnrich,
		)
		schedulers = append(schedulers, enrichScheduler)
	}

	// Snapshot history archival scheduler (if dependencies are provided)
	if deps.SnapshotHistoryLister != nil && deps.SnapshotHistoryRecorder != nil {
		historyConfig := SnapshotHistoryConfig{
			Enabled:  cfg.SnapshotHistory.Enabled,
			Interval: cfg.SnapshotHistory.Interval,
		}
		historyScheduler := NewSnapshotHistoryScheduler(
			deps.SnapshotHistoryLister, deps.SnapshotHistoryRecorder,
			deps.Logger, historyConfig,
		)
		schedulers = append(schedulers, historyScheduler)
	}

	// CardHedger delta poll scheduler (if all dependencies are provided)
	if deps.CardHedgerClient != nil && deps.SyncStateStore != nil && deps.CardIDMappingLookup != nil {
		chConfig := CardHedgerRefreshConfig{
			Enabled:      cfg.CardHedger.Enabled,
			PollInterval: cfg.CardHedger.PollInterval,
		}
		var refreshOpts []RefreshOption
		if deps.APITracker != nil {
			refreshOpts = append(refreshOpts, WithRefreshAPITracker(deps.APITracker))
		}
		chScheduler := NewCardHedgerRefreshScheduler(
			deps.CardHedgerClient, deps.PriceRepo, deps.SyncStateStore,
			deps.CardIDMappingLookup, deps.Logger, chConfig, refreshOpts...,
		)
		schedulers = append(schedulers, chScheduler)
	}

	// CardHedger daily batch scheduler (if client + mapping lister are provided)
	if deps.CardHedgerClient != nil && deps.CardIDMappingLister != nil {
		batchConfig := CardHedgerBatchConfig{
			Enabled:        cfg.CardHedger.Enabled,
			RunInterval:    cfg.CardHedger.BatchInterval,
			MaxCardsPerRun: cfg.CardHedger.MaxCardsPerRun,
		}
		var batchOpts []BatchOption
		if deps.APITracker != nil {
			batchOpts = append(batchOpts, WithBatchAPITracker(deps.APITracker))
		}
		if deps.CardIDMappingSaver != nil {
			batchOpts = append(batchOpts, WithBatchMappingSaver(deps.CardIDMappingSaver))
		}
		if deps.DiscoveryFailureTracker != nil {
			batchOpts = append(batchOpts, WithDiscoveryFailureTracker(deps.DiscoveryFailureTracker))
		}
		if deps.CertSweeper != nil {
			batchOpts = append(batchOpts, WithCertSweeper(deps.CertSweeper))
		}
		batchScheduler := NewCardHedgerBatchScheduler(
			deps.CardHedgerClient, deps.PriceRepo, deps.CardIDMappingLister,
			deps.FavoritesLister, deps.CampaignCardLister,
			deps.Logger, batchConfig, batchOpts...,
		)
		schedulers = append(schedulers, batchScheduler)
		cardDiscoverer = batchScheduler
	}

	// Advisor refresh scheduler (if advisor service and cache store are provided)
	if deps.AdvisorCollector != nil && deps.AdvisorCache != nil {
		schedulers = append(schedulers, NewAdvisorRefreshScheduler(
			deps.AdvisorCollector, deps.AdvisorCache,
			deps.AICallTracker,
			deps.Logger, cfg.AdvisorRefresh,
		))
	}

	// Social content scheduler (if detector is provided)
	if deps.SocialContentDetector != nil {
		var socialOpts []SocialContentOption
		if deps.InstagramTokenRefresher != nil {
			socialOpts = append(socialOpts, WithTokenRefresher(deps.InstagramTokenRefresher))
		}
		schedulers = append(schedulers, NewSocialContentScheduler(
			deps.SocialContentDetector, deps.Logger, cfg.SocialContent, socialOpts...,
		))
	}

	// Metrics poll scheduler (if all dependencies are provided)
	if deps.MetricsPostLister != nil && deps.MetricsSaver != nil && deps.InsightsPoller != nil {
		schedulers = append(schedulers, NewMetricsPollScheduler(
			deps.MetricsPostLister, deps.MetricsSaver, deps.InsightsPoller,
			deps.Logger, cfg.MetricsPoll,
		))
	}

	// Picks refresh scheduler (if generator is provided)
	if deps.PicksGenerator != nil {
		schedulers = append(schedulers, NewPicksRefreshScheduler(
			deps.PicksGenerator, deps.Logger, cfg.PicksRefresh,
		))
	}

	// Card Ladder value refresh scheduler (if client + store are provided)
	if deps.CardLadderClient != nil && deps.CardLadderStore != nil && deps.CardLadderPurchaseLister != nil && deps.CardLadderValueUpdater != nil {
		clRefresh = NewCardLadderRefreshScheduler(
			deps.CardLadderClient, deps.CardLadderStore,
			deps.CardLadderPurchaseLister, deps.CardLadderValueUpdater,
			deps.CardLadderCLRecorder,
			deps.CardLadderSalesStore,
			deps.Logger, cfg.CardLadder,
		)
		schedulers = append(schedulers, clRefresh)
	}

	// JustTCG NM price refresh scheduler (if client is available)
	if deps.JustTCGClient != nil && deps.JustTCGClient.Available() && deps.CardIDMappingLister != nil {
		jtcgConfig := JustTCGRefreshConfig{
			Enabled:      cfg.JustTCG.Enabled,
			RunInterval:  cfg.JustTCG.RunInterval,
			DailyBudget:  cfg.JustTCG.DailyBudget,
			RateInterval: cfg.JustTCG.RateInterval,
		}
		var jtcgOpts []JustTCGRefreshOption
		if deps.APITracker != nil {
			jtcgOpts = append(jtcgOpts, WithJustTCGAPITracker(deps.APITracker))
		}
		jtcgScheduler := NewJustTCGRefreshScheduler(
			deps.JustTCGClient, deps.PriceRepo,
			deps.CardIDMappingLister, deps.CardIDMappingSaver,
			deps.CampaignCardLister,
			deps.Logger, jtcgConfig, jtcgOpts...,
		)
		schedulers = append(schedulers, jtcgScheduler)
	}

	// DH intelligence refresh scheduler (if client + repo are provided)
	if deps.DHClient != nil && deps.DHClient.Available() && deps.DHIntelligenceRepo != nil {
		dhIntelConfig := DHIntelligenceRefreshConfig{
			Enabled:   cfg.DH.Enabled,
			Interval:  1 * time.Hour,
			CacheTTL:  time.Duration(cfg.DH.CacheTTLHours) * time.Hour,
			MaxPerRun: 50,
		}
		schedulers = append(schedulers, NewDHIntelligenceRefreshScheduler(
			deps.DHClient, deps.DHIntelligenceRepo, deps.Logger, dhIntelConfig,
		))
	}

	// DH suggestions scheduler (if client + repo are provided)
	if deps.DHClient != nil && deps.DHClient.Available() && deps.DHSuggestionsRepo != nil {
		dhSuggestConfig := DHSuggestionsConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: 6 * time.Hour,
		}
		schedulers = append(schedulers, NewDHSuggestionsScheduler(
			deps.DHClient, deps.DHSuggestionsRepo, deps.Logger, dhSuggestConfig,
		))
	}

	// DH v2: Orders poll scheduler
	if deps.DHOrdersClient != nil && deps.SyncStateStore != nil && deps.CampaignService != nil {
		ordersPollCfg := DHOrdersPollConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: cfg.DH.OrdersPollInterval,
		}
		schedulers = append(schedulers, NewDHOrdersPollScheduler(
			deps.DHOrdersClient,
			deps.SyncStateStore,
			deps.CampaignService,
			deps.Logger,
			ordersPollCfg,
		))
	}

	// DH v2: Inventory status poll scheduler
	if deps.DHInventoryListClient != nil && deps.SyncStateStore != nil && deps.DHFieldsUpdater != nil && deps.PurchaseByCertLookup != nil {
		inventoryPollCfg := DHInventoryPollConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: cfg.DH.InventoryPollInterval,
		}
		schedulers = append(schedulers, NewDHInventoryPollScheduler(
			deps.DHInventoryListClient,
			deps.SyncStateStore,
			deps.DHFieldsUpdater,
			deps.PurchaseByCertLookup,
			deps.Logger,
			inventoryPollCfg,
		))
	}

	return BuildResult{
		Group:             NewGroup(schedulers...),
		CardDiscoverer:    cardDiscoverer,
		CardLadderRefresh: clRefresh,
	}
}
