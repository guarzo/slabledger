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
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/favorites"
	"github.com/guarzo/slabledger/internal/domain/observability"
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
	pricingAPIKey             string
	logger                    observability.Logger
	databasePath              string
	timingStore               *middleware.TimingStore
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
	CampaignsService          campaigns.Service
	PriceHintsHandler         *handlers.PriceHintsHandler
	CardRequestHandler        *handlers.CardRequestHandlers
	PricingDiagnosticsHandler *handlers.PricingDiagnosticsHandler
	PricingAPIKey             string                      // Bearer token; empty = pricing API disabled
	CampaignsRepo             handlers.CertPriceLookup    // For pricing API handler
	AdvisorHandler            *handlers.AdvisorHandler    // AI advisor; nil = disabled
	SocialHandler             *handlers.SocialHandler     // Social content; nil = disabled
	InstagramHandler          *handlers.InstagramHandler  // Instagram publishing; nil = disabled
	AIStatusHandler           *handlers.AIStatusHandler   // AI usage stats; nil = disabled
	PriceFlagsHandler         *handlers.PriceFlagsHandler // Price flag admin; nil = disabled
	CardLadderHandler         *handlers.CardLadderHandler // Card Ladder admin; nil = disabled
	Logger                    observability.Logger
	AdminEmails               []string
	DatabasePath              string
	TimingStore               *middleware.TimingStore
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
	}

	if cfg.CampaignsHandler != nil {
		rt.campaignsHandler = cfg.CampaignsHandler
	} else if cfg.CampaignsService != nil {
		rt.campaignsHandler = handlers.NewCampaignsHandler(cfg.CampaignsService, cfg.Logger, nil, nil)
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
		oauthEnv := strings.ToLower(os.Getenv("GOOGLE_OAUTH_ENV"))
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
	if token := os.Getenv("LOCAL_API_TOKEN"); token != "" {
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

	// Price hints (admin-only route)
	if rt.priceHintsHandler != nil && rt.authMW != nil {
		hintsRoute := http.HandlerFunc(rt.priceHintsHandler.HandlePriceHints)
		mux.Handle("/api/price-hints", rt.authMW.RequireAdmin(hintsRoute))
		rt.logger.Info(context.Background(), "price hints routes registered",
			observability.String("component", "price-hints"))
	}

	// Health & Status
	mux.HandleFunc("/api/health", rt.healthHandler.HandleHealthCheck)

	// Admin routes
	if rt.adminHandler != nil && rt.authMW != nil {
		mux.Handle("/api/admin/allowlist", rt.authMW.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				rt.adminHandler.HandleListAllowedEmails(w, r)
			case http.MethodPost:
				rt.adminHandler.HandleAddAllowedEmail(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})))
		mux.Handle("/api/admin/allowlist/", rt.authMW.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				rt.adminHandler.HandleRemoveAllowedEmail(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})))
		mux.Handle("/api/admin/users", rt.authMW.RequireAdmin(http.HandlerFunc(rt.adminHandler.HandleListUsers)))
		if rt.apiStatusHandler != nil {
			mux.Handle("/api/admin/api-usage", rt.authMW.RequireAdmin(http.HandlerFunc(rt.apiStatusHandler.HandleAPIUsage)))
		}
		if rt.cacheStatusHandler != nil {
			mux.Handle("/api/admin/cache-stats", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cacheStatusHandler.HandleCacheStats)))
		}
		if rt.databasePath != "" {
			mux.Handle("/api/admin/backup", rt.authMW.RequireAdmin(handlers.HandleBackup(rt.databasePath, rt.logger)))
		}
		if rt.timingStore != nil {
			mux.Handle("/api/admin/metrics", rt.authMW.RequireAdmin(middleware.HandleMetrics(rt.timingStore)))
		}
		if rt.pricingDiagnosticsHandler != nil {
			mux.Handle("GET /api/admin/pricing-diagnostics", rt.authMW.RequireAdmin(http.HandlerFunc(rt.pricingDiagnosticsHandler.HandlePricingDiagnostics)))
		}
		if rt.cardRequestHandler != nil {
			mux.Handle("GET /api/admin/card-requests", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardRequestHandler.HandleListCardRequests)))
			mux.Handle("POST /api/admin/card-requests/{id}/submit", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardRequestHandler.HandleSubmitCardRequest)))
			mux.Handle("POST /api/admin/card-requests/submit-all", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardRequestHandler.HandleSubmitAllCardRequests)))
		}
		if rt.campaignsHandler != nil {
			mux.Handle("GET /api/admin/price-override-stats", rt.authMW.RequireAdmin(http.HandlerFunc(rt.campaignsHandler.HandlePriceOverrideStats)))
		}
		if rt.aiUsageHandler != nil {
			mux.Handle("GET /api/admin/ai-usage", rt.authMW.RequireAdmin(http.HandlerFunc(rt.aiUsageHandler.HandleAIUsage)))
		}
		if rt.priceFlagsHandler != nil {
			mux.Handle("GET /api/admin/price-flags", rt.authMW.RequireAdmin(http.HandlerFunc(rt.priceFlagsHandler.HandleListPriceFlags)))
			mux.Handle("PATCH /api/admin/price-flags/{flagId}/resolve", rt.authMW.RequireAdmin(http.HandlerFunc(rt.priceFlagsHandler.HandleResolvePriceFlag)))
		}
		if rt.cardLadderHandler != nil {
			mux.Handle("POST /api/admin/cardladder/config", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleSaveConfig)))
			mux.Handle("GET /api/admin/cardladder/status", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleStatus)))
			mux.Handle("POST /api/admin/cardladder/refresh", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleRefresh)))
		}
		rt.logger.Info(context.Background(), "admin routes registered")
	}

	// SPA route for admin page
	if rt.authMW != nil {
		mux.Handle("/admin", rt.authMW.RequireAdmin(http.HandlerFunc(rt.spaHandler.HandleIndex)))
	}

	// Global sell sheet & inventory endpoints
	if rt.campaignsHandler != nil {
		sellSheetRoute := http.HandlerFunc(rt.campaignsHandler.HandleGlobalSellSheet)
		selectedSellSheetRoute := http.HandlerFunc(rt.campaignsHandler.HandleSelectedSellSheet)
		inventoryRoute := http.HandlerFunc(rt.campaignsHandler.HandleGlobalInventory)
		if rt.authMW != nil {
			mux.Handle("/api/sell-sheet", rt.authMW.RequireAuth(sellSheetRoute))
			mux.Handle("POST /api/portfolio/sell-sheet", rt.authMW.RequireAuth(selectedSellSheetRoute))
			mux.Handle("/api/inventory", rt.authMW.RequireAuth(inventoryRoute))
		} else {
			mux.HandleFunc("/api/sell-sheet", rt.campaignsHandler.HandleGlobalSellSheet)
			mux.HandleFunc("POST /api/portfolio/sell-sheet", rt.campaignsHandler.HandleSelectedSellSheet)
			mux.HandleFunc("/api/inventory", rt.campaignsHandler.HandleGlobalInventory)
		}
		mux.HandleFunc("/sell-sheet", rt.spaHandler.HandleIndex)
		mux.HandleFunc("/inventory", rt.spaHandler.HandleIndex)
	}

	// Campaign endpoints — uses Go 1.22 method+path mux patterns
	if rt.campaignsHandler != nil {
		// authRoute wraps a handler with auth middleware when available.
		authRoute := func(h http.HandlerFunc) http.Handler {
			if rt.authMW != nil {
				return rt.authMW.RequireAuth(h)
			}
			return h
		}

		// Campaign CRUD
		mux.Handle("GET /api/campaigns", authRoute(rt.campaignsHandler.HandleListCampaigns))
		mux.Handle("POST /api/campaigns", authRoute(rt.campaignsHandler.HandleCreateCampaign))
		mux.Handle("GET /api/campaigns/{id}", authRoute(rt.campaignsHandler.HandleGetCampaign))
		mux.Handle("PUT /api/campaigns/{id}", authRoute(rt.campaignsHandler.HandleUpdateCampaign))
		mux.Handle("DELETE /api/campaigns/{id}", authRoute(rt.campaignsHandler.HandleDelete))

		// Campaign purchases
		mux.Handle("GET /api/campaigns/{id}/purchases", authRoute(rt.campaignsHandler.HandleListPurchases))
		mux.Handle("POST /api/campaigns/{id}/purchases", authRoute(rt.campaignsHandler.HandleCreatePurchase))
		mux.Handle("POST /api/campaigns/{id}/purchases/quick-add", authRoute(rt.campaignsHandler.HandleQuickAdd))
		mux.Handle("DELETE /api/campaigns/{id}/purchases/{purchaseId}", authRoute(rt.campaignsHandler.HandleDeletePurchase))

		// Campaign sales
		mux.Handle("GET /api/campaigns/{id}/sales", authRoute(rt.campaignsHandler.HandleListSales))
		mux.Handle("POST /api/campaigns/{id}/sales", authRoute(rt.campaignsHandler.HandleCreateSale))
		mux.Handle("POST /api/campaigns/{id}/sales/bulk", authRoute(rt.campaignsHandler.HandleBulkSales))

		// Campaign analytics
		mux.Handle("GET /api/campaigns/{id}/pnl", authRoute(rt.campaignsHandler.HandleCampaignPNL))
		mux.Handle("GET /api/campaigns/{id}/pnl-by-channel", authRoute(rt.campaignsHandler.HandlePNLByChannel))
		mux.Handle("GET /api/campaigns/{id}/fill-rate", authRoute(rt.campaignsHandler.HandleFillRate))
		mux.Handle("GET /api/campaigns/{id}/days-to-sell", authRoute(rt.campaignsHandler.HandleDaysToSell))
		mux.Handle("GET /api/campaigns/{id}/inventory", authRoute(rt.campaignsHandler.HandleInventory))
		mux.Handle("POST /api/campaigns/{id}/sell-sheet", authRoute(rt.campaignsHandler.HandleSellSheet))
		mux.Handle("GET /api/campaigns/{id}/tuning", authRoute(rt.campaignsHandler.HandleTuning))
		mux.Handle("GET /api/campaigns/{id}/crack-candidates", authRoute(rt.campaignsHandler.HandleCrackCandidates))
		mux.Handle("GET /api/campaigns/{id}/expected-values", authRoute(rt.campaignsHandler.HandleExpectedValues))
		mux.Handle("POST /api/campaigns/{id}/evaluate-purchase", authRoute(rt.campaignsHandler.HandleEvaluatePurchase))
		mux.Handle("GET /api/campaigns/{id}/activation-checklist", authRoute(rt.campaignsHandler.HandleActivationChecklist))
		mux.Handle("GET /api/campaigns/{id}/projections", authRoute(rt.campaignsHandler.HandleProjections))

		// Global purchase endpoints (cross-campaign)
		mux.Handle("POST /api/purchases/refresh-cl", authRoute(rt.campaignsHandler.HandleGlobalRefreshCL))
		mux.Handle("POST /api/purchases/import-cl", authRoute(rt.campaignsHandler.HandleGlobalImportCL))
		mux.Handle("POST /api/purchases/import-psa", authRoute(rt.campaignsHandler.HandleGlobalImportPSA))
		mux.Handle("GET /api/purchases/export-cl", authRoute(rt.campaignsHandler.HandleGlobalExportCL))
		mux.Handle("POST /api/purchases/import-external", authRoute(rt.campaignsHandler.HandleGlobalImportExternal))
		mux.Handle("POST /api/purchases/import-orders", authRoute(rt.campaignsHandler.HandleImportOrders))
		mux.Handle("POST /api/purchases/import-orders/confirm", authRoute(rt.campaignsHandler.HandleConfirmOrdersSales))
		mux.Handle("POST /api/purchases/import-certs", authRoute(rt.campaignsHandler.HandleImportCerts))
		mux.Handle("GET /api/purchases/export-ebay", authRoute(rt.campaignsHandler.HandleListEbayExport))
		mux.Handle("POST /api/purchases/export-ebay/generate", authRoute(rt.campaignsHandler.HandleGenerateEbayCSV))
		mux.Handle("PATCH /api/purchases/{purchaseId}/campaign", authRoute(rt.campaignsHandler.HandleReassignPurchase))

		// Price override & AI suggestion endpoints
		mux.Handle("PATCH /api/purchases/{purchaseId}/price-override", authRoute(rt.campaignsHandler.HandleSetPriceOverride))
		mux.Handle("DELETE /api/purchases/{purchaseId}/price-override", authRoute(rt.campaignsHandler.HandleClearPriceOverride))
		mux.Handle("POST /api/purchases/{purchaseId}/accept-ai-suggestion", authRoute(rt.campaignsHandler.HandleAcceptAISuggestion))
		mux.Handle("DELETE /api/purchases/{purchaseId}/ai-suggestion", authRoute(rt.campaignsHandler.HandleDismissAISuggestion))

		// Price review & flag endpoints
		mux.Handle("PATCH /api/purchases/{purchaseId}/review-price", authRoute(rt.campaignsHandler.HandleSetReviewedPrice))
		mux.Handle("POST /api/purchases/{purchaseId}/flag", authRoute(rt.campaignsHandler.HandleCreatePriceFlag))

		// Credit & Invoice endpoints
		mux.Handle("GET /api/credit/summary", authRoute(rt.campaignsHandler.HandleCreditSummary))
		mux.Handle("GET /api/credit/config", authRoute(rt.campaignsHandler.HandleGetCashflowConfig))
		mux.Handle("PUT /api/credit/config", authRoute(rt.campaignsHandler.HandleUpdateCashflowConfig))
		mux.Handle("GET /api/credit/invoices", authRoute(rt.campaignsHandler.HandleListInvoices))
		mux.Handle("PUT /api/credit/invoices", authRoute(rt.campaignsHandler.HandleUpdateInvoice))

		// Portfolio endpoints
		mux.Handle("GET /api/portfolio/health", authRoute(rt.campaignsHandler.HandlePortfolioHealth))
		mux.Handle("GET /api/portfolio/channel-velocity", authRoute(rt.campaignsHandler.HandlePortfolioChannelVelocity))
		mux.Handle("GET /api/portfolio/insights", authRoute(rt.campaignsHandler.HandlePortfolioInsights))
		mux.Handle("GET /api/portfolio/suggestions", authRoute(rt.campaignsHandler.HandleCampaignSuggestions))
		mux.Handle("GET /api/portfolio/revocations", authRoute(rt.campaignsHandler.HandleListRevocationFlags))
		mux.Handle("POST /api/portfolio/revocations", authRoute(rt.campaignsHandler.HandleCreateRevocationFlag))
		mux.Handle("GET /api/portfolio/revocations/{flagId}/email", authRoute(rt.campaignsHandler.HandleRevocationEmail))
		mux.Handle("GET /api/portfolio/capital-timeline", authRoute(rt.campaignsHandler.HandleCapitalTimeline))
		mux.Handle("GET /api/portfolio/weekly-review", authRoute(rt.campaignsHandler.HandleWeeklyReview))

		// Cert lookup endpoint
		mux.Handle("GET /api/certs/{certNumber}", authRoute(rt.campaignsHandler.HandleCertLookup))

		// Shopify price sync
		mux.Handle("POST /api/shopify/price-sync", authRoute(rt.campaignsHandler.HandleShopifyPriceSync))

		// SPA routing for campaign deep links and portfolio pages
		mux.HandleFunc("/campaigns/", rt.spaHandler.HandleIndex)
		mux.HandleFunc("/insights", rt.spaHandler.HandleIndex)
		mux.HandleFunc("/suggestions", rt.spaHandler.HandleIndex)
		mux.HandleFunc("/shopify-sync", rt.spaHandler.HandleIndex)

		rt.logger.Info(context.Background(), "campaign routes registered")
	}

	// AI Advisor routes — require auth (fail closed when auth not configured)
	if rt.advisorHandler != nil && rt.authMW != nil {
		mux.Handle("POST /api/advisor/digest", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleDigest)))
		mux.Handle("POST /api/advisor/campaign-analysis", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleCampaignAnalysis)))
		mux.Handle("POST /api/advisor/liquidation-analysis", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleLiquidationAnalysis)))
		mux.Handle("POST /api/advisor/purchase-assessment", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandlePurchaseAssessment)))

		// Cached analysis endpoints
		mux.Handle("GET /api/advisor/cache/{type}", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleGetCached)))
		mux.Handle("POST /api/advisor/refresh/{type}", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleRefreshTrigger)))

		mux.HandleFunc("/advisor", rt.spaHandler.HandleIndex)
		rt.logger.Info(context.Background(), "AI advisor routes registered")
	}

	// Social content routes — require admin
	if rt.socialHandler != nil && rt.authMW != nil {
		mux.Handle("GET /api/social/posts", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleListPosts)))
		mux.Handle("GET /api/social/posts/{id}", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleGetPost)))
		mux.Handle("POST /api/social/posts/generate", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleGenerate)))
		mux.Handle("PATCH /api/social/posts/{id}/caption", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleUpdateCaption)))
		mux.Handle("DELETE /api/social/posts/{id}", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleDelete)))
		mux.Handle("POST /api/social/backfill-images", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleBackfillImages)))
		mux.Handle("POST /api/social/posts/{id}/regenerate-caption", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleRegenerateCaption)))
		mux.Handle("POST /api/social/posts/{id}/upload-slides", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleUploadSlides)))

		mux.Handle("/content", rt.authMW.RequireAdmin(http.HandlerFunc(rt.spaHandler.HandleIndex)))
		rt.logger.Info(context.Background(), "social content routes registered")
	}

	// Media serving route — unauthenticated (Instagram API needs public access)
	if rt.socialHandler != nil {
		mux.HandleFunc("GET /api/media/social/{postId}/{filename}", rt.socialHandler.HandleServeMedia)
	}

	// Image proxy — requires admin (only used by social content publish UI)
	if rt.socialHandler != nil && rt.authMW != nil {
		mux.Handle("GET /api/image-proxy", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleImageProxy)))
	}

	// Instagram integration routes — require admin
	if rt.instagramHandler != nil && rt.authMW != nil {
		mux.Handle("GET /api/instagram/status", rt.authMW.RequireAdmin(http.HandlerFunc(rt.instagramHandler.HandleStatus)))
		mux.Handle("POST /api/instagram/connect", rt.authMW.RequireAdmin(http.HandlerFunc(rt.instagramHandler.HandleConnect)))
		mux.HandleFunc("/auth/instagram/callback", rt.instagramHandler.HandleCallback) // public — OAuth redirect
		mux.Handle("POST /api/instagram/disconnect", rt.authMW.RequireAdmin(http.HandlerFunc(rt.instagramHandler.HandleDisconnect)))
		mux.Handle("POST /api/social/posts/{id}/publish", rt.authMW.RequireAdmin(http.HandlerFunc(rt.instagramHandler.HandlePublish)))
		rt.logger.Info(context.Background(), "instagram integration routes registered")
	}

	// Pricing API (public, bearer token auth)
	if rt.pricingAPIHandler != nil {
		apiKeyAuth := middleware.RequireAPIKey(rt.pricingAPIKey)
		apiRateLimiter := middleware.NewAPIRateLimiter(60, time.Minute)

		mux.HandleFunc("GET /api/v1/health", rt.pricingAPIHandler.HandleHealth)
		mux.Handle("GET /api/v1/prices/{certNumber}",
			apiKeyAuth(apiRateLimiter.Middleware(http.HandlerFunc(rt.pricingAPIHandler.HandleSinglePrice))))
		mux.Handle("POST /api/v1/prices/batch",
			apiKeyAuth(apiRateLimiter.Middleware(http.HandlerFunc(rt.pricingAPIHandler.HandleBatchPrices))))
		rt.logger.Info(context.Background(), "pricing API routes registered")
	}

	return mux
}

// TrackedEndpoints lists the endpoints whose response times are recorded.
var TrackedEndpoints = []string{
	"/api/portfolio/insights",
	"/api/portfolio/capital-timeline",
	"/api/portfolio/weekly-review",
	"/api/campaigns/{id}/tuning",
	"/api/campaigns/{id}/sell-sheet",
}

// ApplyMiddleware wraps the router with middleware layers.
// Order (inside-out): CORS → Gzip → Logging → Timing → SecurityHeaders → Recovery → RateLimiter
// Recovery is outside Logging so panics are caught before the logging layer sees them.
func ApplyMiddleware(handler http.Handler, rateLimiter *middleware.RateLimiter, logger observability.Logger, timingStore *middleware.TimingStore) http.Handler {
	wrapped := middleware.CORSMiddleware(logger, handler)
	wrapped = middleware.GzipMiddleware(wrapped)
	wrapped = middleware.LoggingMiddleware(logger)(wrapped)
	if timingStore != nil {
		wrapped = middleware.TimingMiddleware(timingStore)(wrapped)
	}
	wrapped = middleware.SecurityHeaders(wrapped)
	wrapped = middleware.RecoveryMiddleware(logger)(wrapped)
	wrapped = rateLimiter.Middleware(wrapped)
	return wrapped
}
