package main

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhlistingadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// schedulerDeps bundles all dependencies needed by initializeSchedulers.
type schedulerDeps struct {
	Config                     *config.Config
	Logger                     observability.Logger
	DBTracker                  *postgres.DBTracker
	RefreshCandidates          pricing.RefreshCandidateProvider
	PriceProvImpl              pricing.PriceProvider
	AuthService                auth.Service
	SyncStateRepo              *postgres.SyncStateRepository
	CardIDMappingRepo          *postgres.CardIDMappingRepository
	CampaignStore              *postgres.CampaignStore
	PurchaseStore              *postgres.PurchaseStore
	DHStore                    *postgres.DHStore
	CampaignsService           inventory.Service
	CertLookup                 inventory.CertLookup
	CertEnrichJob              *scheduler.CertEnrichJob    // pre-built; nil if PSA not configured
	PricingEnrichJob           *scheduler.PricingEnrichJob // pre-built; wired into inventory service as the pricing enqueuer
	AdvisorService             advisor.Service
	AdvisorCacheRepo           *postgres.AdvisorCacheRepository
	AICallRepo                 *postgres.AICallRepository
	CardLadderClient           *cardladder.Client
	CardLadderStore            *postgres.CardLadderStore
	CardLadderSalesStore       *postgres.CLSalesStore
	CardLadderCompRefreshStore *postgres.CompRefreshStore
	SchedulerStatsStore        *postgres.SchedulerStatsStore
	MMClient                   *marketmovers.Client
	MMStore                    *postgres.MarketMoversStore
	MMSalesStore               *postgres.MMSalesStore
	DHClient                   *dh.Client
	DHEventStore               *postgres.DHEventStore
	DHIntelligenceRepo         *postgres.MarketIntelligenceRepository
	DHSuggestionsRepo          *postgres.DHSuggestionsRepository
	DHDemandRepo               *postgres.DHDemandRepository
	DHTrajectoryRepo           *postgres.CardPriceTrajectoryRepository
	DHCompCacheStore           *postgres.DHCompCacheStore
	DHPriceSyncService         dhpricing.Service
	GapStore                   *postgres.GapStore
	PSASheetFetcher            scheduler.SheetFetcher
	PSASpreadsheetID           string
	PSATabName                 string
}

// initializeSchedulers builds and starts the scheduler group, returning the
// result and a cancel function to shut them down.
func initializeSchedulers(ctx context.Context, deps schedulerDeps) (*scheduler.BuildResult, context.CancelFunc) {
	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	buildDeps := scheduler.BuildDeps{
		APITracker:                 deps.DBTracker,
		HealthChecker:              deps.DBTracker,
		AccessTracker:              deps.DBTracker,
		RefreshCandidates:          deps.RefreshCandidates,
		PriceProvider:              deps.PriceProvImpl,
		AuthService:                deps.AuthService,
		Logger:                     deps.Logger,
		SyncStateStore:             deps.SyncStateRepo,
		InventoryLister:            &inventoryListAdapter{repo: deps.PurchaseStore},
		SnapshotRefresher:          &snapshotRefreshAdapter{svc: deps.CampaignsService},
		SnapshotEnrichService:      deps.CampaignsService,
		AdvisorCollector:           deps.AdvisorService,
		AdvisorCache:               deps.AdvisorCacheRepo,
		AICallTracker:              deps.AICallRepo,
		CardLadderClient:           deps.CardLadderClient,
		CardLadderStore:            deps.CardLadderStore,
		CardLadderPurchaseLister:   deps.PurchaseStore,
		CardLadderValueUpdater:     deps.PurchaseStore,
		CardLadderGemRateUpdater:   deps.PurchaseStore,
		CardLadderSyncUpdater:      deps.PurchaseStore,
		CardLadderSalesStore:       deps.CardLadderSalesStore,
		CardLadderCompRefreshStore: deps.CardLadderCompRefreshStore,
		SchedulerStatsStore:        deps.SchedulerStatsStore,
	}
	// Wire DH event recorder (nil-safe)
	if deps.DHEventStore != nil {
		buildDeps.EventRecorder = deps.DHEventStore
	}
	// Wire Market Movers (nil-safe: only set if non-nil to avoid typed-nil interface issues)
	if deps.MMClient != nil {
		buildDeps.MMClient = deps.MMClient
	}
	if deps.MMStore != nil {
		buildDeps.MMStore = deps.MMStore
	}
	if deps.MMSalesStore != nil {
		buildDeps.MMSalesStore = deps.MMSalesStore
	}
	if deps.PurchaseStore != nil {
		buildDeps.MMPurchaseLister = deps.PurchaseStore
		buildDeps.MMValueUpdater = deps.PurchaseStore
	}
	// Nil-safe interface conversion for DH dependencies.
	if deps.DHClient != nil {
		buildDeps.DHClient = deps.DHClient
		buildDeps.DHOrdersClient = deps.DHClient
		buildDeps.DHInventoryListClient = deps.DHClient
		// Guard against typed-nil pointers: use individual stores instead of composite repo.
		if deps.PurchaseStore != nil {
			buildDeps.DHFieldsUpdater = deps.PurchaseStore
			buildDeps.PurchaseByCertLookup = deps.PurchaseStore
			buildDeps.DHPushPendingLister = deps.PurchaseStore
			buildDeps.DHPushStatusUpdater = deps.PurchaseStore
			buildDeps.DHPushCandidatesSaver = deps.PurchaseStore
			buildDeps.DHPushHoldSetter = deps.PurchaseStore
			buildDeps.DHPushAttemptsTracker = deps.PurchaseStore
		}
		if deps.DHStore != nil {
			buildDeps.DHPushConfigLoader = deps.DHStore
		}
		if deps.CardIDMappingRepo != nil {
			buildDeps.DHPushCardIDSaver = deps.CardIDMappingRepo
		}
		if deps.CampaignsService != nil {
			buildDeps.CampaignService = deps.CampaignsService
		}
	}
	if deps.DHIntelligenceRepo != nil {
		buildDeps.DHIntelligenceRepo = deps.DHIntelligenceRepo
	}
	if deps.DHTrajectoryRepo != nil {
		buildDeps.DHTrajectoryRepo = deps.DHTrajectoryRepo
	}
	if deps.DHCompCacheStore != nil {
		buildDeps.DHCompCacheStore = deps.DHCompCacheStore
	}
	if deps.DHSuggestionsRepo != nil {
		buildDeps.DHSuggestionsRepo = deps.DHSuggestionsRepo
	}
	if deps.DHDemandRepo != nil {
		buildDeps.DHDemandRepo = deps.DHDemandRepo
	}
	if deps.DHPriceSyncService != nil {
		buildDeps.DHPriceSyncService = deps.DHPriceSyncService
	}
	if deps.PurchaseStore != nil {
		buildDeps.DHUnsoldCardLister = deps.PurchaseStore
		buildDeps.DHIntelligenceSeedLister = deps.PurchaseStore
	}
	// Construct DH reconciler once; shared with scheduler (handlers.go builds
	// its own instance for the admin endpoint — that's intentional and fine
	// since the reconciler is stateless). All inputs are verified non-nil
	// above, so construction failure here is a wiring defect — log loudly
	// so the daily drift scan's absence doesn't go unnoticed.
	if deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() && deps.PurchaseStore != nil {
		var reconcileOpts []dhlisting.ReconcilerOption
		if deps.DHEventStore != nil {
			reconcileOpts = append(reconcileOpts, dhlisting.WithReconcileEventRecorder(deps.DHEventStore))
		}
		reconciler, err := dhlisting.NewReconciler(
			dhlistingadapter.NewInventorySnapshotAdapter(deps.DHClient),
			deps.PurchaseStore,
			deps.PurchaseStore,
			deps.Logger,
			reconcileOpts...,
		)
		if err != nil {
			deps.Logger.Error(ctx, "DH reconciler init failed; daily drift scan disabled", observability.Err(err))
		} else {
			buildDeps.DHReconciler = reconciler
		}
	}
	if deps.GapStore != nil {
		buildDeps.GapStore = deps.GapStore
	}
	// Wire PSA sync (nil-safe)
	if deps.PSASheetFetcher != nil {
		buildDeps.PSASheetFetcher = deps.PSASheetFetcher
		buildDeps.PSAImporter = deps.CampaignsService
		buildDeps.PSASpreadsheetID = deps.PSASpreadsheetID
		buildDeps.PSATabName = deps.PSATabName
	}

	// Wire cert enrichment (nil-safe)
	if deps.CertLookup != nil && deps.PurchaseStore != nil {
		buildDeps.CertLookup = deps.CertLookup
		buildDeps.PurchaseRepo = deps.PurchaseStore
		buildDeps.CampaignService = deps.CampaignsService
	}
	// Pass pre-built CertEnrichJob so the scheduler group uses the same instance
	// that was injected into inventory.service via WithCertEnrichEnqueuer.
	if deps.CertEnrichJob != nil {
		buildDeps.CertEnrichJobPrebuilt = deps.CertEnrichJob
	}

	// Wire crack cache refresh (uses inventory service for RefreshCrackCandidates).
	if deps.CampaignsService != nil {
		buildDeps.CrackCacheService = deps.CampaignsService
	}

	schedulerResult := scheduler.BuildGroup(deps.Config, buildDeps)

	// Wire the pricing-enrich job's providers now that CL/MM schedulers exist.
	// A nil pricer (CL or MM not configured) is filtered inside SetPricers.
	if deps.PricingEnrichJob != nil {
		deps.PricingEnrichJob.SetPricers(schedulerResult.CardLadderRefresh, schedulerResult.MMRefresh)
		schedulerResult.Group.Add(deps.PricingEnrichJob)
	}

	schedulerResult.Group.StartAll(schedulerCtx)

	return &schedulerResult, cancelScheduler
}
