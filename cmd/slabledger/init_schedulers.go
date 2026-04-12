package main

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// schedulerDeps bundles all dependencies needed by initializeSchedulers.
type schedulerDeps struct {
	Config               *config.Config
	Logger               observability.Logger
	DBTracker            *sqlite.DBTracker
	RefreshCandidates    pricing.RefreshCandidateProvider
	PriceProvImpl        pricing.PriceProvider
	CardProvImpl         *tcgdex.TCGdex
	AuthService          auth.Service
	SyncStateRepo        *sqlite.SyncStateRepository
	CardIDMappingRepo    *sqlite.CardIDMappingRepository
	CampaignsRepo        *sqlite.CampaignsRepository
	CampaignsService     campaigns.Service
	AdvisorService       advisor.Service
	AdvisorCacheRepo     *sqlite.AdvisorCacheRepository
	AICallRepo           *sqlite.AICallRepository
	SocialService        social.Service
	SocialRepo           *sqlite.SocialRepository
	PublisherConfigured  bool
	IGTokenRefresher     scheduler.InstagramTokenRefresher
	MetricsPostLister    social.MetricsPostLister
	MetricsSaver         social.MetricsSaver
	InsightsPoller       social.InsightsPoller
	PicksService         picks.Service
	CardLadderClient     *cardladder.Client
	CardLadderStore      *sqlite.CardLadderStore
	CardLadderSalesStore *sqlite.CLSalesStore
	MMClient             *marketmovers.Client
	MMStore              *sqlite.MarketMoversStore
	DHClient             *dh.Client
	DHIntelligenceRepo   *sqlite.MarketIntelligenceRepository
	DHSuggestionsRepo    *sqlite.DHSuggestionsRepository
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
		CardProvider:             deps.CardProvImpl,
		AuthService:              deps.AuthService,
		Logger:                   deps.Logger,
		SyncStateStore:           deps.SyncStateRepo,
		NewSetsProvider:          deps.CardProvImpl.RegistryManager(),
		InventoryLister:          &inventoryListAdapter{repo: deps.CampaignsRepo},
		SnapshotRefresher:        &snapshotRefreshAdapter{svc: deps.CampaignsService},
		SnapshotEnrichService:    deps.CampaignsService,
		SnapshotHistoryLister:    deps.CampaignsRepo,
		SnapshotHistoryRecorder:  deps.CampaignsRepo,
		AdvisorCollector:         deps.AdvisorService,
		AdvisorCache:             deps.AdvisorCacheRepo,
		AICallTracker:            deps.AICallRepo,
		SocialContentDetector:    deps.SocialService,
		InstagramTokenRefresher:  deps.IGTokenRefresher,
		MetricsPostLister:        deps.MetricsPostLister,
		MetricsSaver:             deps.MetricsSaver,
		InsightsPoller:           deps.InsightsPoller,
		PicksGenerator:           deps.PicksService,
		CardLadderClient:         deps.CardLadderClient,
		CardLadderStore:          deps.CardLadderStore,
		CardLadderPurchaseLister: deps.CampaignsRepo,
		CardLadderValueUpdater:   deps.CampaignsRepo,
		CardLadderGemRateUpdater: deps.CampaignsRepo,
		CardLadderSyncUpdater:    deps.CampaignsRepo,
		CardLadderCLRecorder:     deps.CampaignsRepo,
		CardLadderSalesStore:     deps.CardLadderSalesStore,
	}
	// Wire Market Movers (nil-safe: only set if non-nil to avoid typed-nil interface issues)
	if deps.MMClient != nil {
		buildDeps.MMClient = deps.MMClient
	}
	if deps.MMStore != nil {
		buildDeps.MMStore = deps.MMStore
	}
	if deps.CampaignsRepo != nil {
		buildDeps.MMPurchaseLister = deps.CampaignsRepo
		buildDeps.MMValueUpdater = deps.CampaignsRepo
	}
	// Nil-safe interface conversion for DH dependencies.
	if deps.DHClient != nil {
		buildDeps.DHClient = deps.DHClient
		buildDeps.DHOrdersClient = deps.DHClient
		buildDeps.DHInventoryListClient = deps.DHClient
		// Guard against typed-nil pointers: a nil *sqlite.CampaignsRepository
		// assigned to an interface produces a non-nil interface wrapping a nil pointer.
		if deps.CampaignsRepo != nil {
			buildDeps.DHFieldsUpdater = deps.CampaignsRepo
			buildDeps.PurchaseByCertLookup = deps.CampaignsRepo
			buildDeps.DHPushPendingLister = deps.CampaignsRepo
			buildDeps.DHPushStatusUpdater = deps.CampaignsRepo
			buildDeps.DHPushCandidatesSaver = deps.CampaignsRepo
			buildDeps.DHPushConfigLoader = deps.CampaignsRepo
			buildDeps.DHPushHoldSetter = deps.CampaignsRepo
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
	// Wire social publish (nil-safe: only set if publisher was actually configured)
	if deps.PublisherConfigured {
		buildDeps.SocialPublisher = deps.SocialService
		buildDeps.SocialPublishRepo = deps.SocialRepo
	}
	// DHSocialRepo is always wired when SocialRepo is available — DHSocialScheduler
	// does not require Instagram OAuth, only a DH Enterprise key.
	if deps.SocialRepo != nil {
		buildDeps.DHSocialRepo = deps.SocialRepo
	}
	schedulerResult := scheduler.BuildGroup(deps.Config, buildDeps)
	schedulerResult.Group.StartAll(schedulerCtx)

	return &schedulerResult, cancelScheduler
}
