package httpserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// Router configures all HTTP routes and returns the configured handler
type Router struct {
	handler                   *handlers.Handler
	healthHandler             *handlers.HealthHandler
	apiStatusHandler          *handlers.APIStatusHandler
	spaHandler                *handlers.SPAHandler
	authHandler               *handlers.AuthHandlers
	adminHandler              *handlers.AdminHandlers
	authMW                    *middleware.AuthMiddleware
	authRateLimiter           *middleware.RateLimiter
	campaignsHandler          *handlers.CampaignsHandler
	priceHintsHandler         *handlers.PriceHintsHandler
	pricingDiagnosticsHandler *handlers.PricingDiagnosticsHandler
	pricingAPIHandler         *handlers.PricingAPIHandler
	advisorHandler            *handlers.AdvisorHandler
	insightsHandler           *handlers.InsightsHandler
	aiUsageHandler            *handlers.AIStatusHandler
	priceFlagsHandler         *handlers.PriceFlagsHandler
	cardLadderHandler         *handlers.CardLadderHandler
	marketMoversHandler       *handlers.MarketMoversHandler
	opportunitiesHandler      *handlers.OpportunitiesHandler
	psaExchangeHandler        *handlers.PSAExchangeHandler
	dhHandler                 *handlers.DHHandler
	dhReconcileHandler        *handlers.DHReconcileHandler
	sellSheetItemsHandler     *handlers.SellSheetItemsHandler
	cardCatalogHandler        *handlers.CardCatalogHandler
	psaSyncHandler            *handlers.PSASyncHandler
	nichesHandler             *handlers.NichesHandler
	campaignSignalsHandler    *handlers.CampaignSignalsHandler
	liquidationHandler        *handlers.LiquidationHandler
	pricingAPIKey             string
	logger                    observability.Logger
	databasePath              string
	timingStore               *middleware.TimingStore
	googleOAuthEnv            string
	localAPIToken             string
}

// RouterConfig holds configuration for creating a new Router
type RouterConfig struct {
	Handler                   *handlers.Handler
	HealthHandler             *handlers.HealthHandler
	APIStatusHandler          *handlers.APIStatusHandler
	SPAHandler                *handlers.SPAHandler
	AuthService               auth.Service
	CampaignsHandler          *handlers.CampaignsHandler
	CampaignsService          inventory.Service
	ArbitrageService          arbitrage.Service
	PortfolioService          portfolio.Service
	TuningService             tuning.Service
	PriceHintsHandler         *handlers.PriceHintsHandler
	PricingDiagnosticsHandler *handlers.PricingDiagnosticsHandler
	PricingAPIKey             string                           // Bearer token; empty = pricing API disabled
	CampaignsRepo             handlers.CertPriceLookup         // For pricing API handler
	AdvisorHandler            *handlers.AdvisorHandler         // AI advisor; nil = disabled
	InsightsHandler           *handlers.InsightsHandler        // Insights overview; nil = disabled
	AIStatusHandler           *handlers.AIStatusHandler        // AI usage stats; nil = disabled
	PriceFlagsHandler         *handlers.PriceFlagsHandler      // Price flag admin; nil = disabled
	CardLadderHandler         *handlers.CardLadderHandler      // Card Ladder admin; nil = disabled
	MarketMoversHandler       *handlers.MarketMoversHandler    // Market Movers admin; nil = disabled
	OpportunitiesHandler      *handlers.OpportunitiesHandler   // Arbitrage opportunities; nil = disabled
	PSAExchangeHandler        *handlers.PSAExchangeHandler     // PSA-exchange opportunities; nil = disabled
	DHHandler                 *handlers.DHHandler              // DH bulk match + intelligence; nil = disabled
	DHReconcileHandler        *handlers.DHReconcileHandler     // Admin DH reconcile trigger; nil = disabled
	SellSheetItemsHandler     *handlers.SellSheetItemsHandler  // Sell sheet persistence; nil = disabled
	CardCatalogHandler        *handlers.CardCatalogHandler     // CL card catalog search; nil = disabled
	PSASyncHandler            *handlers.PSASyncHandler         // PSA pending items + admin status; nil = disabled
	NichesHandler             *handlers.NichesHandler          // DH niche-opportunity leaderboard; nil = disabled
	CampaignSignalsHandler    *handlers.CampaignSignalsHandler // DH campaign signals; nil = disabled
	LiquidationHandler        *handlers.LiquidationHandler     // Liquidation pricing; nil = disabled
	Logger                    observability.Logger
	AdminEmails               []string
	DatabasePath              string
	TimingStore               *middleware.TimingStore
	GoogleOAuthEnv            string // controls login button visibility; "production" shows it
	LocalAPIToken             string // dev-mode bearer bypass; empty = disabled
}

// NewRouter creates a new router with the given configuration
func NewRouter(cfg RouterConfig) *Router {
	rt := &Router{
		handler:          cfg.Handler,
		healthHandler:    cfg.HealthHandler,
		apiStatusHandler: cfg.APIStatusHandler,
		spaHandler:       cfg.SPAHandler,
		logger:           cfg.Logger,
		databasePath:     cfg.DatabasePath,
		timingStore:      cfg.TimingStore,
		googleOAuthEnv:   cfg.GoogleOAuthEnv,
		localAPIToken:    cfg.LocalAPIToken,
	}

	if cfg.CampaignsHandler != nil {
		rt.campaignsHandler = cfg.CampaignsHandler
	} else if cfg.CampaignsService != nil {
		rt.campaignsHandler = handlers.NewCampaignsHandler(
			cfg.CampaignsService,
			cfg.ArbitrageService,
			cfg.PortfolioService,
			cfg.TuningService,
			cfg.Logger,
			nil,
		)
	}

	if cfg.PriceHintsHandler != nil {
		rt.priceHintsHandler = cfg.PriceHintsHandler
	}

	if cfg.PricingDiagnosticsHandler != nil {
		rt.pricingDiagnosticsHandler = cfg.PricingDiagnosticsHandler
	}

	// Create auth middleware — supports OAuth sessions and/or local API token
	if cfg.AuthService != nil {
		oauthEnv := strings.ToLower(rt.googleOAuthEnv)
		secureCookies := oauthEnv != "development" && oauthEnv != "dev"
		rt.authHandler = handlers.NewAuthHandlers(cfg.AuthService, rt.logger, secureCookies, cfg.AdminEmails)
		rt.adminHandler = handlers.NewAdminHandlers(cfg.AuthService, rt.logger)
		rt.authMW = middleware.NewAuthMiddleware(cfg.AuthService, rt.logger)
		rt.authRateLimiter = middleware.NewAuthRateLimiter(10, time.Second, nil, rt.logger)
	}
	// Enable local API token auth even without OAuth
	if token := rt.localAPIToken; token != "" {
		if rt.authMW == nil {
			rt.authMW = middleware.NewAuthMiddleware(nil, rt.logger)
		}
		rt.authMW.WithLocalAPIToken(token)
		rt.logger.Info(context.Background(), "local API token authentication enabled")
	}

	if cfg.AdvisorHandler != nil {
		rt.advisorHandler = cfg.AdvisorHandler
	}

	if cfg.InsightsHandler != nil {
		rt.insightsHandler = cfg.InsightsHandler
	}

	if cfg.AIStatusHandler != nil {
		rt.aiUsageHandler = cfg.AIStatusHandler
	}

	if cfg.PriceFlagsHandler != nil {
		rt.priceFlagsHandler = cfg.PriceFlagsHandler
	}

	if cfg.CardLadderHandler != nil {
		rt.cardLadderHandler = cfg.CardLadderHandler
	}

	if cfg.MarketMoversHandler != nil {
		rt.marketMoversHandler = cfg.MarketMoversHandler
	}

	if cfg.OpportunitiesHandler != nil {
		rt.opportunitiesHandler = cfg.OpportunitiesHandler
	}

	if cfg.PSAExchangeHandler != nil {
		rt.psaExchangeHandler = cfg.PSAExchangeHandler
	}

	if cfg.DHHandler != nil {
		rt.dhHandler = cfg.DHHandler
	}

	if cfg.DHReconcileHandler != nil {
		rt.dhReconcileHandler = cfg.DHReconcileHandler
	}

	if cfg.SellSheetItemsHandler != nil {
		rt.sellSheetItemsHandler = cfg.SellSheetItemsHandler
	}

	if cfg.CardCatalogHandler != nil {
		rt.cardCatalogHandler = cfg.CardCatalogHandler
	}

	if cfg.PSASyncHandler != nil {
		rt.psaSyncHandler = cfg.PSASyncHandler
	}

	if cfg.NichesHandler != nil {
		rt.nichesHandler = cfg.NichesHandler
	}

	if cfg.CampaignSignalsHandler != nil {
		rt.campaignSignalsHandler = cfg.CampaignSignalsHandler
	}

	if cfg.LiquidationHandler != nil {
		rt.liquidationHandler = cfg.LiquidationHandler
	}

	if cfg.PricingAPIKey != "" && cfg.CampaignsRepo != nil {
		rt.pricingAPIHandler = handlers.NewPricingAPIHandler(cfg.CampaignsRepo, cfg.Logger)
		rt.pricingAPIKey = cfg.PricingAPIKey
	}

	return rt
}

// Setup configures all routes and middleware, returning the final HTTP handler
func (rt *Router) Setup() http.Handler {
	mux := http.NewServeMux()

	// Serve static files
	// Search paths for the frontend build output directory.
	staticDirs := []string{
		filepath.Join("web", "dist"),             // Running from project root
		filepath.Join(".", "web", "dist"),        // Explicit current dir
		filepath.Join("..", "..", "web", "dist"), // Running from cmd/slabledger
	}

	var staticDir string
	for _, dir := range staticDirs {
		if _, err := os.Stat(dir); err == nil {
			staticDir = dir
			break
		}
	}

	if staticDir != "" {
		fileServer := http.FileServer(http.Dir(staticDir))
		mux.Handle("/css/", http.StripPrefix("/", fileServer))
		mux.Handle("/js/", http.StripPrefix("/", fileServer))
		mux.Handle("/assets/", http.StripPrefix("/", fileServer))
		mux.Handle("/favicon.ico", http.StripPrefix("/", fileServer))
		rt.logger.Info(context.Background(), "serving static files", observability.String("dir", staticDir))
	}

	// SPA routing
	mux.HandleFunc("/", rt.spaHandler.HandleIndex)
	mux.HandleFunc("/pricing", rt.spaHandler.HandleIndex)
	mux.HandleFunc("/login", rt.spaHandler.HandleIndex)
	mux.HandleFunc("/campaigns", rt.spaHandler.HandleIndex)

	// Authentication routes
	if rt.authHandler != nil {
		mux.Handle("/auth/google/login", rt.authRateLimiter.Middleware(http.HandlerFunc(rt.authHandler.HandleGoogleLogin)))
		mux.Handle("/auth/google/callback", rt.authRateLimiter.Middleware(http.HandlerFunc(rt.authHandler.HandleGoogleCallback)))
		mux.Handle("/api/auth/logout", rt.authRateLimiter.Middleware(http.HandlerFunc(rt.authHandler.HandleLogout)))
		mux.Handle("/api/auth/user", rt.authRateLimiter.Middleware(rt.authMW.RequireAuth(http.HandlerFunc(rt.authHandler.HandleGetCurrentUser))))
		rt.logger.Info(context.Background(), "authentication routes registered")
	}

	// CL Card Catalog search
	if rt.cardCatalogHandler != nil && rt.authMW != nil {
		mux.Handle("GET /api/cards/catalog", rt.authMW.RequireAuth(http.HandlerFunc(rt.cardCatalogHandler.HandleSearch)))
	}

	// Health & Status
	mux.HandleFunc("/api/health", rt.healthHandler.HandleHealthCheck)

	// Admin routes
	rt.registerAdminRoutes(mux)

	// Campaign endpoints (includes sell sheet, inventory, portfolio, etc.)
	rt.registerCampaignRoutes(mux)

	// AI Advisor routes
	rt.registerAdvisorRoutes(mux)

	// Insights overview routes
	rt.registerInsightsRoutes(mux)

	// Arbitrage opportunities routes
	rt.registerOpportunitiesRoutes(mux)

	// PSA-exchange opportunity routes
	rt.registerPSAExchangeRoutes(mux)

	// DH routes
	rt.registerDHRoutes(mux)

	// Intelligence (niche leaderboard) routes
	rt.registerIntelligenceRoutes(mux)

	// Pricing API (public, bearer token auth)
	rt.registerPricingAPIRoutes(mux)

	// Liquidation pricing routes
	rt.registerLiquidationRoutes(mux)

	return mux
}
