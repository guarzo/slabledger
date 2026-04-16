package main

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// schedulerDeps bundles all dependencies needed by initializeSchedulers.
type schedulerDeps struct {
	Config               *config.Config
	Logger               observability.Logger
	DBTracker            *sqlite.DBTracker
	RefreshCandidates    pricing.RefreshCandidateProvider
	PriceProvImpl        pricing.PriceProvider
	AuthService          auth.Service
	SyncStateRepo        *sqlite.SyncStateRepository
	CardIDMappingRepo    *sqlite.CardIDMappingRepository
	CampaignStore        *sqlite.CampaignStore
	PurchaseStore        *sqlite.PurchaseStore
	DHStore              *sqlite.DHStore
	CampaignsService     inventory.Service
	CertLookup           inventory.CertLookup
	CertEnrichJob        *scheduler.CertEnrichJob // pre-built; nil if PSA not configured
	AdvisorService       advisor.Service
	AdvisorCacheRepo     *sqlite.AdvisorCacheRepository
	AICallRepo           *sqlite.AICallRepository
	CardLadderClient     *cardladder.Client
	CardLadderStore      *sqlite.CardLadderStore
	CardLadderSalesStore *sqlite.CLSalesStore
	MMClient             *marketmovers.Client
	MMStore              *sqlite.MarketMoversStore
	DHClient             *dh.Client
	DHIntelligenceRepo   *sqlite.MarketIntelligenceRepository
	DHSuggestionsRepo    *sqlite.DHSuggestionsRepository
	DHDemandRepo         *sqlite.DHDemandRepository
	GapStore             *sqlite.GapStore
	PSASheetFetcher      scheduler.SheetFetcher
	PSASpreadsheetID     string
	PSATabName           string
}

// initializeSchedulers builds and starts the scheduler group, returning the
// result and a cancel function to shut them down.
func initializeSchedulers(ctx context.Context, deps schedulerDeps) (*scheduler.BuildResult, context.CancelFunc) {
	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	buildDeps := scheduler.BuildDeps{
		APITracker:               deps.DBTracker,
		HealthChecker:            deps.DBTracker,
		AccessTracker:            deps.DBTracker,
		RefreshCandidates:        deps.RefreshCandidates,
		PriceProvider:            deps.PriceProvImpl,
		AuthService:              deps.AuthService,
		Logger:                   deps.Logger,
		SyncStateStore:           deps.SyncStateRepo,
		InventoryLister:          &inventoryListAdapter{repo: deps.PurchaseStore},
		SnapshotRefresher:        &snapshotRefreshAdapter{svc: deps.CampaignsService},
		SnapshotEnrichService:    deps.CampaignsService,
		AdvisorCollector:         deps.AdvisorService,
		AdvisorCache:             deps.AdvisorCacheRepo,
		AICallTracker:            deps.AICallRepo,
		CardLadderClient:         deps.CardLadderClient,
		CardLadderStore:          deps.CardLadderStore,
		CardLadderPurchaseLister: deps.PurchaseStore,
		CardLadderValueUpdater:   deps.PurchaseStore,
		CardLadderGemRateUpdater: deps.PurchaseStore,
		CardLadderSyncUpdater:    deps.PurchaseStore,
		CardLadderSalesStore:     deps.CardLadderSalesStore,
	}
	// Wire Market Movers (nil-safe: only set if non-nil to avoid typed-nil interface issues)
	if deps.MMClient != nil {
		buildDeps.MMClient = deps.MMClient
	}
	if deps.MMStore != nil {
		buildDeps.MMStore = deps.MMStore
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
	if deps.DHSuggestionsRepo != nil {
		buildDeps.DHSuggestionsRepo = deps.DHSuggestionsRepo
	}
	if deps.DHDemandRepo != nil {
		buildDeps.DHDemandRepo = deps.DHDemandRepo
	}
	if deps.PurchaseStore != nil {
		buildDeps.DHUnsoldCardLister = deps.PurchaseStore
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
	schedulerResult.Group.StartAll(schedulerCtx)

	return &schedulerResult, cancelScheduler
}
