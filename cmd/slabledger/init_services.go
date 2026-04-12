package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/azureai"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/dhprice"
	igclient "github.com/guarzo/slabledger/internal/adapters/clients/instagram"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricelookup"
	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	scoringadapter "github.com/guarzo/slabledger/internal/adapters/scoring"
	"github.com/guarzo/slabledger/internal/adapters/storage/mediafs"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/domain/tuning"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// initializePriceProviders creates the DH price provider.
func initializePriceProviders(
	ctx context.Context,
	logger observability.Logger,
	cardIDMappingRepo *sqlite.CardIDMappingRepository,
	dhClient *dh.Client,
) (pricing.PriceProvider, error) {
	if dhClient == nil || !dhClient.EnterpriseAvailable() {
		logger.Warn(ctx, "DH client not available; price provider will be inactive")
		return dhprice.New(nil, nil), nil
	}
	provider := dhprice.New(dhClient, cardIDMappingRepo, dhprice.WithLogger(logger))
	logger.Info(ctx, "DH price provider initialized")
	return provider, nil
}

// campaignsInitResult holds all values returned by initializeCampaignsService.
type campaignsInitResult struct {
	service         inventory.Service
	campaignStore   *sqlite.CampaignStore
	purchaseStore   *sqlite.PurchaseStore
	saleStore       *sqlite.SaleStore
	analyticsStore  *sqlite.AnalyticsStore
	financeStore    *sqlite.FinanceStore
	pricingStore    *sqlite.PricingStore
	dhStore         *sqlite.DHStore
	snapshotStore   *sqlite.SnapshotStore
	sellSheetStore  *sqlite.SellSheetStore
	cardRequestRepo *sqlite.CardRequestRepository
	certLookup      inventory.CertLookup
	certEnrichJob   *scheduler.CertEnrichJob // nil if PSA not configured
	arbSvc          arbitrage.Service
	portSvc         portfolio.Service
	tuningSvc       tuning.Service
	financeService  finance.Service
	exportService   export.Service
}

// initializeCampaignsService creates the campaigns service with all options
// wired, including price lookup and PSA cert lookup. It also creates the
// arbitrage, portfolio, and tuning services that delegate to the same
// repositories.
func initializeCampaignsService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	priceProvImpl pricing.PriceProvider,
	intelRepo *sqlite.MarketIntelligenceRepository,
	mmStore *sqlite.MarketMoversStore,
) campaignsInitResult {
	// Create individual stores instead of composite repository
	campaignStore := sqlite.NewCampaignStore(db.DB, logger)
	purchaseStore := sqlite.NewPurchaseStore(db.DB, logger)
	saleStore := sqlite.NewSaleStore(db.DB, logger)
	analyticsStore := sqlite.NewAnalyticsStore(db.DB, logger)
	financeStore := sqlite.NewFinanceStore(db.DB, logger)
	pricingStore := sqlite.NewPricingStore(db.DB, logger)
	dhStore := sqlite.NewDHStore(db.DB, logger)
	snapshotStore := sqlite.NewSnapshotStore(db.DB, logger)
	sellSheetStore := sqlite.NewSellSheetStore(db.DB, logger)

	priceLookupAdapter := pricelookup.NewAdapter(priceProvImpl)
	campaignOpts := []inventory.ServiceOption{
		inventory.WithPriceLookup(priceLookupAdapter),
		inventory.WithIDGenerator(uuid.NewString),
		inventory.WithMaxSnapshotRetries(cfg.SnapshotEnrich.MaxRetries),
	}

	// PSA cert lookup (optional)
	var certLookup inventory.CertLookup
	var certEnrichJobForSvc *scheduler.CertEnrichJob
	if cfg.Adapters.PSAToken != "" {
		psaClient := psa.NewClient(cfg.Adapters.PSAToken, logger)
		certAdapter := psa.NewCertAdapter(psaClient)
		certLookup = certAdapter
		campaignOpts = append(campaignOpts, inventory.WithCertLookup(certAdapter))
		// CertEnrichJob must be created before NewService so it can be injected via
		// WithCertEnrichEnqueuer. It will also be registered with the scheduler group below.
		certEnrichJobForSvc = scheduler.NewCertEnrichJob(certAdapter, purchaseStore, logger)
		campaignOpts = append(campaignOpts, inventory.WithCertEnrichEnqueuer(certEnrichJobForSvc))
		logger.Info(ctx, "PSA cert lookup and cert enrichment enabled")
	}

	// Card request repository (tracks certs without linked cards)
	cardRequestRepo := sqlite.NewCardRequestRepository(db.DB)

	// History recorders — track CL values and population changes during CSV imports.
	campaignOpts = append(campaignOpts,
		inventory.WithCLValueRecorder(snapshotStore),
		inventory.WithPopulationRecorder(snapshotStore),
	)

	if intelRepo != nil {
		campaignOpts = append(campaignOpts, inventory.WithIntelligenceRepo(intelRepo))
	}

	// Card Ladder comp analytics — CLSalesStore only needs *sql.DB (always available).
	clSalesStore := sqlite.NewCLSalesStore(db.DB)
	campaignOpts = append(campaignOpts, inventory.WithCompSummaryProvider(clSalesStore))

	// Market Movers search title enrichment for CSV export (optional).
	if mmStore != nil {
		mmAdapter := inventory.MMMappingFunc(func(ctx context.Context) (map[string]string, error) {
			mappings, err := mmStore.ListMappings(ctx)
			if err != nil {
				return nil, err
			}
			result := make(map[string]string, len(mappings))
			for _, m := range mappings {
				if m.SearchTitle != "" {
					result[m.SlabSerial] = m.SearchTitle
				}
			}
			return result, nil
		})
		campaignOpts = append(campaignOpts, inventory.WithMMMappings(mmAdapter))
	}

	campaignsService := inventory.NewService(
		campaignStore,  // CampaignRepository
		purchaseStore,  // PurchaseRepository
		saleStore,      // SaleRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		pricingStore,   // PricingRepository
		dhStore,        // DHRepository
		snapshotStore,  // SnapshotRepository
		campaignOpts...,
	)

	arbSvc := arbitrage.NewService(
		campaignStore,  // CampaignRepository
		purchaseStore,  // PurchaseRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		priceLookupAdapter,
		logger,
	)

	portSvc := portfolio.NewService(
		campaignStore,  // CampaignRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		logger,
	)

	tuningSvc := tuning.NewService(
		campaignStore,  // CampaignRepository
		analyticsStore, // AnalyticsRepository
		logger,
	)

	// Create export service
	var exportSvc export.Service
	exportOpts := []export.Option{
		export.WithLogger(logger),
	}
	if intelRepo != nil {
		exportOpts = append(exportOpts, export.WithIntelligenceRepo(intelRepo))
	}
	// Create a minimal composite wrapper to satisfy ExportReader interface
	exportReader := &struct {
		*sqlite.SellSheetStore
		*sqlite.PurchaseStore
		*sqlite.CampaignStore
	}{
		SellSheetStore: sellSheetStore,
		PurchaseStore:  purchaseStore,
		CampaignStore:  campaignStore,
	}
	exportSvc = export.New(exportReader, exportOpts...)

	// Create finance service
	financeSvc := finance.New(financeStore, uuid.NewString)

	return campaignsInitResult{
		service:         campaignsService,
		campaignStore:   campaignStore,
		purchaseStore:   purchaseStore,
		saleStore:       saleStore,
		analyticsStore:  analyticsStore,
		financeStore:    financeStore,
		pricingStore:    pricingStore,
		dhStore:         dhStore,
		snapshotStore:   snapshotStore,
		sellSheetStore:  sellSheetStore,
		cardRequestRepo: cardRequestRepo,
		certLookup:      certLookup,
		certEnrichJob:   certEnrichJobForSvc,
		arbSvc:          arbSvc,
		portSvc:         portSvc,
		tuningSvc:       tuningSvc,
		financeService:  financeSvc,
		exportService:   exportSvc,
	}
}

// initializeAdvisorService creates the Azure AI client and advisor service.
// All return values may be nil/zero if Azure AI is not configured. This is not
// an error.
func initializeAdvisorService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	aiCallRepo *sqlite.AICallRepository,
	campaignsService inventory.Service,
	scoringOpts []scoringadapter.ProviderOption,
	toolOpts ...advisortool.ExecutorOption,
) (llmProvider advisor.LLMProvider, advisorSvc advisor.Service, advisorCacheRepo *sqlite.AdvisorCacheRepository, err error) {
	if cfg.Adapters.AzureAIEndpoint == "" || cfg.Adapters.AzureAIKey == "" {
		return nil, nil, nil, nil
	}

	client, err := azureai.NewClient(azureai.Config{
		Endpoint:       cfg.Adapters.AzureAIEndpoint,
		APIKey:         cfg.Adapters.AzureAIKey,
		DeploymentName: cfg.Adapters.AzureAIDeployment,
	}, azureai.WithLogger(logger))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("initialize azure ai client: %w", err)
	}
	llmProvider = client

	toolExec := advisortool.NewCampaignToolExecutor(campaignsService, toolOpts...)
	advisorCacheRepo = sqlite.NewAdvisorCacheRepository(db.DB, logger)
	advisorOpts := []advisor.ServiceOption{
		advisor.WithLogger(logger),
		advisor.WithAITracker(aiCallRepo),
		advisor.WithCacheStore(advisorCacheRepo),
	}
	if cfg.AdvisorRefresh.MaxToolRounds > 0 {
		advisorOpts = append(advisorOpts, advisor.WithMaxToolRounds(cfg.AdvisorRefresh.MaxToolRounds))
	}

	// Scoring engine: pre-compute factor scores for advisor flows
	scoringProvider := scoringadapter.NewProvider(campaignsService, scoringOpts...)
	advisorOpts = append(advisorOpts, advisor.WithScoringDataProvider(scoringProvider))

	// Data gap tracking for scoring quality reports
	advisorOpts = append(advisorOpts, advisor.WithGapStore(sqlite.NewGapStore(db.DB)))

	advisorSvc = advisor.NewService(client, toolExec, advisorOpts...)
	logger.Info(ctx, "AI advisor initialized",
		observability.String("deployment", cfg.Adapters.AzureAIDeployment))

	return llmProvider, advisorSvc, advisorCacheRepo, nil
}

// initializeSocialService creates the social content service, including
// optional Instagram integration when configured.
type socialServiceResult struct {
	service             social.Service
	repo                *sqlite.SocialRepository
	igClient            *igclient.Client
	igStore             *sqlite.InstagramStore
	igTokenRefresher    scheduler.InstagramTokenRefresher
	publisherConfigured bool
}

func initializeSocialService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	azureAIClient advisor.LLMProvider,
	aiCallRepo *sqlite.AICallRepository,
) socialServiceResult {
	socialRepo := sqlite.NewSocialRepository(db.DB)
	socialOpts := []social.ServiceOption{
		social.WithLogger(logger),
		social.WithAITracker(aiCallRepo),
	}

	// Use a separate model for social content if SOCIAL_AI_DEPLOYMENT is configured.
	socialLLM := azureAIClient
	if cfg.Adapters.SocialAIDeployment != "" && cfg.Adapters.SocialAIDeployment != cfg.Adapters.AzureAIDeployment {
		socialClient, socialErr := azureai.NewClient(azureai.Config{
			Endpoint:       cfg.Adapters.AzureAIEndpoint,
			APIKey:         cfg.Adapters.AzureAIKey,
			DeploymentName: cfg.Adapters.SocialAIDeployment,
		}, azureai.WithLogger(logger))
		if socialErr != nil {
			logger.Warn(ctx, "social AI client init failed, falling back to advisor model",
				observability.Err(socialErr))
		} else {
			socialLLM = socialClient
			logger.Info(ctx, "social content using separate model",
				observability.String("deployment", cfg.Adapters.SocialAIDeployment))
		}
	}
	if socialLLM != nil {
		socialOpts = append(socialOpts, social.WithLLM(socialLLM))
	}

	// Initialize image generation if enabled
	initImageGeneration(ctx, cfg, logger, &socialOpts)

	// Initialize Instagram integration (requires encryption + Instagram config)
	igConfig := config.LoadInstagramOAuthConfig()
	var igClient *igclient.Client
	var igStore *sqlite.InstagramStore
	var igTokenRefresher scheduler.InstagramTokenRefresher
	var publisherConfigured bool

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
			publisherConfigured = true

			igTokenRefresher = igclient.NewTokenRefresher(igClient, igStore, logger)
			logger.Info(ctx, "Instagram integration initialized")
		}
	}

	socialService := social.NewService(socialRepo, socialOpts...)

	return socialServiceResult{
		service:             socialService,
		repo:                socialRepo,
		igClient:            igClient,
		igStore:             igStore,
		igTokenRefresher:    igTokenRefresher,
		publisherConfigured: publisherConfigured,
	}
}

// initImageGeneration configures image generation if enabled and all prerequisites are met.
func initImageGeneration(ctx context.Context, cfg *config.Config, logger observability.Logger, socialOpts *[]social.ServiceOption) {
	if !cfg.Adapters.ImageAIEnabled || cfg.Adapters.ImageAIDeployment == "" ||
		cfg.Adapters.AzureAIEndpoint == "" || cfg.Adapters.AzureAIKey == "" {
		return
	}

	imgClient, imgErr := azureai.NewImageClient(azureai.Config{
		Endpoint:       cfg.Adapters.AzureAIEndpoint,
		APIKey:         cfg.Adapters.AzureAIKey,
		DeploymentName: cfg.Adapters.ImageAIDeployment,
	}, azureai.WithImageLogger(logger))
	if imgErr != nil {
		logger.Warn(ctx, "image generation client init failed", observability.Err(imgErr))
		return
	}

	mediaDir := os.Getenv("MEDIA_DIR")
	if mediaDir == "" {
		mediaDir = "./data/media"
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		logger.Warn(ctx, "BASE_URL not set; AI background generation disabled (cannot construct public URLs)")
		return
	}

	*socialOpts = append(*socialOpts, social.WithImageGenerator(imgClient, cfg.Adapters.ImageAIQuality, mediaDir, baseURL))
	*socialOpts = append(*socialOpts, social.WithMediaStore(mediafs.NewStore(mediaDir)))
	logger.Info(ctx, "AI background generation enabled",
		observability.String("deployment", cfg.Adapters.ImageAIDeployment),
		observability.String("quality", cfg.Adapters.ImageAIQuality))
}

// initializeMetricsPoller creates the metrics repository and, when an Instagram
// client and token store are available, the insights poller adapter.
func initializeMetricsPoller(
	ctx context.Context,
	db *sqlite.DB,
	igClient *igclient.Client,
	igStore *sqlite.InstagramStore,
	logger observability.Logger,
) (*sqlite.MetricsRepository, social.InsightsPoller) {
	metricsRepo := sqlite.NewMetricsRepository(db.DB)
	var poller social.InsightsPoller
	if igClient != nil && igStore != nil {
		poller = igclient.NewInsightsPollerAdapter(igClient, igStore)
		logger.Info(ctx, "Instagram insights poller initialized")
	}
	return metricsRepo, poller
}

// initializeCardLadder creates the Card Ladder client, auth, and store.
// Returns nil values if encryption key is not configured.
func initializeCardLadder(
	ctx context.Context,
	logger observability.Logger,
	db *sqlite.DB,
	encryptor crypto.Encryptor,
) (*cardladder.Client, *cardladder.FirebaseAuth, *sqlite.CardLadderStore) {
	if encryptor == nil {
		logger.Info(ctx, "Card Ladder disabled: encryption key not configured")
		return nil, nil, nil
	}

	store := sqlite.NewCardLadderStore(db.DB, encryptor)

	// Try to load existing config to set up the client
	clCfg, err := store.GetConfig(ctx)
	if err != nil {
		logger.Warn(ctx, "failed to load Card Ladder config", observability.Err(err))
	}

	if clCfg == nil {
		logger.Info(ctx, "Card Ladder not configured; use POST /api/admin/cardladder/config to set up")
		return nil, nil, store
	}

	fbAuth := cardladder.NewFirebaseAuth(clCfg.FirebaseAPIKey)
	client := cardladder.NewClient(
		cardladder.WithTokenManager(fbAuth, clCfg.RefreshToken, time.Time{}),
	)
	logger.Info(ctx, "Card Ladder client initialized",
		observability.Bool("hasEmail", clCfg.Email != ""),
		observability.String("collectionId", clCfg.CollectionID))

	return client, fbAuth, store
}

// initializeMarketMovers creates the Market Movers client, auth, and store.
// Returns nil client if encryption key is not configured or if not yet configured.
func initializeMarketMovers(
	ctx context.Context,
	logger observability.Logger,
	db *sqlite.DB,
	encryptor crypto.Encryptor,
) (*marketmovers.Client, *sqlite.MarketMoversStore) {
	if encryptor == nil {
		logger.Info(ctx, "Market Movers disabled: encryption key not configured")
		return nil, nil
	}

	store := sqlite.NewMarketMoversStore(db.DB, encryptor)

	// Try to load existing config to set up the client
	mmCfg, err := store.GetConfig(ctx)
	if err != nil {
		logger.Warn(ctx, "failed to load Market Movers config", observability.Err(err))
	}

	if mmCfg == nil {
		logger.Info(ctx, "Market Movers not configured; use POST /api/admin/marketmovers/config to set up")
		return nil, store
	}

	mmAuth := marketmovers.NewAuth()
	client := marketmovers.NewClient(
		marketmovers.WithTokenManager(mmAuth, mmCfg.RefreshToken, time.Time{}),
	)
	logger.Info(ctx, "Market Movers client initialized",
		observability.Bool("hasUsername", mmCfg.Username != ""))

	return client, store
}

// schedulerDeps bundles all dependencies needed by initializeSchedulers.
type schedulerDeps struct {
	Config               *config.Config
	Logger               observability.Logger
	DBTracker            *sqlite.DBTracker
	RefreshCandidates    pricing.RefreshCandidateProvider
	PriceProvImpl        pricing.PriceProvider
	CardProvImpl         *tcgdex.TCGdex
	AuthService          auth.Service
	SyncStateRepo        *sqlite.SyncStateRepository
	CardIDMappingRepo    *sqlite.CardIDMappingRepository
	CampaignStore        *sqlite.CampaignStore
	PurchaseStore        *sqlite.PurchaseStore
	DHStore              *sqlite.DHStore
	SnapshotStore        *sqlite.SnapshotStore
	CampaignsService     inventory.Service
	CertLookup           inventory.CertLookup
	CertEnrichJob        *scheduler.CertEnrichJob // pre-built; nil if PSA not configured
	AdvisorService       advisor.Service
	AdvisorCacheRepo     *sqlite.AdvisorCacheRepository
	AICallRepo           *sqlite.AICallRepository
	SocialService        social.Service
	SocialRepo           *sqlite.SocialRepository
	PublisherConfigured  bool
	IGTokenRefresher     scheduler.InstagramTokenRefresher
	MetricsPostLister    social.MetricsPostLister
	MetricsSaver         social.MetricsSaver
	InsightsPoller       social.InsightsPoller
	PicksService         picks.Service
	CardLadderClient     *cardladder.Client
	CardLadderStore      *sqlite.CardLadderStore
	CardLadderSalesStore *sqlite.CLSalesStore
	MMClient             *marketmovers.Client
	MMStore              *sqlite.MarketMoversStore
	DHClient             *dh.Client
	DHIntelligenceRepo   *sqlite.MarketIntelligenceRepository
	DHSuggestionsRepo    *sqlite.DHSuggestionsRepository
	GapStore             *sqlite.GapStore
	PSASheetFetcher      scheduler.SheetFetcher
	PSASpreadsheetID     string
	PSATabName           string
}

// initializeSchedulers builds and starts the scheduler group, returning the
// result and a cancel function to shut them down.
func initializeSchedulers(ctx context.Context, deps schedulerDeps) (*scheduler.BuildResult, context.CancelFunc) {
	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	buildDeps := scheduler.BuildDeps{
		APITracker:            deps.DBTracker,
		HealthChecker:         deps.DBTracker,
		AccessTracker:         deps.DBTracker,
		RefreshCandidates:     deps.RefreshCandidates,
		PriceProvider:         deps.PriceProvImpl,
		CardProvider:          deps.CardProvImpl,
		AuthService:           deps.AuthService,
		Logger:                deps.Logger,
		SyncStateStore:        deps.SyncStateRepo,
		NewSetsProvider:       deps.CardProvImpl.RegistryManager(),
		InventoryLister:       &inventoryListAdapter{repo: deps.PurchaseStore},
		SnapshotRefresher:     &snapshotRefreshAdapter{svc: deps.CampaignsService},
		SnapshotEnrichService: deps.CampaignsService,
		SnapshotHistoryLister: &struct {
			*sqlite.SnapshotStore
			*sqlite.PurchaseStore
		}{
			SnapshotStore: deps.SnapshotStore,
			PurchaseStore: deps.PurchaseStore,
		},
		SnapshotHistoryRecorder:  deps.SnapshotStore,
		AdvisorCollector:         deps.AdvisorService,
		AdvisorCache:             deps.AdvisorCacheRepo,
		AICallTracker:            deps.AICallRepo,
		SocialContentDetector:    deps.SocialService,
		InstagramTokenRefresher:  deps.IGTokenRefresher,
		MetricsPostLister:        deps.MetricsPostLister,
		MetricsSaver:             deps.MetricsSaver,
		InsightsPoller:           deps.InsightsPoller,
		PicksGenerator:           deps.PicksService,
		CardLadderClient:         deps.CardLadderClient,
		CardLadderStore:          deps.CardLadderStore,
		CardLadderPurchaseLister: deps.PurchaseStore,
		CardLadderValueUpdater:   deps.PurchaseStore,
		CardLadderGemRateUpdater: deps.PurchaseStore,
		CardLadderSyncUpdater:    deps.PurchaseStore,
		CardLadderCLRecorder:     deps.SnapshotStore,
		CardLadderSalesStore:     deps.CardLadderSalesStore,
	}
	// Wire Market Movers (nil-safe: only set if non-nil to avoid typed-nil interface issues)
	if deps.MMClient != nil {
		buildDeps.MMClient = deps.MMClient
	}
	if deps.MMStore != nil {
		buildDeps.MMStore = deps.MMStore
	}
	if deps.PurchaseStore != nil {
		buildDeps.MMPurchaseLister = deps.PurchaseStore
		buildDeps.MMValueUpdater = deps.PurchaseStore
	}
	// Nil-safe interface conversion for DH dependencies.
	if deps.DHClient != nil {
		buildDeps.DHClient = deps.DHClient
		buildDeps.DHOrdersClient = deps.DHClient
		buildDeps.DHInventoryListClient = deps.DHClient
		// Guard against typed-nil pointers: use individual stores instead of composite repo.
		if deps.PurchaseStore != nil {
			buildDeps.DHFieldsUpdater = deps.PurchaseStore
			buildDeps.PurchaseByCertLookup = deps.PurchaseStore
			buildDeps.DHPushPendingLister = deps.PurchaseStore
			buildDeps.DHPushStatusUpdater = deps.PurchaseStore
			buildDeps.DHPushCandidatesSaver = deps.PurchaseStore
			buildDeps.DHPushHoldSetter = deps.PurchaseStore
		}
		if deps.DHStore != nil {
			buildDeps.DHPushConfigLoader = deps.DHStore
		}
		if deps.CardIDMappingRepo != nil {
			buildDeps.DHPushCardIDSaver = deps.CardIDMappingRepo
		}
		if deps.CampaignsService != nil {
			buildDeps.CampaignService = deps.CampaignsService
		}
	}
	if deps.DHIntelligenceRepo != nil {
		buildDeps.DHIntelligenceRepo = deps.DHIntelligenceRepo
	}
	if deps.DHSuggestionsRepo != nil {
		buildDeps.DHSuggestionsRepo = deps.DHSuggestionsRepo
	}
	if deps.GapStore != nil {
		buildDeps.GapStore = deps.GapStore
	}
	// Wire PSA sync (nil-safe)
	if deps.PSASheetFetcher != nil {
		buildDeps.PSASheetFetcher = deps.PSASheetFetcher
		buildDeps.PSAImporter = deps.CampaignsService
		buildDeps.PSASpreadsheetID = deps.PSASpreadsheetID
		buildDeps.PSATabName = deps.PSATabName
	}

	// Wire cert enrichment (nil-safe)
	if deps.CertLookup != nil && deps.PurchaseStore != nil {
		buildDeps.CertLookup = deps.CertLookup
		buildDeps.PurchaseRepo = deps.PurchaseStore
		buildDeps.CampaignService = deps.CampaignsService
	}
	// Pass pre-built CertEnrichJob so the scheduler group uses the same instance
	// that was injected into inventory.service via WithCertEnrichEnqueuer.
	if deps.CertEnrichJob != nil {
		buildDeps.CertEnrichJobPrebuilt = deps.CertEnrichJob
	}

	// Wire crack cache refresh (uses inventory service for RefreshCrackCandidates).
	if deps.CampaignsService != nil {
		buildDeps.CrackCacheService = deps.CampaignsService
	}

	// Wire social publish (nil-safe: only set if publisher was actually configured)
	if deps.PublisherConfigured {
		buildDeps.SocialPublisher = deps.SocialService
		buildDeps.SocialPublishRepo = deps.SocialRepo
	}
	schedulerResult := scheduler.BuildGroup(deps.Config, buildDeps)
	schedulerResult.Group.StartAll(schedulerCtx)

	return &schedulerResult, cancelScheduler
}
