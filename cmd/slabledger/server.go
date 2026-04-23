package main

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"

	// Domain interfaces (what we depend on - Dependency Inversion Principle)
	domainArbitrage "github.com/guarzo/slabledger/internal/domain/arbitrage"
	domainAuth "github.com/guarzo/slabledger/internal/domain/auth"
	domainDHListing "github.com/guarzo/slabledger/internal/domain/dhlisting"
	domainDHPricing "github.com/guarzo/slabledger/internal/domain/dhpricing"
	domainExport "github.com/guarzo/slabledger/internal/domain/export"
	domainFinance "github.com/guarzo/slabledger/internal/domain/finance"
	domainCampaigns "github.com/guarzo/slabledger/internal/domain/inventory"
	domainPortfolio "github.com/guarzo/slabledger/internal/domain/portfolio"
	domainPricing "github.com/guarzo/slabledger/internal/domain/pricing"
	domainTuning "github.com/guarzo/slabledger/internal/domain/tuning"
)

// ServerDependencies bundles all dependencies required by startWebServer.
type ServerDependencies struct {
	Config                    *config.Config
	Logger                    observability.Logger
	PriceProv                 domainPricing.PriceProvider
	HealthChecker             domainPricing.HealthChecker
	APITracker                domainPricing.APITracker
	AuthService               domainAuth.Service
	CampaignsService          domainCampaigns.Service
	ArbitrageService          domainArbitrage.Service
	PortfolioService          domainPortfolio.Service
	TuningService             domainTuning.Service
	PriceHintsHandler         *handlers.PriceHintsHandler
	PricingDiagnosticsHandler *handlers.PricingDiagnosticsHandler
	CampaignsRepo             handlers.CertPriceLookup         // For pricing API (cert price lookup)
	PricingAPIKey             string                           // Bearer token; empty = pricing API disabled
	AdvisorHandler            *handlers.AdvisorHandler         // AI advisor; nil = disabled
	AIStatusHandler           *handlers.AIStatusHandler        // AI usage stats; nil = disabled
	PriceFlagsHandler         *handlers.PriceFlagsHandler      // Price flag admin; nil = disabled
	CardLadderHandler         *handlers.CardLadderHandler      // Card Ladder admin; nil = disabled
	MarketMoversHandler       *handlers.MarketMoversHandler    // Market Movers admin; nil = disabled
	PSASyncHandler            *handlers.PSASyncHandler         // PSA pending items + admin status; nil = disabled
	OpportunitiesHandler      *handlers.OpportunitiesHandler   // Arbitrage opportunities; nil = disabled
	DHHandler                 *handlers.DHHandler              // DH bulk match + intelligence; nil = disabled
	DHReconcileHandler        *handlers.DHReconcileHandler     // Admin DH reconcile trigger; nil = disabled
	DHListingService          domainDHListing.Service          // optional: orchestrates DH listing after cert import
	DHPriceSyncService        domainDHPricing.Service          // optional: async DH price re-sync on reviewed-price edits
	ExportService             domainExport.Service             // optional: sell sheet and eBay export
	FinanceService            domainFinance.Service            // optional: finance operations
	SellSheetItemsHandler     *handlers.SellSheetItemsHandler  // Sell sheet persistence; nil = disabled
	CardCatalogHandler        *handlers.CardCatalogHandler     // CL card catalog search; nil = disabled
	NichesHandler             *handlers.NichesHandler          // DH niche-opportunity leaderboard; nil = disabled
	CampaignSignalsHandler    *handlers.CampaignSignalsHandler // DH campaign signals; nil = disabled
	LiquidationHandler        *handlers.LiquidationHandler     // Liquidation pricing; nil = disabled
	InsightsHandler           *handlers.InsightsHandler        // Insights overview; nil = disabled
	SheetFetcher              handlers.SheetFetcher            // optional: Google Sheets fetcher for PSA sync
	SheetsSpreadsheetID       string                           // Google Sheets spreadsheet ID
	SheetsTabName             string                           // Google Sheets tab name
}

// EnvVarValidation holds the result of environment variable validation
type EnvVarValidation struct {
	MissingRequired []string
	MissingOptional []string
	Warnings        []string
}

// validateEnvironmentVariables checks required and optional configuration values.
// All values are read from the centralized config struct rather than re-reading
// environment variables directly.
func validateEnvironmentVariables(ctx context.Context, logger observability.Logger, cfg *config.Config) EnvVarValidation {
	result := EnvVarValidation{}

	// Check optional adapter keys via config struct
	type optionalCheck struct {
		name        string
		value       string
		description string
	}
	optionalVars := []optionalCheck{
		{"ENCRYPTION_KEY", cfg.Auth.EncryptionKey, "Enables user authentication and secure session storage"},
	}

	// Google OAuth is loaded separately via LoadGoogleOAuthConfig; check here for user guidance only
	googleConfig := config.LoadGoogleOAuthConfig()
	optionalVars = append(optionalVars,
		optionalCheck{"GOOGLE_CLIENT_ID", googleConfig.ClientID, "Enables Google OAuth for user authentication"},
		optionalCheck{"GOOGLE_CLIENT_SECRET", googleConfig.ClientSecret, "Google OAuth client secret"},
	)

	for _, ov := range optionalVars {
		if ov.value == "" {
			result.MissingOptional = append(result.MissingOptional, ov.name)
			logger.Debug(ctx, "Optional configuration not set",
				observability.String("variable", ov.name),
				observability.String("description", ov.description))
		}
	}

	if cfg.Auth.EncryptionKey != "" && googleConfig.ClientID == "" {
		result.Warnings = append(result.Warnings,
			"ENCRYPTION_KEY is set but GOOGLE_CLIENT_ID is missing. Authentication will not work without Google OAuth.")
	}

	if len(result.MissingRequired) > 0 {
		logger.Error(ctx, "Server cannot start due to missing required configuration",
			observability.Int("missing_count", len(result.MissingRequired)))
	}

	if len(result.MissingOptional) > 0 {
		logger.Info(ctx, "Some optional features are disabled due to missing configuration",
			observability.Int("disabled_features", len(result.MissingOptional)))
	}

	for _, warning := range result.Warnings {
		logger.Warn(ctx, warning)
	}

	return result
}

// startWebServer initializes and starts the web server.
// Returns an error if ListenAndServe fails (not including graceful shutdown).
func startWebServer(ctx context.Context, deps ServerDependencies) error {
	cfg := deps.Config
	logger := deps.Logger

	// Create rate limiter with configuration
	rateLimiter := middleware.NewRateLimiter(cfg.Mode.RateLimitRequests, time.Minute, cfg.Mode.TrustProxy, logger)

	// Create timing store for observability
	timingStore := middleware.NewTimingStore(httpserver.TrackedEndpoints)

	// Create handlers with domain interfaces
	var handlerOpts []handlers.HandlerOption
	if deps.PriceProv != nil {
		handlerOpts = append(handlerOpts, handlers.WithPriceProvider(deps.PriceProv))
	}
	handler := handlers.NewHandler(
		logger,
		handlerOpts...,
	)

	healthHandler := handlers.NewHealthHandler(
		deps.HealthChecker,
		deps.PriceProv,
		logger,
	)

	spaHandler := handlers.NewSPAHandler(logger)

	// Create API status handler (returns empty data when tracker is nil)
	apiStatusHandler := handlers.NewAPIStatusHandler(deps.APITracker, logger)

	// Create campaigns handler if service is available
	var campaignsHandler *handlers.CampaignsHandler
	if deps.CampaignsService != nil {
		var opts []handlers.CampaignsHandlerOption
		if deps.DHListingService != nil {
			opts = append(opts, handlers.WithDHListingService(deps.DHListingService))
		}
		if deps.DHPriceSyncService != nil {
			opts = append(opts, handlers.WithDHPriceSyncer(dhPriceSyncerAdapter{inner: deps.DHPriceSyncService}))
		}
		if deps.FinanceService != nil {
			opts = append(opts, handlers.WithFinanceService(deps.FinanceService))
		}
		if deps.ExportService != nil {
			opts = append(opts, handlers.WithExportService(deps.ExportService))
		}
		if deps.SheetFetcher != nil && deps.SheetsSpreadsheetID != "" {
			opts = append(opts, handlers.WithSheetFetcher(deps.SheetFetcher, deps.SheetsSpreadsheetID, deps.SheetsTabName))
		}
		campaignsHandler = handlers.NewCampaignsHandler(
			deps.CampaignsService,
			deps.ArbitrageService,
			deps.PortfolioService,
			deps.TuningService,
			logger,
			ctx,
			opts...,
		)
		logger.Info(ctx, "Campaigns handler initialized")
	}

	// Create router and setup routes
	router := httpserver.NewRouter(httpserver.RouterConfig{
		Handler:                   handler,
		HealthHandler:             healthHandler,
		APIStatusHandler:          apiStatusHandler,
		SPAHandler:                spaHandler,
		AuthService:               deps.AuthService,
		CampaignsHandler:          campaignsHandler,
		CampaignsService:          deps.CampaignsService,
		PriceHintsHandler:         deps.PriceHintsHandler,
		PricingDiagnosticsHandler: deps.PricingDiagnosticsHandler,
		PricingAPIKey:             deps.PricingAPIKey,
		CampaignsRepo:             deps.CampaignsRepo,
		AdvisorHandler:            deps.AdvisorHandler,
		AIStatusHandler:           deps.AIStatusHandler,
		PriceFlagsHandler:         deps.PriceFlagsHandler,
		CardLadderHandler:         deps.CardLadderHandler,
		MarketMoversHandler:       deps.MarketMoversHandler,
		PSASyncHandler:            deps.PSASyncHandler,
		OpportunitiesHandler:      deps.OpportunitiesHandler,
		DHHandler:                 deps.DHHandler,
		DHReconcileHandler:        deps.DHReconcileHandler,
		SellSheetItemsHandler:     deps.SellSheetItemsHandler,
		CardCatalogHandler:        deps.CardCatalogHandler,
		NichesHandler:             deps.NichesHandler,
		CampaignSignalsHandler:    deps.CampaignSignalsHandler,
		LiquidationHandler:        deps.LiquidationHandler,
		InsightsHandler:           deps.InsightsHandler,
		Logger:                    logger,
		AdminEmails:               cfg.Auth.AdminEmails,
		DatabasePath:              "",
		TimingStore:               timingStore,
		GoogleOAuthEnv:            cfg.Adapters.GoogleOAuthEnv,
		LocalAPIToken:             cfg.Adapters.LocalAPIToken,
	})
	mux := router.Setup()

	// Apply middleware stack
	httpHandler := httpserver.ApplyMiddleware(mux, rateLimiter, logger, timingStore)

	// Start server
	addr := cfg.Server.ListenAddr
	if addr == "" {
		addr = fmt.Sprintf("0.0.0.0:%d", cfg.Mode.WebPort)
	}
	srv := &http.Server{
		Addr:         addr,
		Handler:      httpHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	panicShutdownChan := make(chan struct{})
	var shutdownOnce sync.Once
	closePanic := func() { shutdownOnce.Do(func() { close(panicShutdownChan) }) }

	var serverErr error
	var serverErrMu sync.Mutex

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(ctx, "server panic recovered",
					observability.String("panic", fmt.Sprintf("%v", r)),
					observability.String("stack", string(debug.Stack())))
				serverErrMu.Lock()
				serverErr = fmt.Errorf("server panic: %v", r)
				serverErrMu.Unlock()
				closePanic()
			}
		}()

		logger.Info(ctx, "Starting web server",
			observability.String("addr", addr),
			observability.String("url", fmt.Sprintf("http://%s", addr)))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "Server failed", observability.Err(err))
			serverErrMu.Lock()
			serverErr = err
			serverErrMu.Unlock()
			closePanic()
		}
	}()

	select {
	case <-ctx.Done():
	case <-panicShutdownChan:
		logger.Warn(ctx, "Initiating shutdown due to server error or panic")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn(shutdownCtx, "Server shutdown error", observability.Err(err))
	}

	// Wait for background handler goroutines (e.g. card discovery) to finish
	// before the caller closes the database.
	if campaignsHandler != nil {
		campaignsHandler.WaitBackground()
	}

	rateLimiter.Close()
	logger.Info(context.Background(), "Server stopped")

	serverErrMu.Lock()
	defer serverErrMu.Unlock()
	return serverErr
}

// dhPriceSyncerAdapter bridges the handler-layer DHPriceSyncer interface
// (void return) to the domain dhpricing.Service (returns SyncResult). The
// handler fires this in a background goroutine and does not use the result;
// failures are logged inside the service.
type dhPriceSyncerAdapter struct{ inner domainDHPricing.Service }

func (a dhPriceSyncerAdapter) SyncPurchasePrice(ctx context.Context, purchaseID string) {
	_ = a.inner.SyncPurchasePrice(ctx, purchaseID)
}
