package main

import (
	"context"
	"os"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/gsheets"
	igclient "github.com/guarzo/slabledger/internal/adapters/clients/instagram"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/favorites"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// handlerInputs bundles all values needed to construct HTTP handlers and
// assemble the ServerDependencies struct. Every field is set by runServer
// before calling createHandlers.
type handlerInputs struct {
	Ctx               context.Context
	Cfg               *config.Config
	Logger            observability.Logger
	DB                *sqlite.DB
	CardProvImpl      *tcgdex.TCGdex
	PriceProvImpl     pricing.PriceProvider
	PriceRepo         *sqlite.DBTracker
	AuthService       auth.Service
	FavoritesService  favorites.Service
	CampaignsService  campaigns.Service
	CampaignsRepo     *sqlite.CampaignsRepository
	CardIDMappingRepo *sqlite.CardIDMappingRepository
	CardRequestRepo   *sqlite.CardRequestRepository
	IntelRepo         *sqlite.MarketIntelligenceRepository
	SuggestionsRepo   *sqlite.DHSuggestionsRepository
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
	PicksService      picks.Service
	SchedulerResult   *scheduler.BuildResult
	GSheetsClient     *gsheets.Client
}

// handlerOutputs holds the constructed handlers that are also needed post-
// server for graceful shutdown (Wait calls).
type handlerOutputs struct {
	DHHandler      *handlers.DHHandler
	AdvisorHandler *handlers.AdvisorHandler
}

// createHandlers constructs all HTTP handlers, wires scheduler refresh
// callbacks, and assembles the ServerDependencies struct ready for
// startWebServer.
func createHandlers(in handlerInputs) (ServerDependencies, handlerOutputs) {
	ctx := in.Ctx
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
		if in.CampaignsRepo != nil {
			mmHandler.SetPurchaseLister(in.CampaignsRepo)
		}
	}

	// Picks handler
	picksHandler := handlers.NewPicksHandler(in.PicksService, logger)

	// Opportunities (arbitrage endpoints)
	var opportunitiesHandler *handlers.OpportunitiesHandler
	if in.CampaignsService != nil {
		opportunitiesHandler = handlers.NewOpportunitiesHandler(in.CampaignsService, logger)
	}

	// DH handler (bulk match + intelligence; nil when client is not configured)
	var dhHandler *handlers.DHHandler
	if in.DHClient != nil && in.DHClient.EnterpriseAvailable() {
		dhHandler = handlers.NewDHHandler(handlers.DHHandlerDeps{
			CertResolver:      in.DHClient,
			CardIDSaver:       in.CardIDMappingRepo,
			PurchaseLister:    in.CampaignsRepo,
			InventoryPusher:   in.DHClient,
			DHFieldsUpdater:   in.CampaignsRepo,
			PushStatusUpdater: in.CampaignsRepo,
			CandidatesSaver:   in.CampaignsRepo,
			StatusCounter:     in.CampaignsRepo,
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
		})
		logger.Info(ctx, "DH handler initialized")
	}

	// Sell sheet items handler
	sellSheetItemsHandler := handlers.NewSellSheetItemsHandler(in.CampaignsRepo, logger)

	// Card catalog handler (CL card catalog search; nil when CL is not configured)
	var cardCatalogHandler *handlers.CardCatalogHandler
	if in.CLClient != nil && in.CLClient.Available() {
		cardCatalogHandler = handlers.NewCardCatalogHandler(in.CLClient, logger)
	}

	// Wire Card Ladder manual refresh into the handler
	if clHandler != nil && in.SchedulerResult.CardLadderRefresh != nil {
		clHandler.SetRefresher(in.SchedulerResult.CardLadderRefresh)
	}
	if clHandler != nil && in.CampaignsRepo != nil {
		clHandler.SetPurchaseLister(in.CampaignsRepo)
	}

	// Wire Market Movers manual refresh into the handler
	if mmHandler != nil && in.SchedulerResult.MMRefresh != nil {
		mmHandler.SetRefresher(in.SchedulerResult.MMRefresh)
	}

	// Price hints handler
	priceHintsHandler := handlers.NewPriceHintsHandler(in.CardIDMappingRepo, logger)

	// Pricing diagnostics handler
	pricingDiagRepo := sqlite.NewPricingDiagnosticsRepository(in.DB.DB)
	pricingDiagHandler := handlers.NewPricingDiagnosticsHandler(pricingDiagRepo, logger)

	// Card request handler (read-only)
	cardRequestHandler := handlers.NewCardRequestHandlers(in.CardRequestRepo, nil, "", logger)

	// Advisor handler (if advisor was initialized)
	var advisorHandler *handlers.AdvisorHandler
	if in.AdvisorService != nil {
		advisorHandler = handlers.NewAdvisorHandler(in.AdvisorService, in.CampaignsService, in.AdvisorCacheRepo, logger)
	}

	// Social handler
	mediaDir := os.Getenv("MEDIA_DIR")
	if mediaDir == "" {
		mediaDir = "./data/media"
	}
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		logger.Warn(context.Background(), "BASE_URL is not set — slide URLs will be derived from request headers")
	} else {
		logger.Info(context.Background(), "BASE_URL configured",
			observability.String("baseURL", baseURL))
	}
	socialHandler := handlers.NewSocialHandler(in.SocialService, in.SocialRepo, logger, mediaDir, baseURL)

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
		CardProv:                  in.CardProvImpl,
		PriceProv:                 in.PriceProvImpl,
		HealthChecker:             in.PriceRepo,
		APITracker:                in.PriceRepo,
		AuthService:               in.AuthService,
		FavoritesService:          in.FavoritesService,
		CampaignsService:          in.CampaignsService,
		CacheStatsProvider:        in.CardProvImpl,
		PriceHintsHandler:         priceHintsHandler,
		CardRequestHandler:        cardRequestHandler,
		PricingDiagnosticsHandler: pricingDiagHandler,
		CampaignsRepo:             in.CampaignsRepo,
		PricingAPIKey:             in.Cfg.Adapters.PricingAPIKey,
		AdvisorHandler:            advisorHandler,
		SocialHandler:             socialHandler,
		InstagramHandler:          igHandler,
		AIStatusHandler:           aiStatusHandler,
		PriceFlagsHandler:         priceFlagsHandler,
		CardLadderHandler:         clHandler,
		MarketMoversHandler:       mmHandler,
		PicksHandler:              picksHandler,
		OpportunitiesHandler:      opportunitiesHandler,
		DHHandler:                 dhHandler,
		SellSheetItemsHandler:     sellSheetItemsHandler,
		CardCatalogHandler:        cardCatalogHandler,
	}
	// Build DHListingService from available components.
	// Nil-safe: only create the service if at least the lister client is available.
	if in.DHClient != nil {
		var listingOpts []campaigns.DHListingServiceOption
		listingOpts = append(listingOpts, campaigns.WithDHListingLister(
			dhlisting.NewInventoryListerAdapter(in.DHClient),
		))
		listingOpts = append(listingOpts, campaigns.WithDHListingCertResolver(
			dhlisting.NewCertResolverAdapter(in.DHClient),
		))
		listingOpts = append(listingOpts, campaigns.WithDHListingPusher(
			dhlisting.NewInventoryPusherAdapter(in.DHClient),
		))
		if in.CampaignsRepo != nil {
			listingOpts = append(listingOpts, campaigns.WithDHListingFieldsUpdater(in.CampaignsRepo))
			listingOpts = append(listingOpts, campaigns.WithDHListingPushStatusUpdater(in.CampaignsRepo))
			listingOpts = append(listingOpts, campaigns.WithDHListingCandidatesSaver(in.CampaignsRepo))
		}
		if in.CardIDMappingRepo != nil {
			listingOpts = append(listingOpts, campaigns.WithDHListingCardIDSaver(in.CardIDMappingRepo))
		}
		deps.DHListingService = campaigns.NewDHListingService(
			in.CampaignsService, in.Logger, listingOpts...,
		)
	}

	// Wire Google Sheets for PSA sync (if client + spreadsheet configured)
	if in.GSheetsClient != nil && in.Cfg.GoogleSheets.SpreadsheetID != "" {
		deps.SheetFetcher = in.GSheetsClient
		deps.SheetsSpreadsheetID = in.Cfg.GoogleSheets.SpreadsheetID
		deps.SheetsTabName = in.Cfg.GoogleSheets.TabName
	}

	out := handlerOutputs{
		DHHandler:      dhHandler,
		AdvisorHandler: advisorHandler,
	}
	return deps, out
}
