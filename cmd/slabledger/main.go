package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/joho/godotenv"

	// Concrete implementations (only imported in main for wiring - Hexagonal Architecture)
	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/google"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/favorites"
	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// initLogger creates a new logger with the specified level and format
func initLogger(level string, jsonFormat bool) observability.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	format := "text"
	if jsonFormat {
		format = "json"
	}

	return telemetry.NewSlogLogger(slogLevel, format)
}

func main() {
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: Error loading .env file: %v\n", err)
		}
	}

	var mode string
	var args []string

	if len(os.Args) == 1 {
		mode = "server"
		args = []string{}
	} else {
		firstArg := os.Args[1]

		switch firstArg {
		case "--help", "-h", "help":
			showHelp()
			os.Exit(0)
		case "--version", "-v", "version":
			config.PrintVersion()
			os.Exit(0)
		case "server":
			mode = "server"
			args = os.Args[2:]
		case "--web":
			mode = "server"
			args = os.Args[1:]
		case "admin":
			mode = "admin"
			args = os.Args[2:]
		default:
			if firstArg[0] == '-' {
				mode = "server"
				args = os.Args[1:]
			} else {
				fmt.Fprintf(os.Stderr, "Error: Unknown command '%s'\n\n", firstArg)
				showHelp()
				os.Exit(1)
			}
		}
	}

	switch mode {
	case "admin":
		if err := handleAdminCommand(args); err != nil {
			fmt.Fprintf(os.Stderr, "Admin command failed: %v\n", err)
			os.Exit(1)
		}
	case "server":
		cfg, err := config.Load(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
			os.Exit(1)
		}
		logger := initLogger(cfg.Logging.Level, cfg.Logging.JSON)

		if err := runServer(&cfg, logger); err != nil {
			logger.Error(context.Background(), "Server error", observability.Err(err))
			os.Exit(1)
		}
	}
}

func showHelp() {
	fmt.Println(`slabledger - Graded Card Portfolio Tracker

USAGE:
    slabledger [command] [arguments]

COMMANDS:
    server              Start web server (default if no command specified)
    admin <command>     Administrative and operational commands
    help                Show this help message
    version             Show version information

SERVER MODE:
    slabledger                    # Start web server (default port 8081)
    slabledger server             # Explicit server mode
    slabledger server --port 9090 # Custom port

EXAMPLES:
    slabledger admin cache-stats       # Show cache statistics

Documentation: docs/USER_GUIDE.md
Web Interface: http://localhost:8081`)
}

func runServer(cfg *config.Config, logger observability.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	envValidation := validateEnvironmentVariables(ctx, logger, cfg)
	if len(envValidation.MissingRequired) > 0 {
		return fmt.Errorf("missing required environment variables: %v. See logs above for details", envValidation.MissingRequired)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info(ctx, "Received interrupt signal, initiating graceful shutdown")
		cancel()
	}()

	// Initialize cache
	appCache := initializeCache(cfg.Cache.Path)

	// Initialize database
	dbPath, err := resolveDatabasePath(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create database directory %s: %w", dbDir, err)
	}

	db, err := sqlite.Open(dbPath, logger)
	if err != nil {
		return fmt.Errorf("open database %s: %w", dbPath, err)
	}
	// db.Close() blocks until in-flight queries complete. Safe because schedulers
	// are stopped and HTTP server is drained before this runs.
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn(ctx, "Failed to close database", observability.Err(err))
		}
	}()

	logger.Info(ctx, "database opened", observability.String("path", dbPath))

	migrationsPath := cfg.Database.MigrationsPath
	if err := sqlite.RunMigrations(db, migrationsPath); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Create DB tracker (API tracking, access tracking, health checks)
	priceRepo := sqlite.NewDBTracker(db)
	refreshCandidateRepo := sqlite.NewRefreshCandidateRepository(db.DB)

	// Initialize authentication
	var authService auth.Service
	var authRepo auth.Repository
	googleConfig := config.LoadGoogleOAuthConfig()

	if encryptionKey := cfg.Auth.EncryptionKey; encryptionKey != "" {
		encryptor, err := crypto.NewAESEncryptor(encryptionKey)
		if err != nil {
			return fmt.Errorf("initialize encryptor: %w", err)
		}

		authRepo = sqlite.NewAuthRepository(db.DB, encryptor)

		if googleConfig.IsConfigured() {
			authService = google.NewOAuthService(
				authRepo, logger,
				googleConfig.ClientID, googleConfig.ClientSecret,
				googleConfig.RedirectURI, googleConfig.Scopes,
			)
			logger.Info(ctx, "authentication service initialized",
				observability.String("redirect_uri", googleConfig.RedirectURI))
		}
	}

	// Initialize favorites
	favoritesRepo := sqlite.NewFavoritesRepository(db.DB)
	favoritesService := favorites.NewService(favoritesRepo)

	// Initialize providers
	cardProvImpl := tcgdex.NewTCGdex(appCache, logger)

	// Card ID mapping repository (caches external provider IDs)
	cardIDMappingRepo := sqlite.NewCardIDMappingRepository(db.DB)

	// Initialize DH client (optional — market intelligence + pricing source)
	var dhClient *dh.Client
	if cfg.Adapters.DHEnterpriseKey != "" && cfg.Adapters.DHBaseURL != "" {
		dhClient = dh.NewClient(
			cfg.Adapters.DHBaseURL,
			dh.WithLogger(logger),
			dh.WithRateLimitRPS(cfg.DH.RateLimitRPS),
			dh.WithEnterpriseKey(cfg.Adapters.DHEnterpriseKey),
			dh.WithPSAKeys(cfg.Adapters.PSAToken),
		)
		psaKeyCount := len(strings.Split(cfg.Adapters.PSAToken, ","))
		if cfg.Adapters.PSAToken == "" {
			psaKeyCount = 0
		}
		logger.Info(ctx, "DH client initialized",
			observability.Int("psa_keys", psaKeyCount))
	}

	// DH repositories (always created — tables exist after migration 000028)
	intelRepo := sqlite.NewMarketIntelligenceRepository(db.DB)
	suggestionsRepo := sqlite.NewDHSuggestionsRepository(db.DB)

	priceProvImpl, err := initializePriceProviders(
		ctx, logger, cardIDMappingRepo,
		dhClient,
	)
	if err != nil {
		return err
	}

	// Create encryptor early — needed by Card Ladder, Market Movers, and MM mapping adapter.
	var clEncryptor crypto.Encryptor
	if cfg.Auth.EncryptionKey != "" {
		var encErr error
		clEncryptor, encErr = crypto.NewAESEncryptor(cfg.Auth.EncryptionKey)
		if encErr != nil {
			logger.Warn(ctx, "encryptor initialization failed, CL/MM token persistence disabled",
				observability.Err(encErr))
		}
	}

	// Create MM store early so the campaigns service can use it for export enrichment.
	var mmStore *sqlite.MarketMoversStore
	if clEncryptor != nil {
		mmStore = sqlite.NewMarketMoversStore(db.DB, clEncryptor)
	}

	campaignsService, campaignsRepo, cardRequestRepo := initializeCampaignsService(
		ctx, cfg, logger, db, priceProvImpl, intelRepo, mmStore,
	)

	// Sync state repository (for delta poll timestamps)
	syncStateRepo := sqlite.NewSyncStateRepository(db.DB)

	// AI call tracking
	aiCallRepo := sqlite.NewAICallRepository(db)

	// Build advisor tool options — inject intelligence repos.
	var advisorToolOpts []advisortool.ExecutorOption
	advisorToolOpts = append(advisorToolOpts, advisortool.WithIntelligenceRepo(intelRepo))
	advisorToolOpts = append(advisorToolOpts, advisortool.WithSuggestionsRepo(suggestionsRepo))
	gapStore := sqlite.NewGapStore(db.DB)
	advisorToolOpts = append(advisorToolOpts, advisortool.WithGapStore(gapStore))

	azureAIClient, advisorService, advisorCacheRepo, err := initializeAdvisorService(
		ctx, cfg, logger, db, aiCallRepo, campaignsService, advisorToolOpts...,
	)
	if err != nil {
		return err
	}

	socialService, socialRepo, igClient, igStore, igTokenRefresher := initializeSocialService(
		ctx, cfg, logger, db, azureAIClient, aiCallRepo,
	)

	metricsRepo, insightsPoller := initializeMetricsPoller(ctx, db, igClient, igStore, logger)

	// Initialize Card Ladder (encryptor was created earlier for MM mapping adapter)
	clClient, _, clStore := initializeCardLadder(ctx, logger, db, clEncryptor)
	var clHandler *handlers.CardLadderHandler
	var clSalesStore *sqlite.CLSalesStore
	if clStore != nil {
		clHandler = handlers.NewCardLadderHandler(clStore, clClient, logger)
		clSalesStore = sqlite.NewCLSalesStore(db.DB)
	}

	// Initialize Market Movers client (store was created earlier for campaigns service)
	mmClient, _ := initializeMarketMovers(ctx, logger, db, clEncryptor)
	var mmHandler *handlers.MarketMoversHandler
	if mmStore != nil {
		mmHandler = handlers.NewMarketMoversHandler(mmStore, mmClient, logger)
		if campaignsRepo != nil {
			mmHandler.SetPurchaseLister(campaignsRepo)
		}
	}

	// Initialize picks
	picksRepo := sqlite.NewPicksRepository(db.DB)
	profitabilityProv := sqlite.NewProfitabilityProvider(db.DB)
	inventoryProv := sqlite.NewInventoryProvider(db.DB)

	picksService := picks.NewService(picksRepo, azureAIClient, profitabilityProv, inventoryProv, logger)
	picksHandler := handlers.NewPicksHandler(picksService, logger)

	// Create opportunities handler (arbitrage endpoints)
	var opportunitiesHandler *handlers.OpportunitiesHandler
	if campaignsService != nil {
		opportunitiesHandler = handlers.NewOpportunitiesHandler(campaignsService, logger)
	}

	// Create DH handler (bulk match + intelligence; nil when client is not configured)
	var dhHandler *handlers.DHHandler
	if dhClient != nil && dhClient.EnterpriseAvailable() {
		dhHandler = handlers.NewDHHandler(
			dhClient, cardIDMappingRepo, campaignsRepo,
			dhClient,      // DHInventoryPusher
			campaignsRepo, // DHFieldsUpdater
			campaignsRepo, // DHPushStatusUpdater
			campaignsRepo, // DHCandidatesSaver
			campaignsRepo, // DHStatusCounter
			intelRepo, suggestionsRepo,
			intelRepo, suggestionsRepo,
			logger,
			ctx,
			dhClient,         // DHHealthReporter
			dhClient,         // DHCountsFetcher
			campaignsService, // DHApproveService
			dhClient,         // DHMatchConfirmer
		)
		logger.Info(ctx, "DH handler initialized")
	}

	// Create sell sheet items handler (always available when auth is configured)
	sellSheetItemsHandler := handlers.NewSellSheetItemsHandler(campaignsRepo, logger)

	// Create card catalog handler (CL card catalog search; nil when CL is not configured)
	var cardCatalogHandler *handlers.CardCatalogHandler
	if clClient != nil && clClient.Available() {
		cardCatalogHandler = handlers.NewCardCatalogHandler(clClient, logger)
	}

	schedulerResult, cancelScheduler := initializeSchedulers(ctx, schedulerDeps{
		Config:               cfg,
		Logger:               logger,
		DBTracker:            priceRepo,
		RefreshCandidates:    refreshCandidateRepo,
		PriceProvImpl:        priceProvImpl,
		CardProvImpl:         cardProvImpl,
		AuthService:          authService,
		SyncStateRepo:        syncStateRepo,
		CardIDMappingRepo:    cardIDMappingRepo,
		CampaignsRepo:        campaignsRepo,
		CampaignsService:     campaignsService,
		AdvisorService:       advisorService,
		AdvisorCacheRepo:     advisorCacheRepo,
		AICallRepo:           aiCallRepo,
		SocialService:        socialService,
		IGTokenRefresher:     igTokenRefresher,
		MetricsPostLister:    metricsRepo,
		MetricsSaver:         metricsRepo,
		InsightsPoller:       insightsPoller,
		PicksService:         picksService,
		CardLadderClient:     clClient,
		CardLadderStore:      clStore,
		CardLadderSalesStore: clSalesStore,
		MMClient:             mmClient,
		MMStore:              mmStore,
		DHClient:             dhClient,
		DHIntelligenceRepo:   intelRepo,
		DHSuggestionsRepo:    suggestionsRepo,
		GapStore:             gapStore,
	})

	// Wire Card Ladder manual refresh into the handler
	if clHandler != nil && schedulerResult.CardLadderRefresh != nil {
		clHandler.SetRefresher(schedulerResult.CardLadderRefresh)
	}
	if clHandler != nil && campaignsRepo != nil {
		clHandler.SetPurchaseLister(campaignsRepo)
	}

	// Wire Market Movers manual refresh into the handler
	if mmHandler != nil && schedulerResult.MMRefresh != nil {
		mmHandler.SetRefresher(schedulerResult.MMRefresh)
	}

	// Create price hints handler
	priceHintsHandler := handlers.NewPriceHintsHandler(cardIDMappingRepo, logger)

	// Create pricing diagnostics handler
	pricingDiagRepo := sqlite.NewPricingDiagnosticsRepository(db.DB)
	pricingDiagHandler := handlers.NewPricingDiagnosticsHandler(pricingDiagRepo, logger)

	// Create card request handler (read-only)
	cardRequestHandler := handlers.NewCardRequestHandlers(cardRequestRepo, nil, "", logger)

	// Create advisor handler (if advisor was initialized above)
	var advisorHandler *handlers.AdvisorHandler
	if advisorService != nil {
		advisorHandler = handlers.NewAdvisorHandler(advisorService, campaignsService, advisorCacheRepo, logger)
	}

	// Create social handler
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
	socialHandler := handlers.NewSocialHandler(socialService, socialRepo, logger, mediaDir, baseURL)

	// Wire metrics repository into social handler for API endpoints
	socialHandler.WithMetricsRepo(metricsRepo)

	// Create AI status handler — only wire tracker when an LLM provider is configured
	var aiTracker ai.AICallTracker
	if azureAIClient != nil {
		aiTracker = aiCallRepo
	}
	aiStatusHandler := handlers.NewAIStatusHandler(aiTracker, logger)

	// Create price flags handler
	priceFlagsHandler := handlers.NewPriceFlagsHandler(campaignsService, logger)

	// Create Instagram handler (if client + store were initialized)
	var igHandler *handlers.InstagramHandler
	if igClient != nil && igStore != nil && authService != nil {
		igHandler = handlers.NewInstagramHandler(igClient, igStore, socialService, authService, logger)
	}

	// Start web server
	deps := ServerDependencies{
		Config:                    cfg,
		Logger:                    logger,
		CardProv:                  cardProvImpl,
		PriceProv:                 priceProvImpl,
		HealthChecker:             priceRepo,
		APITracker:                priceRepo,
		AuthService:               authService,
		FavoritesService:          favoritesService,
		CampaignsService:          campaignsService,
		CacheStatsProvider:        cardProvImpl,
		PriceHintsHandler:         priceHintsHandler,
		CardRequestHandler:        cardRequestHandler,
		PricingDiagnosticsHandler: pricingDiagHandler,
		CampaignsRepo:             campaignsRepo,
		PricingAPIKey:             cfg.Adapters.PricingAPIKey,
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
	// Nil-safe interface conversion: a nil *dh.Client assigned to an interface
	// produces a non-nil interface wrapping a nil pointer, which breaks nil checks.
	if dhClient != nil {
		deps.DHInventoryLister = dhClient
		deps.DHCertResolver = dhClient
		deps.DHInventoryPusher = dhClient
	}
	if campaignsRepo != nil {
		deps.DHFieldsUpdater = campaignsRepo
		deps.DHPushStatusUpdater = campaignsRepo
		deps.DHCandidatesSaver = campaignsRepo
	}
	if cardIDMappingRepo != nil {
		deps.DHCardIDSaver = cardIDMappingRepo
	}
	serverErr := startWebServer(ctx, deps)

	// Graceful scheduler shutdown
	logger.Info(ctx, "shutting down schedulers")
	cancelScheduler()
	schedulerResult.Group.StopAll()

	waitDone := make(chan struct{})
	go func() {
		schedulerResult.Group.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		// Schedulers shut down cleanly
	case <-time.After(30 * time.Second):
		logger.Warn(ctx, "scheduler shutdown timed out after 30s")
	}

	// Wait for any in-flight background DH bulk match to finish
	if dhHandler != nil {
		dhHandler.Wait()
	}

	// Wait for any in-flight background advisor analyses to finish
	if advisorHandler != nil {
		advisorHandler.Wait()
	}

	// Wait for in-flight social caption generation goroutines
	socialService.Wait()

	// Shut down campaign service background workers
	if campaignsService != nil {
		campaignsService.Close()
	}

	return serverErr
}

func initializeCache(cachePath string) cache.Cache {
	if cachePath == "" {
		return nil
	}

	appCache, err := cache.New(cache.Config{
		Type:     "file",
		FilePath: cachePath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Cache initialization failed: %v (path: %s)\n", err, cachePath)
		return nil
	}
	return appCache
}

func resolveDatabasePath(configuredPath string) (string, error) {
	if configuredPath == "" {
		return "", fmt.Errorf("database path cannot be empty")
	}

	if filepath.IsAbs(configuredPath) {
		return filepath.Clean(configuredPath), nil
	}

	absPath, err := filepath.Abs(configuredPath)
	if err != nil {
		return "", fmt.Errorf("resolve path %q to absolute: %w", configuredPath, err)
	}

	return absPath, nil
}
