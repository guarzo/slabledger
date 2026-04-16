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

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/joho/godotenv"

	// Concrete implementations (only imported in main for wiring - Hexagonal Architecture)
	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/google"
	"github.com/guarzo/slabledger/internal/adapters/clients/gsheets"
	scoringadapter "github.com/guarzo/slabledger/internal/adapters/scoring"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/auth"
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
	demandRepo := sqlite.NewDHDemandRepository(db.DB)

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

	campaignsInit := initializeCampaignsService(
		ctx, cfg, logger, db, priceProvImpl, intelRepo, mmStore, dhClient,
	)
	campaignsService := campaignsInit.service
	certLookup := campaignsInit.certLookup
	arbSvc := campaignsInit.arbSvc
	portSvc := campaignsInit.portSvc
	tuningSvc := campaignsInit.tuningSvc
	financeService := campaignsInit.financeService
	exportService := campaignsInit.exportService

	// Sync state repository (for delta poll timestamps)
	syncStateRepo := sqlite.NewSyncStateRepository(db.DB)

	// AI call tracking
	aiCallRepo := sqlite.NewAICallRepository(db)

	// Build advisor tool options — inject intelligence repos.
	gapStore := sqlite.NewGapStore(db.DB)
	advisorToolOpts := []advisortool.ExecutorOption{
		advisortool.WithIntelligenceRepo(intelRepo),
		advisortool.WithSuggestionsRepo(suggestionsRepo),
		advisortool.WithGapStore(gapStore),
		advisortool.WithArbitrageService(arbSvc),
		advisortool.WithPortfolioService(portSvc),
		advisortool.WithTuningService(tuningSvc),
		advisortool.WithFinanceService(financeService),
		advisortool.WithExportService(exportService),
	}

	azureAIClient, advisorService, advisorCacheRepo, err := initializeAdvisorService(
		ctx, cfg, logger, db, aiCallRepo, campaignsService,
		[]scoringadapter.ProviderOption{
			scoringadapter.WithArbitrageService(arbSvc),
			scoringadapter.WithPortfolioService(portSvc),
			scoringadapter.WithTuningService(tuningSvc),
		},
		advisorToolOpts...,
	)
	if err != nil {
		return err
	}

	// Initialize Card Ladder (encryptor was created earlier for MM mapping adapter)
	clClient, _, clStore := initializeCardLadder(ctx, logger, db, clEncryptor)
	var clSalesStore *sqlite.CLSalesStore
	if clStore != nil {
		clSalesStore = sqlite.NewCLSalesStore(db.DB)
	}

	// Initialize Market Movers client (store was created earlier for campaigns service)
	mmClient, _ := initializeMarketMovers(ctx, logger, db, clEncryptor)

	// Initialize Google Sheets client for PSA sync (nil if not configured)
	var gsheetsClient *gsheets.Client
	if cfg.GoogleSheets.CredentialsJSON != "" {
		var err error
		gsheetsClient, err = gsheets.New(cfg.GoogleSheets.CredentialsJSON, logger)
		if err != nil {
			logger.Error(ctx, "failed to initialize Google Sheets client", observability.Err(err))
		} else {
			logger.Info(ctx, "Google Sheets client initialized")
		}
	}

	sDeps := schedulerDeps{
		Config:               cfg,
		Logger:               logger,
		DBTracker:            priceRepo,
		RefreshCandidates:    refreshCandidateRepo,
		PriceProvImpl:        priceProvImpl,
		AuthService:          authService,
		SyncStateRepo:        syncStateRepo,
		CardIDMappingRepo:    cardIDMappingRepo,
		CampaignStore:        campaignsInit.campaignStore,
		PurchaseStore:        campaignsInit.purchaseStore,
		DHStore:              campaignsInit.dhStore,
		CampaignsService:     campaignsService,
		CertLookup:           certLookup,
		CertEnrichJob:        campaignsInit.certEnrichJob,
		AdvisorService:       advisorService,
		AdvisorCacheRepo:     advisorCacheRepo,
		AICallRepo:           aiCallRepo,
		CardLadderClient:     clClient,
		CardLadderStore:      clStore,
		CardLadderSalesStore: clSalesStore,
		MMClient:             mmClient,
		MMStore:              mmStore,
		DHClient:             dhClient,
		DHIntelligenceRepo:   intelRepo,
		DHSuggestionsRepo:    suggestionsRepo,
		DHDemandRepo:         demandRepo,
		GapStore:             gapStore,
		PSASpreadsheetID:     cfg.GoogleSheets.SpreadsheetID,
		PSATabName:           cfg.GoogleSheets.TabName,
	}
	if gsheetsClient != nil {
		sDeps.PSASheetFetcher = gsheetsClient
	}
	schedulerResult, cancelScheduler := initializeSchedulers(ctx, sDeps)

	// Create pending items repository for PSA sync handler.
	pendingItemsRepo := sqlite.NewPendingItemsRepository(db.DB)

	deps, hOut := createHandlers(ctx, handlerInputs{
		Cfg:               cfg,
		Logger:            logger,
		DB:                db,
		PriceProvImpl:     priceProvImpl,
		PriceRepo:         priceRepo,
		AuthService:       authService,
		CampaignsService:  campaignsService,
		ArbitrageService:  arbSvc,
		PortfolioService:  portSvc,
		TuningService:     tuningSvc,
		FinanceService:    financeService,
		ExportService:     exportService,
		PurchaseStore:     campaignsInit.purchaseStore,
		SellSheetStore:    campaignsInit.sellSheetStore,
		CardIDMappingRepo: cardIDMappingRepo,
		IntelRepo:         intelRepo,
		SuggestionsRepo:   suggestionsRepo,
		DemandRepo:        demandRepo,
		AdvisorService:    advisorService,
		AdvisorCacheRepo:  advisorCacheRepo,
		AzureAIClient:     azureAIClient,
		AICallRepo:        aiCallRepo,
		CLClient:          clClient,
		CLStore:           clStore,
		MMStore:           mmStore,
		MMClient:          mmClient,
		DHClient:          dhClient,
		SchedulerResult:   schedulerResult,
		GSheetsClient:     gsheetsClient,
		PendingItemsRepo:  pendingItemsRepo,
	})
	serverErr := startWebServer(ctx, deps)

	shutdownGracefully(ctx, logger, cancelScheduler, schedulerResult, hOut, campaignsService, cfg.Server.SchedulerShutdownTimeout)

	return serverErr
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
