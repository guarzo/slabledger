package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/joho/godotenv"

	// Concrete implementations (only imported in main for wiring - Hexagonal Architecture)
	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/azureai"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/fusionprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/google"
	igclient "github.com/guarzo/slabledger/internal/adapters/clients/instagram"
	"github.com/guarzo/slabledger/internal/adapters/clients/pokemonprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricelookup"
	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/favorites"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/crypto"

	"github.com/google/uuid"
)

// favoritesListAdapter adapts sqlite.FavoritesRepository to the scheduler.FavoritesLister interface.
type favoritesListAdapter struct {
	repo *sqlite.FavoritesRepository
}

func (a *favoritesListAdapter) ListAllDistinctCards(ctx context.Context) ([]scheduler.FavoriteCard, error) {
	cards, err := a.repo.ListAllDistinctCards(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.FavoriteCard, len(cards))
	for i, c := range cards {
		result[i] = scheduler.FavoriteCard{
			CardName:   c.CardName,
			SetName:    c.SetName,
			CardNumber: c.CardNumber,
		}
	}
	return result, nil
}

// inventoryListAdapter adapts sqlite.CampaignsRepository to the scheduler.InventoryLister interface.
type inventoryListAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *inventoryListAdapter) ListUnsoldInventory(ctx context.Context) ([]scheduler.InventoryPurchase, error) {
	purchases, err := a.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.InventoryPurchase, len(purchases))
	for i, p := range purchases {
		result[i] = scheduler.InventoryPurchase{
			ID:              p.ID,
			CardName:        p.CardName,
			CardNumber:      p.CardNumber,
			SetName:         p.SetName,
			GradeValue:      p.GradeValue,
			Grader:          p.Grader,
			BuyCostCents:    p.BuyCostCents,
			CLValueCents:    p.CLValueCents,
			PSAListingTitle: p.PSAListingTitle,
			SnapshotDate:    p.SnapshotDate,
		}
	}
	return result, nil
}

// snapshotRefreshAdapter adapts campaigns.Service to the scheduler.SnapshotRefresher interface.
type snapshotRefreshAdapter struct {
	svc campaigns.Service
}

func (a *snapshotRefreshAdapter) RefreshSnapshot(ctx context.Context, p scheduler.InventoryPurchase) bool {
	return a.svc.RefreshPurchaseSnapshot(ctx, p.ID, campaigns.CardIdentity{
		CardName: p.CardName, CardNumber: p.CardNumber, SetName: p.SetName, PSAListingTitle: p.PSAListingTitle,
	}, p.GradeValue, p.CLValueCents)
}

// campaignCardListAdapter adapts sqlite.CampaignsRepository to the scheduler.CampaignCardLister interface.
type campaignCardListAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *campaignCardListAdapter) ListUnsoldCards(ctx context.Context) ([]scheduler.UnsoldCard, error) {
	infos, err := a.repo.ListUnsoldCards(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.UnsoldCard, len(infos))
	for i, info := range infos {
		result[i] = scheduler.UnsoldCard{CardName: info.CardName, SetName: info.SetName, CardNumber: info.CardNumber}
	}
	return result, nil
}

// cardIDMappingListAdapter adapts sqlite.CardIDMappingRepository to the scheduler.CardIDMappingLister interface.
type cardIDMappingListAdapter struct {
	repo *sqlite.CardIDMappingRepository
}

func (a *cardIDMappingListAdapter) ListByProvider(ctx context.Context, provider string) ([]scheduler.CardIDMapping, error) {
	mappings, err := a.repo.ListByProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.CardIDMapping, len(mappings))
	for i, m := range mappings {
		result[i] = scheduler.CardIDMapping{
			CardName:        m.CardName,
			SetName:         m.SetName,
			CollectorNumber: m.CollectorNumber,
			ExternalID:      m.ExternalID,
		}
	}
	return result, nil
}

// --- PSA image backfill adapters ---

type psaImageListerAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *psaImageListerAdapter) ListPurchasesMissingImages(ctx context.Context, limit int) ([]psa.PurchaseImageRow, error) {
	rows, err := a.repo.ListPurchasesMissingImages(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]psa.PurchaseImageRow, len(rows))
	for i, r := range rows {
		result[i] = psa.PurchaseImageRow{ID: r.ID, CertNumber: r.CertNumber}
	}
	return result, nil
}

type psaImageUpdaterAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *psaImageUpdaterAdapter) UpdatePurchaseImageURLs(ctx context.Context, id, frontURL, backURL string) error {
	return a.repo.UpdatePurchaseImageURLs(ctx, id, frontURL, backURL)
}

// newCardDiscovererAdapter returns the scheduler.CardDiscoverer directly as a
// handlers.CardDiscoverer. Both interfaces now use campaigns.CardIdentity, so no
// conversion is needed — the scheduler type satisfies the handler interface.
func newCardDiscovererAdapter(d scheduler.CardDiscoverer) handlers.CardDiscoverer {
	if d == nil {
		return nil
	}
	return d
}

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

	// Create price repository
	priceRepo := sqlite.NewPriceRepository(db)

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

	// Discovery failure tracker (persists CardHedger discovery failures for diagnostics)
	discoveryFailureRepo := sqlite.NewDiscoveryFailureRepository(db.DB)

	pcProvider, err := pricecharting.NewPriceCharting(
		pricecharting.DefaultConfig(cfg.Adapters.PriceChartingToken), appCache, logger,
		pricecharting.WithHintResolver(cardIDMappingRepo),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize PriceCharting provider: %w", err)
	}
	defer func() {
		if err := pcProvider.Close(); err != nil {
			logger.Warn(ctx, "failed to close PriceCharting provider", observability.Err(err))
		}
	}()

	pokemonPriceClientImpl := pokemonprice.NewClient(cfg.Adapters.PokemonPriceKey, pokemonprice.WithLogger(logger))
	cardHedgerClientImpl := cardhedger.NewClient(cfg.Adapters.CardHedgerKey, cardhedger.WithLogger(logger))

	// Wrap secondary clients in adapters that implement fusion.SecondaryPriceSource
	secondarySources := []fusion.SecondaryPriceSource{
		fusionprice.NewPokemonPriceAdapter(pokemonPriceClientImpl, fusionprice.WithPPLogger(logger)),
		fusionprice.NewCardHedgerAdapter(cardHedgerClientImpl, cardIDMappingRepo, logger,
			fusionprice.WithCardHedgerHintResolver(cardIDMappingRepo)),
	}

	priceProvImpl := fusionprice.NewFusionProviderWithRepo(
		pcProvider, secondarySources,
		appCache, priceRepo, priceRepo, priceRepo, logger,
		fusionprice.DefaultFreshnessDuration,
		cfg.Fusion.CacheTTL,
		cfg.Fusion.PriceChartingTimeout,
		cfg.Fusion.SecondarySourceTimeout,
		fusionprice.WithCardProvider(cardProvImpl),
	)
	logger.Info(ctx, "Fusion price provider initialized with 3 sources",
		observability.Bool("pokemonprice_available", pokemonPriceClientImpl.Available()),
		observability.Bool("cardhedger_available", cardHedgerClientImpl.Available()))

	// Initialize campaigns
	campaignsRepo := sqlite.NewCampaignsRepository(db.DB)
	priceLookupAdapter := pricelookup.NewAdapter(priceProvImpl)
	campaignOpts := []campaigns.ServiceOption{
		campaigns.WithPriceLookup(priceLookupAdapter),
		campaigns.WithIDGenerator(uuid.NewString),
		campaigns.WithMaxSnapshotRetries(cfg.SnapshotEnrich.MaxRetries),
	}

	if cfg.Adapters.PSAToken != "" {
		psaClient := psa.NewClient(cfg.Adapters.PSAToken, logger)
		certAdapter := psa.NewCertAdapter(psaClient)
		campaignOpts = append(campaignOpts, campaigns.WithCertLookup(certAdapter))
		logger.Info(ctx, "PSA cert lookup enabled")
	}

	// Card request repository (tracks certs without linked cards in CardHedger)
	cardRequestRepo := sqlite.NewCardRequestRepository(db.DB)

	// Wire cert→card_id resolver if CardHedger is available
	if cardHedgerClientImpl.Available() {
		certResolverOpts := []cardhedger.CertResolverOption{
			cardhedger.WithMissingCardTracker(cardRequestRepo),
		}
		certResolver := cardhedger.NewCertResolver(cardHedgerClientImpl, cardIDMappingRepo, logger, certResolverOpts...)
		campaignOpts = append(campaignOpts, campaigns.WithCardIDResolver(certResolver))
	}

	// History recorders — track CL values and population changes during CSV imports.
	campaignOpts = append(campaignOpts,
		campaigns.WithCLValueRecorder(campaignsRepo),
		campaigns.WithPopulationRecorder(campaignsRepo),
	)
	campaignsService := campaigns.NewService(campaignsRepo, campaignOpts...)

	// Sync state repository (for delta poll timestamps)
	syncStateRepo := sqlite.NewSyncStateRepository(db.DB)

	// AI call tracking
	aiCallRepo := sqlite.NewAICallRepository(db)

	// Initialize shared Azure AI client (used by advisor + social caption generation)
	var azureAIClient advisor.LLMProvider
	var advisorService advisor.Service
	var advisorCacheRepo *sqlite.AdvisorCacheRepository
	if cfg.Adapters.AzureAIEndpoint != "" && cfg.Adapters.AzureAIKey != "" {
		client, err := azureai.NewClient(azureai.Config{
			Endpoint:       cfg.Adapters.AzureAIEndpoint,
			APIKey:         cfg.Adapters.AzureAIKey,
			DeploymentName: cfg.Adapters.AzureAIDeployment,
		}, azureai.WithLogger(logger))
		if err != nil {
			return fmt.Errorf("initialize azure ai client: %w", err)
		}
		azureAIClient = client

		toolExec := advisortool.NewCampaignToolExecutor(campaignsService)
		advisorCacheRepo = sqlite.NewAdvisorCacheRepository(db.DB, logger)
		advisorService = advisor.NewService(client, toolExec,
			advisor.WithLogger(logger),
			advisor.WithMaxToolRounds(5),
			advisor.WithAITracker(aiCallRepo),
			advisor.WithCacheStore(advisorCacheRepo),
		)
		logger.Info(ctx, "AI advisor initialized",
			observability.String("deployment", cfg.Adapters.AzureAIDeployment))
	}

	mediaDir := os.Getenv("MEDIA_DIR")
	if mediaDir == "" {
		mediaDir = "./data/media"
	}
	baseURL := os.Getenv("BASE_URL")

	// Initialize social content service
	socialRepo := sqlite.NewSocialRepository(db.DB)
	var socialService social.Service
	var socialOpts []social.ServiceOption
	socialOpts = append(socialOpts, social.WithLogger(logger))
	socialOpts = append(socialOpts, social.WithAITracker(aiCallRepo))
	if azureAIClient != nil {
		socialOpts = append(socialOpts, social.WithLLM(azureAIClient))
	}
	// Initialize Instagram integration (requires encryption + Instagram config)
	igConfig := config.LoadInstagramOAuthConfig()
	var igClient *igclient.Client
	var igStore *sqlite.InstagramStore
	var igTokenRefresher scheduler.InstagramTokenRefresher

	if igConfig.IsConfigured() && cfg.Auth.EncryptionKey != "" {
		igEncryptor, igErr := crypto.NewAESEncryptor(cfg.Auth.EncryptionKey)
		if igErr != nil {
			logger.Error(ctx, "Instagram encryption init failed — Instagram integration disabled",
				observability.Err(igErr))
		} else {
			igClient = igclient.NewClient(igConfig.AppID, igConfig.AppSecret, igConfig.RedirectURI, logger)
			igStore = sqlite.NewInstagramStore(db.DB, igEncryptor)

			publisher := igclient.NewPublisherAdapter(igClient)
			tokenProvider := igclient.NewTokenProvider(igStore)
			socialOpts = append(socialOpts, social.WithPublisher(publisher, tokenProvider))

			igTokenRefresher = igclient.NewTokenRefresher(igClient, igStore, logger)
			logger.Info(ctx, "Instagram integration initialized")
		}
	}

	socialService = social.NewService(socialRepo, socialOpts...)

	// Build and start scheduler group
	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	schedulerResult := scheduler.BuildGroup(cfg, scheduler.BuildDeps{
		PriceRepo:               priceRepo,
		APITracker:              priceRepo,
		HealthChecker:           priceRepo,
		AccessTracker:           priceRepo,
		PriceProvider:           priceProvImpl,
		CardProvider:            cardProvImpl,
		AuthService:             authService,
		Logger:                  logger,
		CardHedgerClient:        cardHedgerClientImpl,
		SyncStateStore:          syncStateRepo,
		CardIDMappingLookup:     cardIDMappingRepo,
		CardIDMappingLister:     &cardIDMappingListAdapter{repo: cardIDMappingRepo},
		CardIDMappingSaver:      cardIDMappingRepo,
		DiscoveryFailureTracker: discoveryFailureRepo,
		FavoritesLister:         &favoritesListAdapter{repo: favoritesRepo},
		CampaignCardLister:      &campaignCardListAdapter{repo: campaignsRepo},
		NewSetsProvider:         cardProvImpl.RegistryManager(),
		InventoryLister:         &inventoryListAdapter{repo: campaignsRepo},
		SnapshotRefresher:       &snapshotRefreshAdapter{svc: campaignsService},
		SnapshotEnrichService:   campaignsService,
		SnapshotHistoryLister:   campaignsRepo,
		SnapshotHistoryRecorder: campaignsRepo,
		AdvisorCollector:        advisorService,
		AdvisorCache:            advisorCacheRepo,
		AICallTracker:           aiCallRepo,
		SocialContentDetector:   socialService,
		InstagramTokenRefresher: igTokenRefresher,
	})
	schedulerResult.Group.StartAll(schedulerCtx)

	// Create price hints handler
	priceHintsHandler := handlers.NewPriceHintsHandler(cardIDMappingRepo, logger)

	// Create pricing diagnostics handler
	pricingDiagRepo := sqlite.NewPricingDiagnosticsRepository(db.DB)
	pricingDiagHandler := handlers.NewPricingDiagnosticsHandler(pricingDiagRepo, logger)

	// Create card request handler — listing always works; submit endpoints
	// require both CardHedger availability and a configured client ID.
	var cardReqClient handlers.CardRequester
	cardReqClientID := cfg.Adapters.CardHedgerClientID
	if cardHedgerClientImpl.Available() && cardReqClientID != "" {
		cardReqClient = cardHedgerClientImpl
		logger.Info(ctx, "card request handler initialized")
	} else {
		logger.Info(ctx, "card request handler initialized (read-only)")
	}
	cardRequestHandler := handlers.NewCardRequestHandlers(cardRequestRepo, cardReqClient, cardReqClientID, logger)

	// Create advisor handler (if advisor was initialized above)
	var advisorHandler *handlers.AdvisorHandler
	if advisorService != nil {
		advisorHandler = handlers.NewAdvisorHandler(advisorService, campaignsService, advisorCacheRepo, logger)
	}

	// Create social handler
	socialHandler := handlers.NewSocialHandler(socialService, socialRepo, logger, mediaDir, baseURL)

	// Wire image backfiller if PSA image token is available
	if cfg.Adapters.PSAImageToken != "" {
		psaImageClient := psa.NewClient(cfg.Adapters.PSAImageToken, logger)
		backfiller := psa.NewImageBackfiller(
			psaImageClient,
			&psaImageListerAdapter{repo: campaignsRepo},
			&psaImageUpdaterAdapter{repo: campaignsRepo},
			logger,
		)
		socialHandler.WithBackfiller(backfiller)
		logger.Info(ctx, "PSA image backfill enabled")
	}

	// Create AI status handler — only wire tracker when an LLM provider is configured
	var aiTracker ai.AICallTracker
	if azureAIClient != nil {
		aiTracker = aiCallRepo
	}
	aiStatusHandler := handlers.NewAIStatusHandler(aiTracker, logger)

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
		CardDiscoverer:            newCardDiscovererAdapter(schedulerResult.CardDiscoverer),
		PriceHintsHandler:         priceHintsHandler,
		CardHedgerStats:           cardHedgerClientImpl,
		CardRequestHandler:        cardRequestHandler,
		PricingDiagnosticsHandler: pricingDiagHandler,
		CampaignsRepo:             campaignsRepo,
		PricingAPIKey:             cfg.Adapters.PricingAPIKey,
		AdvisorHandler:            advisorHandler,
		SocialHandler:             socialHandler,
		InstagramHandler:          igHandler,
		AIStatusHandler:           aiStatusHandler,
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

	// Wait for any in-flight background advisor analyses to finish
	if advisorHandler != nil {
		advisorHandler.Wait()
	}

	// Wait for in-flight social caption generation goroutines
	socialService.Wait()

	// Shut down campaign service background workers
	campaignsService.Close()

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
