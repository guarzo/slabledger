package main

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhlistingadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/gsheets"
	igclient "github.com/guarzo/slabledger/internal/adapters/clients/instagram"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/domain/tuning"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// handlerInputs bundles all values needed to construct HTTP handlers and
// assemble the ServerDependencies struct. Every field is set by runServer
// before calling createHandlers.
type handlerInputs struct {
	Cfg               *config.Config
	Logger            observability.Logger
	DB                *sqlite.DB
	PriceProvImpl     pricing.PriceProvider
	PriceRepo         *sqlite.DBTracker
	AuthService       auth.Service
	CampaignsService  inventory.Service
	ArbitrageService  arbitrage.Service
	PortfolioService  portfolio.Service
	TuningService     tuning.Service
	FinanceService    finance.Service
	ExportService     export.Service
	PurchaseStore     *sqlite.PurchaseStore
	SellSheetStore    *sqlite.SellSheetStore
	CardIDMappingRepo *sqlite.CardIDMappingRepository
	IntelRepo         *sqlite.MarketIntelligenceRepository
	SuggestionsRepo   *sqlite.DHSuggestionsRepository
	DemandRepo        *sqlite.DHDemandRepository
	AdvisorService    advisor.Service
	AdvisorCacheRepo  *sqlite.AdvisorCacheRepository
	AzureAIClient     advisor.LLMProvider
	AICallRepo        *sqlite.AICallRepository
	SocialService     social.Service
	SocialRepo        *sqlite.SocialRepository
	IGClient          *igclient.Client
	IGStore           *sqlite.InstagramStore
	MetricsRepo       *sqlite.MetricsRepository
	CLClient          *cardladder.Client
	CLStore           *sqlite.CardLadderStore
	MMStore           *sqlite.MarketMoversStore
	MMClient          *marketmovers.Client
	DHClient          *dh.Client
	SchedulerResult   *scheduler.BuildResult
	GSheetsClient     *gsheets.Client
	PendingItemsRepo  *sqlite.PendingItemsRepository
}

// handlerOutputs holds the constructed handlers that are also needed post-
// server for graceful shutdown (Wait calls).
type handlerOutputs struct {
	DHHandler      *handlers.DHHandler
	AdvisorHandler *handlers.AdvisorHandler
	SocialHandler  *handlers.SocialHandler
}

// createHandlers constructs all HTTP handlers, wires scheduler refresh
// callbacks, and assembles the ServerDependencies struct ready for
// startWebServer. Handler construction order follows the route order defined
// in docs/API.md. When adding new handlers, update docs/API.md in the same commit.
func createHandlers(ctx context.Context, in handlerInputs) (ServerDependencies, handlerOutputs) {
	logger := in.Logger

	// Card Ladder handler
	var clHandler *handlers.CardLadderHandler
	if in.CLStore != nil {
		clHandler = handlers.NewCardLadderHandler(in.CLStore, in.CLClient, logger)
	}

	// Market Movers handler
	var mmHandler *handlers.MarketMoversHandler
	if in.MMStore != nil {
		mmHandler = handlers.NewMarketMoversHandler(in.MMStore, in.MMClient, logger)
		if in.PurchaseStore != nil {
			mmHandler.SetPurchaseLister(in.PurchaseStore)
		}
	}

	// PSA Sync handler (pending items + admin status)
	var psaSyncHandler *handlers.PSASyncHandler
	if in.PendingItemsRepo != nil {
		var refresher handlers.PSASyncRefresher
		if in.SchedulerResult != nil && in.SchedulerResult.PSASync != nil {
			refresher = in.SchedulerResult.PSASync
		}
		var svc handlers.PSASyncPurchaseCreator
		if in.CampaignsService != nil {
			svc = in.CampaignsService
		}
		psaSyncHandler = handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
			PendingRepo:   in.PendingItemsRepo,
			Refresher:     refresher,
			Service:       svc,
			SpreadsheetID: in.Cfg.GoogleSheets.SpreadsheetID,
			Interval:      in.Cfg.PSASync.Interval.String(),
			Logger:        logger,
		})
	}

	// Opportunities (arbitrage endpoints)
	var opportunitiesHandler *handlers.OpportunitiesHandler
	if in.ArbitrageService != nil {
		opportunitiesHandler = handlers.NewOpportunitiesHandler(in.ArbitrageService, logger)
	}

	// DH handler (bulk match + intelligence; nil when client is not configured)
	var dhHandler *handlers.DHHandler
	if in.DHClient != nil && in.DHClient.EnterpriseAvailable() {
		reconciler, err := dhlisting.NewReconciler(
			dhlistingadapter.NewInventorySnapshotAdapter(in.DHClient),
			in.PurchaseStore,
			in.PurchaseStore,
			logger,
		)
		if err != nil {
			logger.Warn(ctx, "DH reconciler init failed", observability.Err(err))
		}
		dhHandler = handlers.NewDHHandler(handlers.DHHandlerDeps{
			CertResolver:      in.DHClient,
			CardIDSaver:       in.CardIDMappingRepo,
			PurchaseLister:    in.PurchaseStore,
			InventoryPusher:   in.DHClient,
			DHFieldsUpdater:   in.PurchaseStore,
			PushStatusUpdater: in.PurchaseStore,
			CandidatesSaver:   in.PurchaseStore,
			StatusCounter:     in.PurchaseStore,
			PendingLister:     in.PurchaseStore,
			IntelRepo:         in.IntelRepo,
			SuggestionsRepo:   in.SuggestionsRepo,
			IntelCounter:      in.IntelRepo,
			SuggestCounter:    in.SuggestionsRepo,
			Logger:            logger,
			BaseCtx:           ctx,
			HealthReporter:    in.DHClient,
			CountsFetcher:     in.DHClient,
			DHApproveService:  in.CampaignsService,
			MatchConfirmer:    in.DHClient,
			Reconciler:        reconciler,
		})
		logger.Info(ctx, "DH handler initialized")
	}

	// Sell sheet items handler
	sellSheetItemsHandler := handlers.NewSellSheetItemsHandler(in.SellSheetStore, logger)

	// Niches handler (DH niche-opportunity leaderboard) and campaign-signals handler.
	// Both share the same demand.Service instance. Requires the DH demand repo;
	// coverage lookup runs against the campaigns DB.
	var nichesHandler *handlers.NichesHandler
	var campaignSignalsHandler *handlers.CampaignSignalsHandler
	if in.DemandRepo != nil {
		coverage := sqlite.NewCampaignCoverageLookup(in.DB.DB)
		demandSvc := demand.NewService(in.DemandRepo, coverage)
		nichesHandler = handlers.NewNichesHandler(demandSvc, logger)
		campaignSignalsHandler = handlers.NewCampaignSignalsHandler(demandSvc, logger)
	}

	// Card catalog handler (CL card catalog search; nil when CL is not configured)
	var cardCatalogHandler *handlers.CardCatalogHandler
	if in.CLClient != nil && in.CLClient.Available() {
		cardCatalogHandler = handlers.NewCardCatalogHandler(in.CLClient, logger)
	}

	// Wire Card Ladder manual refresh into the handler
	if clHandler != nil && in.SchedulerResult != nil && in.SchedulerResult.CardLadderRefresh != nil {
		clHandler.SetRefresher(in.SchedulerResult.CardLadderRefresh)
	}
	if clHandler != nil && in.PurchaseStore != nil {
		clHandler.SetPurchaseLister(in.PurchaseStore)
		clHandler.SetSyncUpdater(in.PurchaseStore)
	}

	// Wire Market Movers manual refresh into the handler
	if mmHandler != nil && in.SchedulerResult != nil && in.SchedulerResult.MMRefresh != nil {
		mmHandler.SetRefresher(in.SchedulerResult.MMRefresh)
	}

	// Price hints handler
	priceHintsHandler := handlers.NewPriceHintsHandler(in.CardIDMappingRepo, logger)

	// Pricing diagnostics handler
	pricingDiagRepo := sqlite.NewPricingDiagnosticsRepository(in.DB.DB)
	pricingDiagHandler := handlers.NewPricingDiagnosticsHandler(pricingDiagRepo, logger)

	// Advisor handler (if advisor was initialized)
	var advisorHandler *handlers.AdvisorHandler
	if in.AdvisorService != nil {
		advisorHandler = handlers.NewAdvisorHandler(in.AdvisorService, in.CampaignsService, in.AdvisorCacheRepo, logger)
	}

	// Social handler
	mediaDir := in.Cfg.Server.MediaDir
	baseURL := in.Cfg.Server.BaseURL
	if baseURL == "" {
		logger.Warn(ctx, "BASE_URL is not set — slide URLs will be derived from request headers")
	} else {
		logger.Info(ctx, "BASE_URL configured",
			observability.String("baseURL", baseURL))
	}
	socialHandler := handlers.NewSocialHandler(in.SocialService, in.SocialRepo, logger, mediaDir, baseURL,
		handlers.WithBaseCtx(ctx))

	// Wire metrics repository into social handler for API endpoints
	socialHandler.WithMetricsRepo(in.MetricsRepo)

	// AI status handler — only wire tracker when an LLM provider is configured
	var aiTracker ai.AICallTracker
	if in.AzureAIClient != nil {
		aiTracker = in.AICallRepo
	}
	aiStatusHandler := handlers.NewAIStatusHandler(aiTracker, logger)

	// Price flags handler
	priceFlagsHandler := handlers.NewPriceFlagsHandler(in.CampaignsService, logger)

	// Instagram handler (if client + store were initialized)
	var igHandler *handlers.InstagramHandler
	if in.IGClient != nil && in.IGStore != nil && in.AuthService != nil {
		igHandler = handlers.NewInstagramHandler(in.IGClient, in.IGStore, in.SocialService, in.AuthService, logger)
	}

	// Assemble ServerDependencies
	deps := ServerDependencies{
		Config:                    in.Cfg,
		Logger:                    logger,
		PriceProv:                 in.PriceProvImpl,
		HealthChecker:             in.PriceRepo,
		APITracker:                in.PriceRepo,
		AuthService:               in.AuthService,
		CampaignsService:          in.CampaignsService,
		ArbitrageService:          in.ArbitrageService,
		PortfolioService:          in.PortfolioService,
		TuningService:             in.TuningService,
		PriceHintsHandler:         priceHintsHandler,
		PricingDiagnosticsHandler: pricingDiagHandler,
		CampaignsRepo:             in.PurchaseStore,
		PricingAPIKey:             in.Cfg.Adapters.PricingAPIKey,
		AdvisorHandler:            advisorHandler,
		SocialHandler:             socialHandler,
		InstagramHandler:          igHandler,
		AIStatusHandler:           aiStatusHandler,
		PriceFlagsHandler:         priceFlagsHandler,
		CardLadderHandler:         clHandler,
		MarketMoversHandler:       mmHandler,
		PSASyncHandler:            psaSyncHandler,
		OpportunitiesHandler:      opportunitiesHandler,
		DHHandler:                 dhHandler,
		SellSheetItemsHandler:     sellSheetItemsHandler,
		CardCatalogHandler:        cardCatalogHandler,
		NichesHandler:             nichesHandler,
		CampaignSignalsHandler:    campaignSignalsHandler,
	}
	// Build DHListingService from available components.
	// Nil-safe: only create the service if at least the lister client is available.
	if in.DHClient != nil {
		listingOpts := []dhlisting.DHListingServiceOption{
			dhlisting.WithDHListingLister(dhlistingadapter.NewInventoryAdapter(in.DHClient)),
			dhlisting.WithDHListingCertResolver(dhlistingadapter.NewCertResolverAdapter(in.DHClient)),
			dhlisting.WithDHListingPusher(dhlistingadapter.NewInventoryPusherAdapter(in.DHClient)),
		}
		if in.PurchaseStore != nil {
			listingOpts = append(listingOpts,
				dhlisting.WithDHListingFieldsUpdater(in.PurchaseStore),
				dhlisting.WithDHListingPushStatusUpdater(in.PurchaseStore),
				dhlisting.WithDHListingCandidatesSaver(in.PurchaseStore),
			)
		}
		if in.CardIDMappingRepo != nil {
			listingOpts = append(listingOpts, dhlisting.WithDHListingCardIDSaver(in.CardIDMappingRepo))
		}
		svc, err := dhlisting.NewDHListingService(
			in.CampaignsService, in.Logger, listingOpts...,
		)
		if err != nil {
			in.Logger.Error(ctx, "create DH listing service", observability.Err(err))
		} else {
			deps.DHListingService = svc
		}
	}

	// Wire services from initialization
	deps.ExportService = in.ExportService
	deps.FinanceService = in.FinanceService

	// Wire Google Sheets for PSA sync (if client + spreadsheet configured)
	if in.GSheetsClient != nil && in.Cfg.GoogleSheets.SpreadsheetID != "" {
		deps.SheetFetcher = in.GSheetsClient
		deps.SheetsSpreadsheetID = in.Cfg.GoogleSheets.SpreadsheetID
		deps.SheetsTabName = in.Cfg.GoogleSheets.TabName
	}

	out := handlerOutputs{
		DHHandler:      dhHandler,
		AdvisorHandler: advisorHandler,
		SocialHandler:  socialHandler,
	}
	return deps, out
}
