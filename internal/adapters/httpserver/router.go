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
	"github.com/guarzo/slabledger/internal/domain/favorites"
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
	cacheStatusHandler        *handlers.CacheStatusHandler
	spaHandler                *handlers.SPAHandler
	authHandler               *handlers.AuthHandlers
	adminHandler              *handlers.AdminHandlers
	authMW                    *middleware.AuthMiddleware
	authRateLimiter           *middleware.RateLimiter
	favoritesHandler          *handlers.FavoritesHandlers
	campaignsHandler          *handlers.CampaignsHandler
	priceHintsHandler         *handlers.PriceHintsHandler
	cardRequestHandler        *handlers.CardRequestHandlers
	pricingDiagnosticsHandler *handlers.PricingDiagnosticsHandler
	pricingAPIHandler         *handlers.PricingAPIHandler
	advisorHandler            *handlers.AdvisorHandler
	socialHandler             *handlers.SocialHandler
	instagramHandler          *handlers.InstagramHandler
	aiUsageHandler            *handlers.AIStatusHandler
	priceFlagsHandler         *handlers.PriceFlagsHandler
	cardLadderHandler         *handlers.CardLadderHandler
	marketMoversHandler       *handlers.MarketMoversHandler
	picksHandler              *handlers.PicksHandler
	opportunitiesHandler      *handlers.OpportunitiesHandler
	dhHandler                 *handlers.DHHandler
	sellSheetItemsHandler     *handlers.SellSheetItemsHandler
	cardCatalogHandler        *handlers.CardCatalogHandler
	psaSyncHandler            *handlers.PSASyncHandler
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
	CacheStatusHandler        *handlers.CacheStatusHandler
	SPAHandler                *handlers.SPAHandler
	AuthService               auth.Service
	FavoritesService          favorites.Service
	CampaignsHandler          *handlers.CampaignsHandler
	CampaignsService          inventory.Service
	ArbitrageService          arbitrage.Service
	PortfolioService          portfolio.Service
	TuningService             tuning.Service
	PriceHintsHandler         *handlers.PriceHintsHandler
	CardRequestHandler        *handlers.CardRequestHandlers
	PricingDiagnosticsHandler *handlers.PricingDiagnosticsHandler
	PricingAPIKey             string                          // Bearer token; empty = pricing API disabled
	CampaignsRepo             handlers.CertPriceLookup        // For pricing API handler
	AdvisorHandler            *handlers.AdvisorHandler        // AI advisor; nil = disabled
	SocialHandler             *handlers.SocialHandler         // Social content; nil = disabled
	InstagramHandler          *handlers.InstagramHandler      // Instagram publishing; nil = disabled
	AIStatusHandler           *handlers.AIStatusHandler       // AI usage stats; nil = disabled
	PriceFlagsHandler         *handlers.PriceFlagsHandler     // Price flag admin; nil = disabled
	CardLadderHandler         *handlers.CardLadderHandler     // Card Ladder admin; nil = disabled
	MarketMoversHandler       *handlers.MarketMoversHandler   // Market Movers admin; nil = disabled
	PicksHandler              *handlers.PicksHandler          // AI picks; nil = disabled
	OpportunitiesHandler      *handlers.OpportunitiesHandler  // Arbitrage opportunities; nil = disabled
	DHHandler                 *handlers.DHHandler             // DH bulk match + intelligence; nil = disabled
	SellSheetItemsHandler     *handlers.SellSheetItemsHandler // Sell sheet persistence; nil = disabled
	CardCatalogHandler        *handlers.CardCatalogHandler    // CL card catalog search; nil = disabled
	PSASyncHandler            *handlers.PSASyncHandler        // PSA pending items + admin status; nil = disabled
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
		handler:            cfg.Handler,
		healthHandler:      cfg.HealthHandler,
		apiStatusHandler:   cfg.APIStatusHandler,
		cacheStatusHandler: cfg.CacheStatusHandler,
		spaHandler:         cfg.SPAHandler,
		logger:             cfg.Logger,
		databasePath:       cfg.DatabasePath,
		timingStore:        cfg.TimingStore,
		googleOAuthEnv:     cfg.GoogleOAuthEnv,
		localAPIToken:      cfg.LocalAPIToken,
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

	if cfg.CardRequestHandler != nil {
		rt.cardRequestHandler = cfg.CardRequestHandler
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

		if cfg.FavoritesService != nil {
			rt.favoritesHandler = handlers.NewFavoritesHandlers(cfg.FavoritesService, rt.logger)
		}
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

	if cfg.SocialHandler != nil {
		rt.socialHandler = cfg.SocialHandler
	}

	if cfg.InstagramHandler != nil {
		rt.instagramHandler = cfg.InstagramHandler
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

	if cfg.PicksHandler != nil {
		rt.picksHandler = cfg.PicksHandler
	}

	if cfg.OpportunitiesHandler != nil {
		rt.opportunitiesHandler = cfg.OpportunitiesHandler
	}

	if cfg.DHHandler != nil {
		rt.dhHandler = cfg.DHHandler
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

	// Favorites page requires authentication
	if rt.authMW != nil {
		mux.Handle("/favorites", rt.authMW.RequireAuth(http.HandlerFunc(rt.spaHandler.HandleIndex)))
	} else {
		mux.HandleFunc("/favorites", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusFound)
		})
	}

	// Authentication routes
	if rt.authHandler != nil {
		mux.Handle("/auth/google/login", rt.authRateLimiter.Middleware(http.HandlerFunc(rt.authHandler.HandleGoogleLogin)))
		mux.Handle("/auth/google/callback", rt.authRateLimiter.Middleware(http.HandlerFunc(rt.authHandler.HandleGoogleCallback)))
		mux.Handle("/api/auth/logout", rt.authRateLimiter.Middleware(http.HandlerFunc(rt.authHandler.HandleLogout)))
		mux.Handle("/api/auth/user", rt.authRateLimiter.Middleware(rt.authMW.RequireAuth(http.HandlerFunc(rt.authHandler.HandleGetCurrentUser))))
		rt.logger.Info(context.Background(), "authentication routes registered")
	}

	// Favorites routes
	if rt.favoritesHandler != nil && rt.authMW != nil {
		mux.Handle("/api/favorites", rt.authMW.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				rt.favoritesHandler.HandleListFavorites(w, r)
			case http.MethodPost:
				rt.favoritesHandler.HandleAddFavorite(w, r)
			case http.MethodDelete:
				rt.favoritesHandler.HandleRemoveFavorite(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})))
		mux.Handle("/api/favorites/toggle", rt.authMW.RequireAuth(http.HandlerFunc(rt.favoritesHandler.HandleToggleFavorite)))
		mux.Handle("/api/favorites/check", rt.authMW.RequireAuth(http.HandlerFunc(rt.favoritesHandler.HandleCheckFavorites)))
		rt.logger.Info(context.Background(), "favorites routes registered")
	}

	// Core API endpoints — require authentication to protect external API budget
	if rt.authMW != nil {
		mux.Handle("/api/cards/search", rt.authMW.RequireAuth(http.HandlerFunc(rt.handler.HandleCardSearch)))
		mux.Handle("/api/cards/pricing", rt.authMW.RequireAuth(http.HandlerFunc(rt.handler.HandleCardPricing)))
	} else {
		// Fail closed: reject requests when auth middleware is not configured
		// rather than exposing external API budget to unauthenticated callers.
		noAuth := func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "authentication not configured", http.StatusServiceUnavailable)
		}
		mux.HandleFunc("/api/cards/search", noAuth)
		mux.HandleFunc("/api/cards/pricing", noAuth)
		rt.logger.Warn(context.Background(), "card search/pricing routes registered without auth — requests will be rejected")
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

	// AI Picks routes
	rt.registerPicksRoutes(mux)

	// Arbitrage opportunities routes
	rt.registerOpportunitiesRoutes(mux)

	// DH routes
	rt.registerDHRoutes(mux)

	// Social content & Instagram routes
	rt.registerSocialRoutes(mux)

	// Pricing API (public, bearer token auth)
	rt.registerPricingAPIRoutes(mux)

	return mux
}
