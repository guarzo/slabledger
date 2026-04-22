package scheduler

import (
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	domainCampaigns "github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/scoring"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// BuildDeps holds the dependencies needed to build the scheduler group.
type BuildDeps struct {
	APITracker        pricing.APITracker
	HealthChecker     pricing.HealthChecker
	AccessTracker     pricing.AccessTracker
	RefreshCandidates pricing.RefreshCandidateProvider
	PriceProvider     pricing.PriceProvider
	AuthService       auth.Service // may be nil if auth is not configured
	Logger            observability.Logger

	// Sync state (shared by DH schedulers)
	SyncStateStore SyncStateStore

	// Inventory snapshot refresh dependencies (optional)
	InventoryLister   InventoryLister
	SnapshotRefresher SnapshotRefresher

	// Snapshot enrichment dependencies (optional)
	SnapshotEnrichService SnapshotEnrichService

	// Advisor refresh dependencies (optional)
	AdvisorCollector AdvisorCollector
	AdvisorCache     advisor.CacheStore
	AICallTracker    ai.AICallTracker

	// DH dependencies (optional)
	DHClient                 *dh.Client
	DHIntelligenceRepo       intelligence.Repository
	DHTrajectoryRepo         intelligence.TrajectoryRepository
	DHCompCacheStore         *postgres.DHCompCacheStore
	DHSuggestionsRepo        intelligence.SuggestionsRepository
	DHDemandRepo             demand.Repository      // niche-opportunity cache (T1/T3)
	DHUnsoldCardLister       UnsoldDHCardLister     // seeds analytics with our inventory
	DHIntelligenceSeedLister IntelligenceSeedLister // seeds market_intelligence with our inventory

	// DH inventory reconciler (optional; enables the daily DH reconcile scheduler).
	DHReconciler dhlisting.Reconciler

	// DH price-sync service (optional; enables the DH price-sync reconciliation scheduler).
	DHPriceSyncService dhpricing.Service

	// DH event recorder (shared across DH schedulers)
	EventRecorder dhevents.Recorder

	// DH v2 dependencies (optional)
	DHOrdersClient        DHOrdersClient
	DHInventoryListClient DHInventoryListClient
	DHFieldsUpdater       DHFieldsUpdater
	PurchaseByCertLookup  PurchaseByCertLookup
	CampaignService       domainCampaigns.Service

	// DH push dependencies (optional)
	DHPushPendingLister   DHPushPendingLister
	DHPushStatusUpdater   DHPushStatusUpdater
	DHPushCardIDSaver     DHPushCardIDSaver
	DHPushCandidatesSaver DHPushCandidatesSaver
	DHPushConfigLoader    DHPushConfigLoader
	DHPushHoldSetter      DHPushHoldSetter
	DHPushAttemptsTracker DHPushAttemptsTracker

	// Scoring gap cleanup dependencies (optional)
	GapStore scoring.GapStore

	// Card Ladder dependencies (optional)
	CardLadderClient         *cardladder.Client
	CardLadderStore          *postgres.CardLadderStore
	CardLadderPurchaseLister CardLadderPurchaseLister
	CardLadderValueUpdater   CardLadderValueUpdater
	CardLadderGemRateUpdater CardLadderGemRateUpdater
	CardLadderSyncUpdater    CardLadderSyncUpdater
	CardLadderSalesStore     *postgres.CLSalesStore

	// SchedulerStatsStore persists per-scheduler lastRun stats so the admin UI
	// survives server restarts. Optional — when nil, stats remain in-memory.
	SchedulerStatsStore *postgres.SchedulerStatsStore

	// Market Movers dependencies (optional)
	MMClient         *marketmovers.Client
	MMStore          *postgres.MarketMoversStore
	MMSalesStore     *postgres.MMSalesStore
	MMPurchaseLister MMPurchaseLister
	MMValueUpdater   MMValueUpdater

	// PSA sync dependencies (optional)
	PSASheetFetcher  SheetFetcher
	PSAImporter      PSAImporter
	PSASpreadsheetID string
	PSATabName       string

	// Cert enrichment dependencies (optional).
	// If CertEnrichJobPrebuilt is set, it is used directly (skipping creation from CertLookup/PurchaseRepo).
	// This allows the job to be created before NewService for early injection via WithCertEnrichEnqueuer.
	CertEnrichJobPrebuilt *CertEnrichJob
	CertLookup            domainCampaigns.CertLookup
	PurchaseRepo          domainCampaigns.PurchaseRepository

	// Crack cache refresh dependencies (optional).
	// CrackCacheService is the inventory service used by CrackCacheRefreshJob to call RefreshCrackCandidates.
	CrackCacheService domainCampaigns.Service
}

// BuildResult holds the scheduler group and optional auxiliary references.
type BuildResult struct {
	Group             *Group
	CardLadderRefresh *CardLadderRefreshScheduler   // nil if Card Ladder is not configured
	MMRefresh         *MarketMoversRefreshScheduler // nil if Market Movers is not configured
	PSASync           *PSASyncScheduler             // nil if PSA sync is not configured
	CertEnrichJob     *CertEnrichJob                // nil if cert lookup is not configured
	CrackCacheJob     *CrackCacheRefreshJob         // nil if inventory service is not configured
	DHOrdersPoll      *DHOrdersPollScheduler        // nil if DH orders poll is not configured
	DHReconcile       *DHReconcileScheduler         // nil if DH reconciler is not configured
}

// BuildGroup constructs a scheduler Group from centralized configuration and dependencies.
func BuildGroup(cfg *config.Config, deps BuildDeps) BuildResult {
	var schedulers []Scheduler
	var clRefresh *CardLadderRefreshScheduler
	var mmRefresh *MarketMoversRefreshScheduler
	var psaSync *PSASyncScheduler
	var certEnrichJob *CertEnrichJob
	var crackCacheJob *CrackCacheRefreshJob
	var dhReconcile *DHReconcileScheduler

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
		deps.RefreshCandidates, deps.APITracker, deps.HealthChecker, deps.PriceProvider,
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

	// Advisor refresh scheduler (if advisor service and cache store are provided)
	if deps.AdvisorCollector != nil && deps.AdvisorCache != nil {
		schedulers = append(schedulers, NewAdvisorRefreshScheduler(
			deps.AdvisorCollector, deps.AdvisorCache,
			deps.AICallTracker,
			deps.Logger, cfg.AdvisorRefresh,
		))
	}

	// Card Ladder value refresh scheduler.
	// Created whenever the store and purchase interfaces are available, even if
	// no client exists yet at startup. SetClient is called by the handler when
	// credentials are saved for the first time, activating the scheduler without
	// requiring a server restart.
	if deps.CardLadderStore != nil && deps.CardLadderPurchaseLister != nil && deps.CardLadderValueUpdater != nil {
		var clOpts []CardLadderRefreshOption
		if deps.DHPushStatusUpdater != nil {
			clOpts = append(clOpts, WithCLDHPushUpdater(deps.DHPushStatusUpdater))
		}
		if deps.CardLadderSyncUpdater != nil {
			clOpts = append(clOpts, WithCLSyncUpdater(deps.CardLadderSyncUpdater))
		}
		if deps.EventRecorder != nil {
			clOpts = append(clOpts, WithCLEventRecorder(deps.EventRecorder))
		}
		if deps.SchedulerStatsStore != nil {
			clOpts = append(clOpts, WithCLStatsStore(deps.SchedulerStatsStore))
		}
		clRefresh = NewCardLadderRefreshScheduler(
			deps.CardLadderClient, deps.CardLadderStore,
			deps.CardLadderPurchaseLister, deps.CardLadderValueUpdater,
			deps.CardLadderGemRateUpdater,
			deps.CardLadderSalesStore,
			deps.Logger, cfg.CardLadder,
			clOpts...,
		)
		schedulers = append(schedulers, clRefresh)
	}

	// DH intelligence refresh scheduler (if client + repo are provided)
	if deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() && deps.DHIntelligenceRepo != nil {
		dhIntelConfig := DHIntelligenceRefreshConfig{
			Enabled:   cfg.DH.Enabled,
			Interval:  1 * time.Hour,
			CacheTTL:  time.Duration(cfg.DH.CacheTTLHours) * time.Hour,
			MaxPerRun: 50,
		}
		var intelOpts []IntelligenceRefreshOption
		if deps.DHIntelligenceSeedLister != nil {
			intelOpts = append(intelOpts, WithIntelligenceSeedLister(deps.DHIntelligenceSeedLister))
		}
		schedulers = append(schedulers, NewDHIntelligenceRefreshScheduler(
			deps.DHClient, deps.DHIntelligenceRepo, deps.Logger, dhIntelConfig, intelOpts...,
		))
	}

	// DH demand analytics refresh scheduler (daily; niche-opportunity cache).
	// Feature-flagged via cfg.DHAnalyticsRefresh.Enabled — off by default until
	// the DH impression pipeline is healthy (launch gate).
	if deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() && deps.DHDemandRepo != nil {
		schedulers = append(schedulers, NewDHAnalyticsRefreshScheduler(
			deps.DHClient,
			deps.DHDemandRepo,
			deps.DHUnsoldCardLister,
			deps.Logger,
			cfg.DHAnalyticsRefresh,
		))
	}

	// Card trajectory refresh scheduler (weekly). Uses DH's graded-sales-analytics
	// `recent_sales` array. DH may return that array empty even when
	// `total_sales > 0`; the scheduler skips cards with no `recent_sales` and
	// runs harmlessly until DH populates the field, at which point trajectory
	// buckets start landing on their own.
	if deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() && deps.DHTrajectoryRepo != nil && deps.DHIntelligenceSeedLister != nil {
		schedulers = append(schedulers, NewCardTrajectoryRefreshScheduler(
			deps.DHClient,
			deps.DHTrajectoryRepo,
			deps.DHIntelligenceSeedLister,
			deps.Logger,
			CardTrajectoryRefreshConfig{
				Enabled:  cfg.DH.Enabled,
				Interval: 7 * 24 * time.Hour,
			},
			deps.DHCompCacheStore,
		))
	}

	// DH suggestions scheduler (if client + repo are provided)
	if deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() && deps.DHSuggestionsRepo != nil {
		dhSuggestConfig := DHSuggestionsConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: 6 * time.Hour,
		}
		schedulers = append(schedulers, NewDHSuggestionsScheduler(
			deps.DHClient, deps.DHSuggestionsRepo, deps.Logger, dhSuggestConfig,
		))
	}

	// DH v2: Orders poll scheduler
	var dhOrdersPoll *DHOrdersPollScheduler
	if deps.DHOrdersClient != nil && deps.SyncStateStore != nil && deps.CampaignService != nil {
		ordersPollCfg := DHOrdersPollConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: cfg.DH.OrdersPollInterval,
		}
		dhOrdersPoll = NewDHOrdersPollScheduler(
			deps.DHOrdersClient,
			deps.SyncStateStore,
			deps.CampaignService,
			deps.EventRecorder,
			deps.Logger,
			ordersPollCfg,
		)
		schedulers = append(schedulers, dhOrdersPoll)
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
			deps.EventRecorder,
			deps.Logger,
			inventoryPollCfg,
		))
	}

	// DH inventory reconciliation scheduler (hourly drift scan).
	if deps.DHReconciler != nil {
		reconcileCfg := config.DHReconcileConfig{
			Enabled:  cfg.DHReconcile.Enabled,
			Interval: cfg.DHReconcile.Interval,
		}
		dhReconcile = NewDHReconcileScheduler(
			deps.DHReconciler,
			deps.Logger,
			reconcileCfg,
		)
		schedulers = append(schedulers, dhReconcile)
	}

	// DH price-sync scheduler: reconciles reviewed price vs DH listing price.
	if deps.DHPriceSyncService != nil {
		schedulers = append(schedulers, NewDHPriceSyncScheduler(
			deps.DHPriceSyncService,
			deps.Logger,
			DHPriceSyncConfig{
				Enabled:  cfg.DHPriceSync.Enabled,
				Interval: cfg.DHPriceSync.Interval,
			},
		))
	}

	// DH v2: Push scheduler — matches pending purchases and pushes to DH inventory
	if deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() &&
		deps.DHPushPendingLister != nil && deps.DHPushStatusUpdater != nil &&
		deps.DHPushCardIDSaver != nil && deps.DHFieldsUpdater != nil {
		pushCfg := DHPushConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: cfg.DH.PushInterval,
		}
		var pushOpts []DHPushOption
		if deps.DHPushCandidatesSaver != nil {
			pushOpts = append(pushOpts, WithDHPushCandidatesSaver(deps.DHPushCandidatesSaver))
		}
		if deps.DHPushConfigLoader != nil {
			pushOpts = append(pushOpts, WithDHPushConfigLoader(deps.DHPushConfigLoader))
		}
		if deps.DHPushHoldSetter != nil {
			pushOpts = append(pushOpts, WithDHPushHoldSetter(deps.DHPushHoldSetter))
		}
		if deps.DHPushAttemptsTracker != nil {
			pushOpts = append(pushOpts, WithDHPushAttemptsTracker(deps.DHPushAttemptsTracker))
		}
		if deps.EventRecorder != nil {
			pushOpts = append(pushOpts, WithDHPushEventRecorder(deps.EventRecorder))
		}
		// DH client also implements PSAImport for the off-catalog fallback.
		pushOpts = append(pushOpts, WithDHPushPSAImporter(deps.DHClient))
		schedulers = append(schedulers, NewDHPushScheduler(
			deps.DHPushPendingLister,
			deps.DHPushStatusUpdater,
			deps.DHClient,
			deps.DHClient,
			deps.DHFieldsUpdater,
			deps.DHPushCardIDSaver,
			deps.Logger,
			pushCfg,
			pushOpts...,
		))
	}

	// PSA Google Sheets sync scheduler (if fetcher and importer are provided)
	if deps.PSASheetFetcher != nil && deps.PSAImporter != nil && deps.PSASpreadsheetID != "" {
		psaSync = NewPSASyncScheduler(
			deps.PSASheetFetcher, deps.PSAImporter,
			deps.Logger, cfg.PSASync,
			deps.PSASpreadsheetID, deps.PSATabName,
		)
		schedulers = append(schedulers, psaSync)
	}

	// Cert enrichment scheduler (if cert lookup is provided).
	// Prefer the pre-built job (created before service construction) so the same instance
	// is injected into inventory.service via WithCertEnrichEnqueuer.
	if deps.CertEnrichJobPrebuilt != nil {
		certEnrichJob = deps.CertEnrichJobPrebuilt
		schedulers = append(schedulers, certEnrichJob)
	} else if deps.CertLookup != nil && deps.PurchaseRepo != nil {
		certEnrichJob = NewCertEnrichJob(
			deps.CertLookup, deps.PurchaseRepo,
			deps.Logger,
		)
		schedulers = append(schedulers, certEnrichJob)
	}

	// Crack cache refresh scheduler (if inventory service is provided).
	if deps.CrackCacheService != nil {
		crackCacheJob = NewCrackCacheRefreshJob(deps.CrackCacheService, deps.Logger)
		schedulers = append(schedulers, crackCacheJob)
	}

	// Market Movers value refresh scheduler.
	// Created whenever the store and purchase interfaces are available, even if
	// no client exists yet at startup. SetClient is called by the handler when
	// credentials are saved for the first time, activating the scheduler without
	// requiring a server restart.
	if deps.MMStore != nil && deps.MMPurchaseLister != nil && deps.MMValueUpdater != nil {
		mmRefresh = NewMarketMoversRefreshScheduler(
			deps.MMClient, deps.MMStore,
			deps.MMPurchaseLister, deps.MMValueUpdater,
			deps.Logger, cfg.MarketMovers,
		)
		if deps.MMSalesStore != nil {
			mmRefresh.SetSalesStore(deps.MMSalesStore)
		}
		schedulers = append(schedulers, mmRefresh)
	}

	return BuildResult{
		Group:             NewGroup(schedulers...),
		CardLadderRefresh: clRefresh,
		MMRefresh:         mmRefresh,
		PSASync:           psaSync,
		CertEnrichJob:     certEnrichJob,
		CrackCacheJob:     crackCacheJob,
		DHOrdersPoll:      dhOrdersPoll,
		DHReconcile:       dhReconcile,
	}
}
