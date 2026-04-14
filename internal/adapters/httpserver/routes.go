package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// registerAdminRoutes wires admin, price hints, and SPA admin page routes.
func (rt *Router) registerAdminRoutes(mux *http.ServeMux) {
	// Price hints (admin-only route)
	if rt.priceHintsHandler != nil && rt.authMW != nil {
		hintsRoute := http.HandlerFunc(rt.priceHintsHandler.HandlePriceHints)
		mux.Handle("/api/price-hints", rt.authMW.RequireAdmin(hintsRoute))
		rt.logger.Info(context.Background(), "price hints routes registered",
			observability.String("component", "price-hints"))
	}

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
		if rt.databasePath != "" {
			mux.Handle("/api/admin/backup", rt.authMW.RequireAdmin(handlers.HandleBackup(rt.databasePath, rt.logger)))
		}
		if rt.timingStore != nil {
			mux.Handle("/api/admin/metrics", rt.authMW.RequireAdmin(middleware.HandleMetrics(rt.timingStore)))
		}
		if rt.pricingDiagnosticsHandler != nil {
			mux.Handle("GET /api/admin/pricing-diagnostics", rt.authMW.RequireAdmin(http.HandlerFunc(rt.pricingDiagnosticsHandler.HandlePricingDiagnostics)))
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
			mux.Handle("GET /api/admin/cardladder/failures", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleFailures)))
			mux.Handle("POST /api/admin/cardladder/refresh", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleRefresh)))
			mux.Handle("POST /api/admin/cardladder/add-card", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleAddCard)))
			mux.Handle("POST /api/admin/cardladder/sync-to-cl", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleSyncToCardLadder)))
		}
		if rt.marketMoversHandler != nil {
			mux.Handle("POST /api/admin/marketmovers/config", rt.authMW.RequireAdmin(http.HandlerFunc(rt.marketMoversHandler.HandleSaveConfig)))
			mux.Handle("GET /api/admin/marketmovers/status", rt.authMW.RequireAdmin(http.HandlerFunc(rt.marketMoversHandler.HandleStatus)))
			mux.Handle("GET /api/admin/marketmovers/failures", rt.authMW.RequireAdmin(http.HandlerFunc(rt.marketMoversHandler.HandleFailures)))
			mux.Handle("POST /api/admin/marketmovers/refresh", rt.authMW.RequireAdmin(http.HandlerFunc(rt.marketMoversHandler.HandleRefresh)))
			mux.Handle("POST /api/admin/marketmovers/sync-collection", rt.authMW.RequireAdmin(http.HandlerFunc(rt.marketMoversHandler.HandleSyncCollection)))
		}
		if rt.psaSyncHandler != nil {
			mux.Handle("GET /api/admin/psa-sync/status", rt.authMW.RequireAdmin(http.HandlerFunc(rt.psaSyncHandler.HandleStatus)))
			mux.Handle("POST /api/admin/psa-sync/refresh", rt.authMW.RequireAdmin(http.HandlerFunc(rt.psaSyncHandler.HandleRefresh)))
			mux.Handle("GET /api/admin/psa-sync/pending", rt.authMW.RequireAdmin(http.HandlerFunc(rt.psaSyncHandler.HandleListPendingItems)))
			mux.Handle("POST /api/admin/psa-sync/pending/{id}/assign", rt.authMW.RequireAdmin(http.HandlerFunc(rt.psaSyncHandler.HandleAssignPendingItem)))
			mux.Handle("DELETE /api/admin/psa-sync/pending/{id}", rt.authMW.RequireAdmin(http.HandlerFunc(rt.psaSyncHandler.HandleDismissPendingItem)))
		}
		rt.logger.Info(context.Background(), "admin routes registered")
	}

	// SPA route for admin page
	if rt.authMW != nil {
		mux.Handle("/admin", rt.authMW.RequireAdmin(http.HandlerFunc(rt.spaHandler.HandleIndex)))
	}
}

// registerCampaignRoutes wires campaign CRUD, purchases, sales, analytics, and portfolio endpoints.
func (rt *Router) registerCampaignRoutes(mux *http.ServeMux) {
	if rt.campaignsHandler == nil {
		return
	}

	// authRoute wraps a handler with auth middleware when available.
	authRoute := func(h http.HandlerFunc) http.Handler {
		if rt.authMW != nil {
			return rt.authMW.RequireAuth(h)
		}
		return h
	}

	// Global sell sheet & inventory endpoints
	mux.Handle("GET /api/sell-sheet", authRoute(rt.campaignsHandler.HandleGlobalSellSheet))
	mux.Handle("POST /api/portfolio/sell-sheet", authRoute(rt.campaignsHandler.HandleSelectedSellSheet))
	mux.Handle("GET /api/inventory", authRoute(rt.campaignsHandler.HandleGlobalInventory))
	mux.HandleFunc("/sell-sheet", rt.spaHandler.HandleIndex)
	mux.HandleFunc("/inventory", rt.spaHandler.HandleIndex)

	// Sell sheet item persistence
	if rt.sellSheetItemsHandler != nil && rt.authMW != nil {
		mux.Handle("GET /api/sell-sheet/items", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleGetItems)))
		mux.Handle("PUT /api/sell-sheet/items", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleAddItems)))
		mux.Handle("DELETE /api/sell-sheet/items", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleRemoveItems)))
		mux.Handle("DELETE /api/sell-sheet/items/all", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleClearItems)))
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
	mux.Handle("DELETE /api/campaigns/{id}/purchases/{purchaseId}/sale", authRoute(rt.campaignsHandler.HandleDeleteSale))

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
	mux.Handle("POST /api/purchases/sync-psa-sheets", authRoute(rt.campaignsHandler.HandleSyncPSASheets))
	// PSA pending items are served under /api/admin/psa-sync/pending/ (see registerAdminRoutes)
	mux.Handle("GET /api/purchases/export-cl", authRoute(rt.campaignsHandler.HandleGlobalExportCL))
	mux.Handle("GET /api/purchases/export-mm", authRoute(rt.campaignsHandler.HandleGlobalExportMM))
	mux.Handle("POST /api/purchases/refresh-mm", authRoute(rt.campaignsHandler.HandleGlobalRefreshMM))
	mux.Handle("POST /api/purchases/import-external", authRoute(rt.campaignsHandler.HandleGlobalImportExternal))
	mux.Handle("POST /api/purchases/import-orders", authRoute(rt.campaignsHandler.HandleImportOrders))
	mux.Handle("POST /api/purchases/import-orders/confirm", authRoute(rt.campaignsHandler.HandleConfirmOrdersSales))
	mux.Handle("POST /api/purchases/import-certs", authRoute(rt.campaignsHandler.HandleImportCerts))
	mux.Handle("POST /api/purchases/scan-cert", authRoute(rt.campaignsHandler.HandleScanCert))
	mux.Handle("POST /api/purchases/resolve-cert", authRoute(rt.campaignsHandler.HandleResolveCert))
	mux.Handle("GET /api/purchases/export-ebay", authRoute(rt.campaignsHandler.HandleListEbayExport))
	mux.Handle("POST /api/purchases/export-ebay/generate", authRoute(rt.campaignsHandler.HandleGenerateEbayCSV))
	mux.Handle("PATCH /api/purchases/{purchaseId}/campaign", authRoute(rt.campaignsHandler.HandleReassignPurchase))
	mux.Handle("PATCH /api/purchases/{purchaseId}/buy-cost", authRoute(rt.campaignsHandler.HandleUpdateBuyCost))

	// Price override & AI suggestion endpoints
	mux.Handle("PATCH /api/purchases/{purchaseId}/price-override", authRoute(rt.campaignsHandler.HandleSetPriceOverride))
	mux.Handle("DELETE /api/purchases/{purchaseId}/price-override", authRoute(rt.campaignsHandler.HandleClearPriceOverride))
	mux.Handle("POST /api/purchases/{purchaseId}/accept-ai-suggestion", authRoute(rt.campaignsHandler.HandleAcceptAISuggestion))
	mux.Handle("DELETE /api/purchases/{purchaseId}/ai-suggestion", authRoute(rt.campaignsHandler.HandleDismissAISuggestion))

	// Price review & flag endpoints
	mux.Handle("PATCH /api/purchases/{purchaseId}/review-price", authRoute(rt.campaignsHandler.HandleSetReviewedPrice))
	mux.Handle("POST /api/purchases/{purchaseId}/flag", authRoute(rt.campaignsHandler.HandleCreatePriceFlag))

	// Credit & Invoice endpoints
	mux.Handle("GET /api/credit/summary", authRoute(rt.campaignsHandler.HandleCapitalSummary))
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

// registerAdvisorRoutes wires the AI advisor endpoints.
func (rt *Router) registerAdvisorRoutes(mux *http.ServeMux) {
	if rt.advisorHandler == nil || rt.authMW == nil {
		return
	}
	mux.Handle("POST /api/advisor/digest", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleDigest)))
	mux.Handle("POST /api/advisor/campaign-analysis", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleCampaignAnalysis)))
	mux.Handle("POST /api/advisor/liquidation-analysis", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleLiquidationAnalysis)))
	mux.Handle("POST /api/advisor/purchase-assessment", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandlePurchaseAssessment)))
	mux.Handle("GET /api/advisor/cache/{type}", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleGetCached)))
	mux.Handle("POST /api/advisor/refresh/{type}", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleRefreshTrigger)))
	mux.HandleFunc("/advisor", rt.spaHandler.HandleIndex)
	rt.logger.Info(context.Background(), "AI advisor routes registered")
}

// registerSocialRoutes wires social content, media serving, and Instagram integration endpoints.
func (rt *Router) registerSocialRoutes(mux *http.ServeMux) {
	if rt.socialHandler != nil && rt.authMW != nil {
		mux.Handle("GET /api/social/posts", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleListPosts)))
		mux.Handle("GET /api/social/posts/{id}", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleGetPost)))
		mux.Handle("POST /api/social/posts/generate", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleGenerate)))
		mux.Handle("PATCH /api/social/posts/{id}/caption", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleUpdateCaption)))
		mux.Handle("DELETE /api/social/posts/{id}", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleDelete)))
		mux.Handle("POST /api/social/posts/{id}/regenerate-caption", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleRegenerateCaption)))
		mux.Handle("POST /api/social/posts/{id}/upload-slides", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleUploadSlides)))
		mux.Handle("GET /api/social/posts/{id}/metrics", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleGetMetrics)))
		mux.Handle("GET /api/social/metrics/summary", rt.authMW.RequireAdmin(http.HandlerFunc(rt.socialHandler.HandleGetMetricsSummary)))
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
}

// registerPricingAPIRoutes wires the public pricing API endpoints (bearer token auth).
func (rt *Router) registerPricingAPIRoutes(mux *http.ServeMux) {
	if rt.pricingAPIHandler == nil {
		return
	}
	apiKeyAuth := middleware.RequireAPIKey(rt.pricingAPIKey)
	apiRateLimiter := middleware.NewAPIRateLimiter(60, time.Minute)

	mux.HandleFunc("GET /api/v1/health", rt.pricingAPIHandler.HandleHealth)
	mux.Handle("GET /api/v1/prices/{certNumber}",
		apiKeyAuth(apiRateLimiter.Middleware(http.HandlerFunc(rt.pricingAPIHandler.HandleSinglePrice))))
	mux.Handle("POST /api/v1/prices/batch",
		apiKeyAuth(apiRateLimiter.Middleware(http.HandlerFunc(rt.pricingAPIHandler.HandleBatchPrices))))
	rt.logger.Info(context.Background(), "pricing API routes registered")
}

// registerDHRoutes wires the DH bulk match, export, intelligence, and suggestions endpoints.
func (rt *Router) registerDHRoutes(mux *http.ServeMux) {
	if rt.dhHandler == nil || rt.authMW == nil {
		if rt.dhHandler != nil {
			rt.logger.Warn(context.Background(), "skipping DH route registration: auth middleware not configured")
		}
		return
	}
	mux.Handle("POST /api/dh/match", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleBulkMatch)))
	mux.Handle("GET /api/dh/unmatched", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleUnmatched)))
	mux.Handle("GET /api/dh/intelligence", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleGetIntelligence)))
	mux.Handle("GET /api/dh/suggestions", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleGetSuggestions)))
	mux.Handle("GET /api/dh/suggestions/inventory-alerts", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleInventoryAlerts)))
	mux.Handle("GET /api/dh/status", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleGetStatus)))
	mux.Handle("POST /api/dh/fix-match", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleFixMatch)))
	mux.Handle("POST /api/dh/select-match", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleSelectMatch)))
	mux.Handle("POST /api/dh/dismiss", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleDismissMatch)))
	mux.Handle("POST /api/dh/undismiss", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleUndismissMatch)))
	mux.Handle("POST /api/dh/approve/{purchaseId}", rt.authMW.RequireAuth(http.HandlerFunc(rt.dhHandler.HandleApproveDHPush)))
	mux.Handle("POST /api/dh/reconcile", rt.authMW.RequireAdmin(http.HandlerFunc(rt.dhHandler.HandleReconcile)))
	mux.Handle("GET /api/admin/dh-push-config", rt.authMW.RequireAdmin(http.HandlerFunc(rt.dhHandler.HandleGetDHPushConfig)))
	mux.Handle("PUT /api/admin/dh-push-config", rt.authMW.RequireAdmin(http.HandlerFunc(rt.dhHandler.HandleSaveDHPushConfig)))
	rt.logger.Info(context.Background(), "DH routes registered")
}

// registerOpportunitiesRoutes wires the cross-campaign arbitrage opportunity endpoints.
func (rt *Router) registerOpportunitiesRoutes(mux *http.ServeMux) {
	if rt.opportunitiesHandler == nil || rt.authMW == nil {
		return
	}
	mux.Handle("GET /api/opportunities/acquisition", rt.authMW.RequireAuth(http.HandlerFunc(rt.opportunitiesHandler.HandleGetAcquisitionTargets)))
	mux.Handle("GET /api/opportunities/crack", rt.authMW.RequireAuth(http.HandlerFunc(rt.opportunitiesHandler.HandleGetCrackOpportunities)))
	rt.logger.Info(context.Background(), "opportunities routes registered")
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
