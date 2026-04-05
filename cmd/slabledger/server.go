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
	domainAuth "github.com/guarzo/slabledger/internal/domain/auth"
	domainCampaigns "github.com/guarzo/slabledger/internal/domain/campaigns"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	domainFavorites "github.com/guarzo/slabledger/internal/domain/favorites"
	domainPricing "github.com/guarzo/slabledger/internal/domain/pricing"
)

// ServerDependencies bundles all dependencies required by startWebServer.
type ServerDependencies struct {
	Config                    *config.Config
	Logger                    observability.Logger
	CardProv                  domainCards.CardProvider
	PriceProv                 domainPricing.PriceProvider
	HealthChecker             domainPricing.HealthChecker
	APITracker                domainPricing.APITracker
	AuthService               domainAuth.Service
	FavoritesService          domainFavorites.Service
	CampaignsService          domainCampaigns.Service
	CacheStatsProvider        handlers.CacheStatsProvider
	CardDiscoverer            handlers.CardDiscoverer // optional: triggers CardHedger discovery after imports
	PriceHintsHandler         *handlers.PriceHintsHandler
	CardHedgerStats           handlers.CardHedgerStats // optional: live CardHedger counters
	CardRequestHandler        *handlers.CardRequestHandlers
	PricingDiagnosticsHandler *handlers.PricingDiagnosticsHandler
	CampaignsRepo             domainCampaigns.Repository      // For pricing API (cert price lookup)
	PricingAPIKey             string                          // Bearer token; empty = pricing API disabled
	AdvisorHandler            *handlers.AdvisorHandler        // AI advisor; nil = disabled
	SocialHandler             *handlers.SocialHandler         // Social content; nil = disabled
	InstagramHandler          *handlers.InstagramHandler      // Instagram publishing; nil = disabled
	AIStatusHandler           *handlers.AIStatusHandler       // AI usage stats; nil = disabled
	PriceFlagsHandler         *handlers.PriceFlagsHandler     // Price flag admin; nil = disabled
	CardLadderHandler         *handlers.CardLadderHandler     // Card Ladder admin; nil = disabled
	SalesCompsHandler         *handlers.SalesCompsHandler     // Sales comps; nil = disabled
	PicksHandler              *handlers.PicksHandler          // AI picks; nil = disabled
	OpportunitiesHandler      *handlers.OpportunitiesHandler  // Arbitrage opportunities; nil = disabled
	DHHandler                 *handlers.DHHandler             // DH bulk match + intelligence; nil = disabled
	SellSheetItemsHandler     *handlers.SellSheetItemsHandler // Sell sheet persistence; nil = disabled
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

	// Check required adapter keys via config struct
	type requiredCheck struct {
		name        string
		value       string
		description string
	}
	requiredVars := []requiredCheck{
		{"PRICECHARTING_TOKEN", cfg.Adapters.PriceChartingToken, "Required for graded card pricing data. Get your API token from pricecharting.com"},
	}

	for _, rv := range requiredVars {
		if rv.value == "" {
			result.MissingRequired = append(result.MissingRequired, rv.name)
			logger.Error(ctx, "Missing required configuration",
				observability.String("variable", rv.name),
				observability.String("description", rv.description))
		}
	}

	// Check optional adapter keys via config struct
	type optionalCheck struct {
		name        string
		value       string
		description string
	}
	optionalVars := []optionalCheck{
		{"CARD_HEDGER_API_KEY", cfg.Adapters.CardHedgerKey, "Enables CardHedger as supplementary pricing source"},
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

	// Create card search service
	searchService := domainCards.NewSearchService(deps.CardProv)

	// Create handlers with domain interfaces
	var handlerOpts []handlers.HandlerOption
	if deps.PriceProv != nil {
		handlerOpts = append(handlerOpts, handlers.WithPriceProvider(deps.PriceProv))
	}
	handler := handlers.NewHandler(
		deps.CardProv,
		searchService,
		logger,
		handlerOpts...,
	)

	healthHandler := handlers.NewHealthHandler(
		deps.HealthChecker,
		deps.CardProv,
		deps.PriceProv,
		logger,
	)

	spaHandler := handlers.NewSPAHandler(logger)

	// Create API status handler (returns empty data when tracker is nil)
	apiStatusHandler := handlers.NewAPIStatusHandler(deps.APITracker, logger)
	if deps.CardHedgerStats != nil {
		apiStatusHandler.WithCardHedgerStats(deps.CardHedgerStats)
	}

	// Create cache status handler
	var cacheStatusHandler *handlers.CacheStatusHandler
	if deps.CacheStatsProvider != nil {
		cacheStatusHandler = handlers.NewCacheStatusHandler(deps.CacheStatsProvider, logger)
	}

	// Create campaigns handler if service is available
	var campaignsHandler *handlers.CampaignsHandler
	if deps.CampaignsService != nil {
		campaignsHandler = handlers.NewCampaignsHandler(deps.CampaignsService, logger, deps.CardDiscoverer, ctx)
		logger.Info(ctx, "Campaigns handler initialized")
	}

	// Create router and setup routes
	router := httpserver.NewRouter(httpserver.RouterConfig{
		Handler:                   handler,
		HealthHandler:             healthHandler,
		APIStatusHandler:          apiStatusHandler,
		CacheStatusHandler:        cacheStatusHandler,
		SPAHandler:                spaHandler,
		AuthService:               deps.AuthService,
		FavoritesService:          deps.FavoritesService,
		CampaignsHandler:          campaignsHandler,
		CampaignsService:          deps.CampaignsService,
		PriceHintsHandler:         deps.PriceHintsHandler,
		CardRequestHandler:        deps.CardRequestHandler,
		PricingDiagnosticsHandler: deps.PricingDiagnosticsHandler,
		PricingAPIKey:             deps.PricingAPIKey,
		CampaignsRepo:             deps.CampaignsRepo,
		AdvisorHandler:            deps.AdvisorHandler,
		SocialHandler:             deps.SocialHandler,
		InstagramHandler:          deps.InstagramHandler,
		AIStatusHandler:           deps.AIStatusHandler,
		PriceFlagsHandler:         deps.PriceFlagsHandler,
		CardLadderHandler:         deps.CardLadderHandler,
		SalesCompsHandler:         deps.SalesCompsHandler,
		PicksHandler:              deps.PicksHandler,
		OpportunitiesHandler:      deps.OpportunitiesHandler,
		DHHandler:                 deps.DHHandler,
		SellSheetItemsHandler:     deps.SellSheetItemsHandler,
		Logger:                    logger,
		AdminEmails:               cfg.Auth.AdminEmails,
		DatabasePath:              cfg.Database.Path,
		TimingStore:               timingStore,
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
