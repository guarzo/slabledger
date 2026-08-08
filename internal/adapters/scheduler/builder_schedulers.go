package scheduler

import (
	"time"

	"github.com/guarzo/slabledger/internal/platform/config"
)

// Per-scheduler constructors for BuildGroup. Each returns nil when its
// prerequisites are not met, so BuildGroup stays a flat sequence of calls.
//
// Every helper returns a concrete pointer type rather than the Scheduler
// interface: BuildGroup nil-checks the result, and a nil *T assigned to an
// interface first would compare non-nil.

// buildPriceRefreshScheduler builds the price refresh scheduler. It has no
// optional dependencies and is never nil.
func buildPriceRefreshScheduler(cfg *config.Config, deps BuildDeps) *PriceRefreshScheduler {
	schedulerConfig := Config{
		RefreshInterval:    cfg.PriceRefresh.RefreshInterval,
		BatchSize:          cfg.PriceRefresh.BatchSize,
		BatchDelay:         cfg.PriceRefresh.BatchDelay,
		MaxBurstCalls:      cfg.PriceRefresh.MaxBurstCalls,
		MaxCallsPerHour:    cfg.PriceRefresh.MaxCallsPerHour,
		BurstPauseDuration: cfg.PriceRefresh.BurstPauseDuration,
		Enabled:            cfg.PriceRefresh.Enabled,
	}
	return NewPriceRefreshScheduler(
		deps.RefreshCandidates, deps.APITracker, deps.HealthChecker, deps.PriceProvider,
		deps.Logger, schedulerConfig,
	)
}

// buildSessionCleanupScheduler builds the session cleanup scheduler (if auth is enabled).
func buildSessionCleanupScheduler(cfg *config.Config, deps BuildDeps) *SessionCleanupScheduler {
	if deps.AuthService == nil {
		return nil
	}
	cleanupConfig := SessionCleanupConfig{
		Enabled:  cfg.SessionCleanup.Enabled,
		Interval: cfg.SessionCleanup.Interval,
	}
	return NewSessionCleanupScheduler(deps.AuthService, deps.Logger, cleanupConfig)
}

// buildAccessLogCleanupScheduler builds the access log cleanup scheduler (if enabled).
func buildAccessLogCleanupScheduler(cfg *config.Config, deps BuildDeps) *AccessLogCleanupScheduler {
	if !cfg.Maintenance.AccessLogCleanupEnabled || cfg.Maintenance.AccessLogRetentionDays <= 0 {
		return nil
	}
	accessLogConfig := AccessLogCleanupConfig{
		Enabled:       cfg.Maintenance.AccessLogCleanupEnabled,
		Interval:      cfg.Maintenance.AccessLogCleanupInterval,
		RetentionDays: cfg.Maintenance.AccessLogRetentionDays,
	}
	return NewAccessLogCleanupScheduler(deps.AccessTracker, deps.Logger, accessLogConfig)
}

// buildDHEventCleanupScheduler builds the dh_state_events cleanup scheduler
// (if enabled and a pruner is provided).
func buildDHEventCleanupScheduler(cfg *config.Config, deps BuildDeps) *DHEventCleanupScheduler {
	if deps.EventPruner == nil {
		return nil
	}
	if !cfg.Maintenance.DHEventCleanupEnabled || cfg.Maintenance.DHEventRetentionDays <= 0 {
		return nil
	}
	dhEventConfig := DHEventCleanupConfig{
		Enabled:       cfg.Maintenance.DHEventCleanupEnabled,
		Interval:      cfg.Maintenance.DHEventCleanupInterval,
		RetentionDays: cfg.Maintenance.DHEventRetentionDays,
	}
	return NewDHEventCleanupScheduler(deps.EventPruner, deps.Logger, dhEventConfig)
}

// buildGapCleanupScheduler builds the scoring data gap cleanup scheduler
// (if a gap store is provided).
func buildGapCleanupScheduler(deps BuildDeps) *GapCleanupScheduler {
	if deps.GapStore == nil {
		return nil
	}
	return NewGapCleanupScheduler(deps.GapStore, deps.Logger)
}

// buildInventoryRefreshScheduler builds the inventory snapshot refresh scheduler
// (if its dependencies are provided).
func buildInventoryRefreshScheduler(cfg *config.Config, deps BuildDeps) *InventoryRefreshScheduler {
	if deps.InventoryLister == nil || deps.SnapshotRefresher == nil {
		return nil
	}
	invConfig := config.InventoryRefreshConfig{
		Enabled:        cfg.InventoryRefresh.Enabled,
		Interval:       cfg.InventoryRefresh.Interval,
		StaleThreshold: cfg.InventoryRefresh.StaleThreshold,
		BatchSize:      cfg.InventoryRefresh.BatchSize,
		BatchDelay:     cfg.InventoryRefresh.BatchDelay,
	}
	return NewInventoryRefreshScheduler(
		deps.InventoryLister, deps.SnapshotRefresher,
		deps.Logger, invConfig,
	)
}

// buildSnapshotEnrichScheduler builds the snapshot enrichment scheduler, which
// processes pending snapshots from async imports.
func buildSnapshotEnrichScheduler(cfg *config.Config, deps BuildDeps) *SnapshotEnrichScheduler {
	if deps.SnapshotEnrichService == nil {
		return nil
	}
	return NewSnapshotEnrichScheduler(deps.SnapshotEnrichService, deps.Logger, cfg.SnapshotEnrich)
}

// buildCardLadderRefreshScheduler builds the Card Ladder value refresh scheduler.
//
// Created whenever the store and purchase interfaces are available, even if no
// client exists yet at startup. SetClient is called by the handler when credentials
// are saved for the first time, activating the scheduler without requiring a
// server restart.
func buildCardLadderRefreshScheduler(cfg *config.Config, deps BuildDeps) *CardLadderRefreshScheduler {
	if deps.CardLadderStore == nil || deps.CardLadderPurchaseLister == nil || deps.CardLadderValueUpdater == nil {
		return nil
	}
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
	if deps.CardLadderCompRefreshStore != nil {
		clOpts = append(clOpts, WithCLCompRefreshStore(deps.CardLadderCompRefreshStore))
	}
	return NewCardLadderRefreshScheduler(
		deps.CardLadderClient, deps.CardLadderStore,
		deps.CardLadderPurchaseLister, deps.CardLadderValueUpdater,
		deps.CardLadderGemRateUpdater,
		deps.CardLadderSalesStore,
		deps.Logger, cfg.CardLadder,
		clOpts...,
	)
}

// buildDHIntelligenceRefreshScheduler builds the DH intelligence refresh scheduler
// (if client + repo are provided).
func buildDHIntelligenceRefreshScheduler(cfg *config.Config, deps BuildDeps) *DHIntelligenceRefreshScheduler {
	if deps.DHClient == nil || !deps.DHClient.EnterpriseAvailable() || deps.DHIntelligenceRepo == nil {
		return nil
	}
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
	if deps.DHTombstoneRepo != nil {
		intelOpts = append(intelOpts, WithIntelligenceTombstoneRepo(deps.DHTombstoneRepo))
	}
	return NewDHIntelligenceRefreshScheduler(
		deps.DHClient, deps.DHIntelligenceRepo, deps.Logger, dhIntelConfig, intelOpts...,
	)
}

// buildDHAnalyticsRefreshScheduler builds the DH demand analytics refresh scheduler
// (daily; niche-opportunity cache).
func buildDHAnalyticsRefreshScheduler(cfg *config.Config, deps BuildDeps) *DHAnalyticsRefreshScheduler {
	if deps.DHClient == nil || !deps.DHClient.EnterpriseAvailable() || deps.DHDemandRepo == nil {
		return nil
	}
	return NewDHAnalyticsRefreshScheduler(
		deps.DHClient,
		deps.DHDemandRepo,
		deps.DHUnsoldCardLister,
		deps.Logger,
		cfg.DHAnalyticsRefresh,
	)
}

// buildCardTrajectoryRefreshScheduler builds the card trajectory refresh scheduler
// (weekly). It uses DH's graded-sales-analytics recent_sales array to build weekly
// price trajectory buckets.
func buildCardTrajectoryRefreshScheduler(cfg *config.Config, deps BuildDeps) *CardTrajectoryRefreshScheduler {
	if deps.DHClient == nil || !deps.DHClient.EnterpriseAvailable() ||
		deps.DHTrajectoryRepo == nil || deps.DHIntelligenceSeedLister == nil {
		return nil
	}
	return NewCardTrajectoryRefreshScheduler(
		deps.DHClient,
		deps.DHTrajectoryRepo,
		deps.DHIntelligenceSeedLister,
		deps.Logger,
		CardTrajectoryRefreshConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: 7 * 24 * time.Hour,
		},
		deps.DHCompCacheStore,
	)
}

// buildDHSuggestionsScheduler builds the DH suggestions scheduler
// (if client + repo are provided).
func buildDHSuggestionsScheduler(cfg *config.Config, deps BuildDeps) *DHSuggestionsScheduler {
	if deps.DHClient == nil || !deps.DHClient.EnterpriseAvailable() || deps.DHSuggestionsRepo == nil {
		return nil
	}
	dhSuggestConfig := DHSuggestionsConfig{
		Enabled:  cfg.DH.Enabled,
		Interval: 6 * time.Hour,
	}
	return NewDHSuggestionsScheduler(
		deps.DHClient, deps.DHSuggestionsRepo, deps.Logger, dhSuggestConfig,
	)
}

// buildDHOrdersPollScheduler builds the DH v2 orders poll scheduler.
func buildDHOrdersPollScheduler(cfg *config.Config, deps BuildDeps) *DHOrdersPollScheduler {
	if deps.DHOrdersClient == nil || deps.SyncStateStore == nil || deps.OrdersImporter == nil {
		return nil
	}
	ordersPollCfg := DHOrdersPollConfig{
		Enabled:  cfg.DH.Enabled,
		Interval: cfg.DH.OrdersPollInterval,
	}
	return NewDHOrdersPollScheduler(
		deps.DHOrdersClient,
		deps.SyncStateStore,
		deps.OrdersImporter,
		deps.EventRecorder,
		deps.Logger,
		ordersPollCfg,
	)
}

// buildDHInventoryPollScheduler builds the DH v2 inventory status poll scheduler.
func buildDHInventoryPollScheduler(cfg *config.Config, deps BuildDeps) *DHInventoryPollScheduler {
	if deps.DHInventoryListClient == nil || deps.SyncStateStore == nil ||
		deps.DHFieldsUpdater == nil || deps.PurchaseByCertLookup == nil {
		return nil
	}
	inventoryPollCfg := DHInventoryPollConfig{
		Enabled:  cfg.DH.Enabled,
		Interval: cfg.DH.InventoryPollInterval,
	}
	return NewDHInventoryPollScheduler(
		deps.DHInventoryListClient,
		deps.SyncStateStore,
		deps.DHFieldsUpdater,
		deps.PurchaseByCertLookup,
		deps.EventRecorder,
		deps.Logger,
		inventoryPollCfg,
	)
}

// buildDHSoldReconcilerScheduler builds the DH sold-status reconciler, a safety net
// for the best-effort dh_status update in CreateSale.
func buildDHSoldReconcilerScheduler(cfg *config.Config, deps BuildDeps) *DHSoldReconcilerScheduler {
	if deps.PurchaseRepo == nil {
		return nil
	}
	soldReconcilerCfg := DHSoldReconcilerConfig{
		Enabled:  cfg.DHSoldReconciler.Enabled,
		Interval: cfg.DHSoldReconciler.Interval,
	}
	return NewDHSoldReconcilerScheduler(
		deps.PurchaseRepo,
		deps.PurchaseRepo,
		deps.Logger,
		soldReconcilerCfg,
	)
}

// buildDHReconcileScheduler builds the DH inventory reconciliation scheduler
// (hourly drift scan).
func buildDHReconcileScheduler(cfg *config.Config, deps BuildDeps) *DHReconcileScheduler {
	if deps.DHReconciler == nil {
		return nil
	}
	reconcileCfg := config.DHReconcileConfig{
		Enabled:  cfg.DHReconcile.Enabled,
		Interval: cfg.DHReconcile.Interval,
	}
	return NewDHReconcileScheduler(
		deps.DHReconciler,
		deps.Logger,
		reconcileCfg,
	)
}

// buildDHPriceSyncScheduler builds the DH price-sync scheduler, which reconciles
// reviewed price vs DH listing price.
func buildDHPriceSyncScheduler(cfg *config.Config, deps BuildDeps) *DHPriceSyncScheduler {
	if deps.DHPriceSyncService == nil {
		return nil
	}
	return NewDHPriceSyncScheduler(
		deps.DHPriceSyncService,
		deps.Logger,
		DHPriceSyncConfig{
			Enabled:  cfg.DHPriceSync.Enabled,
			Interval: cfg.DHPriceSync.Interval,
		},
	)
}

// buildDHPushScheduler builds the DH v2 push scheduler, which matches pending
// purchases and pushes them to DH inventory.
func buildDHPushScheduler(cfg *config.Config, deps BuildDeps) *DHPushScheduler {
	if deps.DHClient == nil || !deps.DHClient.EnterpriseAvailable() ||
		deps.DHPushPendingLister == nil || deps.DHPushStatusUpdater == nil ||
		deps.DHPushCardIDSaver == nil || deps.DHFieldsUpdater == nil {
		return nil
	}
	pushCfg := DHPushConfig{
		Enabled:  cfg.DH.Enabled,
		Interval: cfg.DH.PushInterval,
	}
	var pushOpts []DHPushOption
	if deps.DHPushConfigLoader != nil {
		pushOpts = append(pushOpts, WithDHPushConfigLoader(deps.DHPushConfigLoader))
	}
	if deps.DHPushHoldSetter != nil {
		pushOpts = append(pushOpts, WithDHPushHoldSetter(deps.DHPushHoldSetter))
	}
	if deps.EventRecorder != nil {
		pushOpts = append(pushOpts, WithDHPushEventRecorder(deps.EventRecorder))
	}
	if deps.DHPushRelister != nil {
		pushOpts = append(pushOpts, WithDHPushRelister(deps.DHPushRelister))
	}
	if deps.DHPushAttemptInc != nil {
		pushOpts = append(pushOpts, WithDHPushAttemptIncrementer(deps.DHPushAttemptInc))
	}
	return NewDHPushScheduler(
		deps.DHPushPendingLister,
		deps.DHPushStatusUpdater,
		deps.DHClient, // implements DHPushPSAImporter via PSAImport()
		deps.DHFieldsUpdater,
		deps.DHPushCardIDSaver,
		deps.Logger,
		pushCfg,
		pushOpts...,
	)
}

// buildPSASyncScheduler builds the PSA portal sync scheduler (if provider and
// importer are provided).
func buildPSASyncScheduler(cfg *config.Config, deps BuildDeps) *PSASyncScheduler {
	if deps.PSARowProvider == nil || deps.PSAImporter == nil {
		return nil
	}
	return NewPSASyncScheduler(
		deps.PSARowProvider, deps.PSAImporter,
		deps.Logger, cfg.PSASync,
		WithPSATokenRefresher(deps.PSATokenRefresher),
	)
}

// buildCertEnrichJob builds the cert enrichment scheduler (if cert lookup is provided).
//
// The pre-built job is preferred (created before service construction) so the same
// instance is injected into inventory.service via WithCertEnrichEnqueuer.
func buildCertEnrichJob(deps BuildDeps) *CertEnrichJob {
	if deps.CertEnrichJobPrebuilt != nil {
		return deps.CertEnrichJobPrebuilt
	}
	if deps.CertLookup == nil || deps.PurchaseRepo == nil {
		return nil
	}
	return NewCertEnrichJob(
		deps.CertLookup, deps.PurchaseRepo,
		deps.Logger,
	)
}
