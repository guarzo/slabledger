package scheduler

import (
	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
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

	// AI call tracking (used by various AI-related schedulers if present).
	AICallTracker ai.AICallTracker

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
	DHPushPendingLister DHPushPendingLister
	DHPushStatusUpdater DHPushStatusUpdater
	DHPushCardIDSaver   DHPushCardIDSaver
	DHPushAttemptInc    DHPushAttemptIncrementer

	// DH card tombstone repo (optional; suppresses repeated 404s).
	DHTombstoneRepo    pricing.DHCardTombstoneRepo
	DHPushConfigLoader DHPushConfigLoader
	DHPushHoldSetter   DHPushHoldSetter
	DHPushRelister     DHPushRelister

	// Scoring gap cleanup dependencies (optional)
	GapStore scoring.GapStore

	// Card Ladder dependencies (optional)
	CardLadderClient           *cardladder.Client
	CardLadderStore            *postgres.CardLadderStore
	CardLadderPurchaseLister   CardLadderPurchaseLister
	CardLadderValueUpdater     CardLadderValueUpdater
	CardLadderGemRateUpdater   CardLadderGemRateUpdater
	CardLadderSyncUpdater      CardLadderSyncUpdater
	CardLadderSalesStore       *postgres.CLSalesStore
	CardLadderCompRefreshStore *postgres.CompRefreshStore

	// SchedulerStatsStore persists per-scheduler lastRun stats so the admin UI
	// survives server restarts. Optional — when nil, stats remain in-memory.
	SchedulerStatsStore *postgres.SchedulerStatsStore

	// PSA sync dependencies (optional)
	PSARowProvider    RowProvider
	PSATokenRefresher TokenRefresher
	PSAImporter       PSAImporter

	// Cert enrichment dependencies (optional).
	// If CertEnrichJobPrebuilt is set, it is used directly (skipping creation from CertLookup/PurchaseRepo).
	// This allows the job to be created before NewService for early injection via WithCertEnrichEnqueuer.
	CertEnrichJobPrebuilt *CertEnrichJob
	CertLookup            domainCampaigns.CertLookup
	PurchaseRepo          domainCampaigns.PurchaseRepository
}

// BuildResult holds the scheduler group and optional auxiliary references.
type BuildResult struct {
	Group             *Group
	CardLadderRefresh *CardLadderRefreshScheduler // nil if Card Ladder is not configured
	PSASync           *PSASyncScheduler           // nil if PSA sync is not configured
	CertEnrichJob     *CertEnrichJob              // nil if cert lookup is not configured
	DHOrdersPoll      *DHOrdersPollScheduler      // nil if DH orders poll is not configured
	DHReconcile       *DHReconcileScheduler       // nil if DH reconciler is not configured
}

// BuildGroup constructs a scheduler Group from centralized configuration and dependencies.
//
// Each scheduler is built by its own buildXScheduler helper in builder_schedulers.go,
// which returns nil when that scheduler's prerequisites are not met. The helpers return
// concrete pointer types rather than the Scheduler interface so the nil checks below
// test the pointer itself: a nil *T assigned to a Scheduler interface would compare
// non-nil and add a broken entry to the group.
func BuildGroup(cfg *config.Config, deps BuildDeps) BuildResult {
	var schedulers []Scheduler

	// Price refresh is the one unconditional scheduler; every other is gated.
	schedulers = append(schedulers, buildPriceRefreshScheduler(cfg, deps))

	if s := buildSessionCleanupScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildAccessLogCleanupScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildGapCleanupScheduler(deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildInventoryRefreshScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildSnapshotEnrichScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}

	clRefresh := buildCardLadderRefreshScheduler(cfg, deps)
	if clRefresh != nil {
		schedulers = append(schedulers, clRefresh)
	}

	if s := buildDHIntelligenceRefreshScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildDHAnalyticsRefreshScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildCardTrajectoryRefreshScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildDHSuggestionsScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}

	dhOrdersPoll := buildDHOrdersPollScheduler(cfg, deps)
	if dhOrdersPoll != nil {
		schedulers = append(schedulers, dhOrdersPoll)
	}

	if s := buildDHInventoryPollScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildDHSoldReconcilerScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}

	dhReconcile := buildDHReconcileScheduler(cfg, deps)
	if dhReconcile != nil {
		schedulers = append(schedulers, dhReconcile)
	}

	if s := buildDHPriceSyncScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}
	if s := buildDHPushScheduler(cfg, deps); s != nil {
		schedulers = append(schedulers, s)
	}

	psaSync := buildPSASyncScheduler(cfg, deps)
	if psaSync != nil {
		schedulers = append(schedulers, psaSync)
	}

	certEnrichJob := buildCertEnrichJob(deps)
	if certEnrichJob != nil {
		schedulers = append(schedulers, certEnrichJob)
	}

	return BuildResult{
		Group:             NewGroup(schedulers...),
		CardLadderRefresh: clRefresh,
		PSASync:           psaSync,
		CertEnrichJob:     certEnrichJob,
		DHOrdersPoll:      dhOrdersPoll,
		DHReconcile:       dhReconcile,
	}
}
