package main

// runtime.go holds the wiring struct and the phase helpers that build it.
// runServer (main.go) calls these in order — infrastructure, then
// repositories/auth, then campaigns/integrations — and uses the resulting
// wiring to build scheduler and handler inputs. Extracted from runServer
// itself to keep that function under the funlen budget (SLA-97); the
// decomposition is composition-only, no behavior changed.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhlistingadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/google"
	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// wiring holds every piece runServer assembles across its build phases. A
// single struct (rather than a long chain of locals) is what lets the
// phases live in separate functions: each phase reads what an earlier
// phase set and writes what a later phase or the scheduler/handler input
// builders need.
type wiring struct {
	db                   *postgres.DB
	priceRepo            *postgres.DBTracker
	refreshCandidateRepo *postgres.RefreshCandidateRepository
	dhTombstoneStore     *postgres.DHCardTombstoneStore
	authService          auth.Service
	cardIDMappingRepo    *postgres.CardIDMappingRepository
	dhClient             *dh.Client
	intelRepo            *postgres.MarketIntelligenceRepository
	suggestionsRepo      *postgres.DHSuggestionsRepository
	demandRepo           *postgres.DHDemandRepository
	trajectoryRepo       *postgres.CardPriceTrajectoryRepository
	eventStore           *postgres.DHEventStore
	priceProvImpl        pricing.PriceProvider
	syncStateRepo        *postgres.SyncStateRepository
	clClient             *cardladder.Client
	clStore              *postgres.CardLadderStore
	clSalesStore         *postgres.CLSalesStore
	clCompRefreshStore   *postgres.CompRefreshStore
	schedulerStatsStore  *postgres.SchedulerStatsStore
	psaSnapshotStore     *postgres.PSAPortalSnapshotStore
	psaRowProvider       *psaportal.SnapshotRowProvider
	dhPriceSyncService   dhpricing.Service
	campaignsInit        campaignsInitResult
}

// setupInfrastructure opens the database, starts the Prometheus metrics
// server, and runs migrations. It returns cleanup funcs instead of
// deferring internally: a defer registered in this method fires when this
// method returns, not when runServer does, which would tear the database
// and metrics server down before the HTTP server ever starts. The caller
// must defer dbCleanup before metricsCleanup — see the comment at the
// runServer call site for why the order matters.
func (w *wiring) setupInfrastructure(ctx context.Context, cfg *config.Config, logger observability.Logger) (dbCleanup func(), metricsCleanup func(), err error) {
	db, err := postgres.Open(ctx, cfg.Database.URL, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	w.db = db
	// db.Close() blocks until in-flight queries complete. Safe because schedulers
	// are stopped and HTTP server is drained before this runs.
	dbCleanup = func() {
		if err := db.Close(); err != nil {
			logger.Warn(ctx, "Failed to close database", observability.Err(err))
		}
	}

	// Prometheus metrics server on :9091 — scraped by Fly's built-in Prometheus.
	// Separate port so app middleware (auth, rate limiter) doesn't interfere.
	metricsReg := prometheus.NewRegistry()
	metricsReg.MustRegister(collectors.NewGoCollector())
	metricsReg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsReg.MustRegister(postgres.NewDBStatsCollector("slabledger", db.Stats))

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(metricsReg, promhttp.HandlerOpts{Registry: metricsReg}))
	metricsSrv := &http.Server{
		Addr:              ":9091",
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info(ctx, "metrics server listening", observability.String("addr", metricsSrv.Addr))
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "metrics server error", observability.Err(err))
		}
	}()
	metricsCleanup = func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsSrv.Shutdown(shutCtx); err != nil {
			logger.Warn(context.Background(), "metrics server shutdown error", observability.Err(err))
		}
	}

	migrationsPath := cfg.Database.MigrationsPath
	if err := postgres.RunMigrations(db, migrationsPath); err != nil {
		return dbCleanup, metricsCleanup, fmt.Errorf("run migrations: %w", err)
	}

	return dbCleanup, metricsCleanup, nil
}

// buildRepositoriesAndAuth builds the repositories, optional auth service,
// optional DH client, and the price provider. Requires w.db to be set.
func (w *wiring) buildRepositoriesAndAuth(ctx context.Context, cfg *config.Config, logger observability.Logger) error {
	// Create DB tracker (API tracking, access tracking, health checks)
	w.priceRepo = postgres.NewDBTracker(w.db)
	w.refreshCandidateRepo = postgres.NewRefreshCandidateRepository(w.db.DB)
	w.dhTombstoneStore = postgres.NewDHCardTombstoneStore(w.db.DB)

	// Initialize authentication
	googleConfig := config.LoadGoogleOAuthConfig()

	if encryptionKey := cfg.Auth.EncryptionKey; encryptionKey != "" {
		encryptor, err := crypto.NewAESEncryptor(encryptionKey)
		if err != nil {
			return fmt.Errorf("initialize encryptor: %w", err)
		}

		authRepo := postgres.NewAuthRepository(w.db.DB, encryptor)

		if googleConfig.IsConfigured() {
			w.authService = google.NewOAuthService(
				authRepo, logger,
				googleConfig.ClientID, googleConfig.ClientSecret,
				googleConfig.RedirectURI, googleConfig.Scopes,
			)
			logger.Info(ctx, "authentication service initialized",
				observability.String("redirect_uri", googleConfig.RedirectURI))
		}
	}

	// Card ID mapping repository (caches external provider IDs)
	w.cardIDMappingRepo = postgres.NewCardIDMappingRepository(w.db.DB)

	// Initialize DH client (optional — market intelligence + pricing source)
	if cfg.Adapters.DHEnterpriseKey != "" && cfg.Adapters.DHBaseURL != "" {
		w.dhClient = dh.NewClient(
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
	w.intelRepo = postgres.NewMarketIntelligenceRepository(w.db.DB)
	w.suggestionsRepo = postgres.NewDHSuggestionsRepository(w.db.DB)
	w.demandRepo = postgres.NewDHDemandRepository(w.db.DB)
	w.trajectoryRepo = postgres.NewCardPriceTrajectoryRepository(w.db.DB)

	// DH event store — records pipeline state transitions (migration 000068)
	w.eventStore = postgres.NewDHEventStore(w.db.DB)

	priceProvImpl, err := initializePriceProviders(
		ctx, logger, w.cardIDMappingRepo,
		w.dhClient,
		w.dhTombstoneStore,
	)
	if err != nil {
		return err
	}
	w.priceProvImpl = priceProvImpl

	return nil
}

// buildCampaignsAndIntegrations builds the campaigns service and the
// third-party integrations that depend on it (Card Ladder, PSA portal
// snapshot provider, DH price re-sync service). Requires w.db,
// w.priceProvImpl, w.intelRepo, w.dhClient, w.eventStore, and
// w.cardIDMappingRepo to already be set.
func (w *wiring) buildCampaignsAndIntegrations(ctx context.Context, cfg *config.Config, logger observability.Logger) {
	// Create encryptor early — needed by Card Ladder.
	var clEncryptor crypto.Encryptor
	if cfg.Auth.EncryptionKey != "" {
		var encErr error
		clEncryptor, encErr = crypto.NewAESEncryptor(cfg.Auth.EncryptionKey)
		if encErr != nil {
			logger.Warn(ctx, "encryptor initialization failed, CL token persistence disabled",
				observability.Err(encErr))
		}
	}

	w.campaignsInit = initializeCampaignsService(
		ctx, cfg, logger, w.db, w.priceProvImpl, w.intelRepo, w.dhClient, w.eventStore, w.cardIDMappingRepo,
	)

	// Sync state repository (for delta poll timestamps)
	w.syncStateRepo = postgres.NewSyncStateRepository(w.db.DB)

	// Initialize Card Ladder
	clClient, _, clStore := initializeCardLadder(ctx, logger, w.db, clEncryptor)
	w.clClient = clClient
	w.clStore = clStore
	if clStore != nil {
		w.clSalesStore = postgres.NewCLSalesStore(w.db.DB)
		w.clCompRefreshStore = postgres.NewCompRefreshStore(w.db.DB)
	}
	w.schedulerStatsStore = postgres.NewSchedulerStatsStore(w.db.DB)

	// PSA portal rows are harvested out-of-process by the `psa-harvest` job (it
	// needs a browser to pass Cloudflare, which the lean app image can't run) and
	// stored as a snapshot in Postgres; the reader only queries the DB.
	if cfg.PSAPortal.Enabled {
		w.psaSnapshotStore = postgres.NewPSAPortalSnapshotStore(w.db.DB)
		w.psaRowProvider = psaportal.NewSnapshotRowProvider(w.psaSnapshotStore, logger)
		logger.Info(ctx, "PSA portal snapshot provider initialized")
	}

	// DH price re-sync service: drives both the inline goroutine on review-price
	// edits (via CampaignsHandler) and the periodic reconciliation scheduler.
	// Constructed once so both consumers share the same instance.
	if w.dhClient != nil && w.dhClient.EnterpriseAvailable() && w.campaignsInit.purchaseStore != nil {
		w.dhPriceSyncService = dhpricing.NewService(
			w.campaignsInit.purchaseStore,                    // PurchaseLookup: GetPurchase + ListDHPriceDrift
			dhlistingadapter.NewInventoryAdapter(w.dhClient), // DHPriceUpdater
			w.campaignsInit.purchaseStore,                    // DHPriceWriter: UpdatePurchaseDHPriceSync
			w.campaignsInit.purchaseStore,                    // DHReconcileResetter
			logger,
		)
	}
}

// schedulerDeps builds the schedulerDeps struct for initializeSchedulers
// from the wiring's fields.
func (w *wiring) schedulerDeps(cfg *config.Config, logger observability.Logger) schedulerDeps {
	sDeps := schedulerDeps{
		Config:                     cfg,
		Logger:                     logger,
		DBTracker:                  w.priceRepo,
		RefreshCandidates:          w.refreshCandidateRepo,
		PriceProvImpl:              w.priceProvImpl,
		AuthService:                w.authService,
		SyncStateRepo:              w.syncStateRepo,
		CardIDMappingRepo:          w.cardIDMappingRepo,
		CampaignStore:              w.campaignsInit.campaignStore,
		PurchaseStore:              w.campaignsInit.purchaseStore,
		DHStore:                    w.campaignsInit.dhStore,
		CampaignsService:           w.campaignsInit.service,
		ImportService:              w.campaignsInit.importService,
		CertLookup:                 w.campaignsInit.certLookup,
		CertEnrichJob:              w.campaignsInit.certEnrichJob,
		PricingEnrichJob:           w.campaignsInit.pricingEnrichJob,
		CardLadderClient:           w.clClient,
		CardLadderStore:            w.clStore,
		CardLadderSalesStore:       w.clSalesStore,
		CardLadderCompRefreshStore: w.clCompRefreshStore,
		SchedulerStatsStore:        w.schedulerStatsStore,
		DHClient:                   w.dhClient,
		SaleStore:                  w.campaignsInit.saleStore,
		DHEventStore:               w.eventStore,
		DHIntelligenceRepo:         w.intelRepo,
		DHSuggestionsRepo:          w.suggestionsRepo,
		DHDemandRepo:               w.demandRepo,
		DHTrajectoryRepo:           w.trajectoryRepo,
		DHCompCacheStore:           w.campaignsInit.dhCompStore,
		DHPriceSyncService:         w.dhPriceSyncService,
		DHTombstoneStore:           w.dhTombstoneStore,
	}
	if w.psaRowProvider != nil {
		sDeps.PSARowProvider = w.psaRowProvider
	}
	return sDeps
}

// handlerInputs builds the handlerInputs struct for createHandlers from
// the wiring's fields plus the scheduler result createHandlers needs to
// wire manual-refresh callbacks.
func (w *wiring) handlerInputs(cfg *config.Config, logger observability.Logger, schedulerResult *scheduler.BuildResult) handlerInputs {
	return handlerInputs{
		Cfg:                  cfg,
		Logger:               logger,
		DB:                   w.db,
		PriceProvImpl:        w.priceProvImpl,
		PriceRepo:            w.priceRepo,
		AuthService:          w.authService,
		CampaignsService:     w.campaignsInit.service,
		ImportService:        w.campaignsInit.importService,
		ArbitrageService:     w.campaignsInit.arbSvc,
		PortfolioService:     w.campaignsInit.portSvc,
		TuningService:        w.campaignsInit.tuningSvc,
		CampaignStore:        w.campaignsInit.campaignStore,
		FinanceService:       w.campaignsInit.financeService,
		ExportService:        w.campaignsInit.exportService,
		PurchaseStore:        w.campaignsInit.purchaseStore,
		CardIDMappingRepo:    w.cardIDMappingRepo,
		IntelRepo:            w.intelRepo,
		TrajectoryRepo:       w.trajectoryRepo,
		SuggestionsRepo:      w.suggestionsRepo,
		DemandRepo:           w.demandRepo,
		CLClient:             w.clClient,
		CLStore:              w.clStore,
		DHClient:             w.dhClient,
		DHEventStore:         w.eventStore,
		DHStore:              w.campaignsInit.dhStore,
		DHPriceSyncService:   w.dhPriceSyncService,
		SyncStateRepo:        w.syncStateRepo,
		SchedulerResult:      schedulerResult,
		PSARowProvider:       w.psaRowProvider,
		PSARowsSnapshotStore: w.psaSnapshotStore,
		PendingItemsRepo:     w.campaignsInit.pendingItemsRepo,
		DHTombstoneStore:     w.dhTombstoneStore,
	}
}
