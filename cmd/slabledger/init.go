package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/azureai"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/doubleholo"
	"github.com/guarzo/slabledger/internal/adapters/clients/fusionprice"
	igclient "github.com/guarzo/slabledger/internal/adapters/clients/instagram"
	"github.com/guarzo/slabledger/internal/adapters/clients/justtcg"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricelookup"
	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/mediafs"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// initializePriceProviders creates the PriceCharting and CardHedger clients
// and wires them together into the FusionProvider.
func initializePriceProviders(
	ctx context.Context,
	cfg *config.Config,
	appCache cache.Cache,
	logger observability.Logger,
	cardProvImpl *tcgdex.TCGdex,
	priceRepo *sqlite.PriceRepository,
	cardIDMappingRepo *sqlite.CardIDMappingRepository,
	dhClient *doubleholo.Client,
	intelRepo *sqlite.MarketIntelligenceRepository,
) (priceProvider *fusionprice.FusionPriceProvider, cardHedgerClient *cardhedger.Client, pcProvider *pricecharting.PriceCharting, err error) {
	pcProvider, err = pricecharting.NewPriceCharting(
		pricecharting.DefaultConfig(cfg.Adapters.PriceChartingToken), appCache, logger,
		pricecharting.WithHintResolver(cardIDMappingRepo),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize PriceCharting provider: %w", err)
	}

	cardHedgerClient = cardhedger.NewClient(cfg.Adapters.CardHedgerKey, cardhedger.WithLogger(logger))

	// Wrap secondary clients in adapters that implement fusion.SecondaryPriceSource
	secondarySources := []fusion.SecondaryPriceSource{
		fusionprice.NewCardHedgerAdapter(cardHedgerClient, cardIDMappingRepo, logger,
			fusionprice.WithCardHedgerHintResolver(cardIDMappingRepo)),
	}

	// Add DoubleHolo as a secondary fusion source if available
	dhAvailable := false
	if dhClient != nil && dhClient.Available() {
		dhOpts := []fusionprice.DoubleHoloAdapterOption{}
		if intelRepo != nil {
			dhOpts = append(dhOpts, fusionprice.WithDHIntelligenceStore(intelRepo))
		}
		dhAdapter := fusionprice.NewDoubleHoloAdapter(dhClient, cardIDMappingRepo, logger, dhOpts...)
		secondarySources = append(secondarySources, dhAdapter)
		dhAvailable = true
		logger.Info(ctx, "DoubleHolo adapter registered as secondary fusion source")
	}

	priceProvider = fusionprice.NewFusionProviderWithRepo(
		pcProvider, secondarySources,
		appCache, priceRepo, priceRepo, priceRepo, logger,
		fusionprice.DefaultFreshnessDuration,
		cfg.Fusion.CacheTTL,
		cfg.Fusion.PriceChartingTimeout,
		cfg.Fusion.SecondarySourceTimeout,
		fusionprice.WithCardProvider(cardProvImpl),
	)
	logger.Info(ctx, "Fusion price provider initialized",
		observability.Int("secondary_sources", len(secondarySources)),
		observability.Bool("cardhedger_available", cardHedgerClient.Available()),
		observability.Bool("doubleholo_available", dhAvailable))

	return priceProvider, cardHedgerClient, pcProvider, nil
}

// initializeCampaignsService creates the campaigns service with all options
// wired, including price lookup, PSA cert lookup, and CardHedger cert resolver.
func initializeCampaignsService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	priceProvImpl *fusionprice.FusionPriceProvider,
	cardHedgerClientImpl *cardhedger.Client,
	cardIDMappingRepo *sqlite.CardIDMappingRepository,
) (campaigns.Service, *sqlite.CampaignsRepository, *sqlite.CardRequestRepository) {
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

	return campaignsService, campaignsRepo, cardRequestRepo
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
	campaignsService campaigns.Service,
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
	advisorSvc = advisor.NewService(client, toolExec, advisorOpts...)
	logger.Info(ctx, "AI advisor initialized",
		observability.String("deployment", cfg.Adapters.AzureAIDeployment))

	return llmProvider, advisorSvc, advisorCacheRepo, nil
}

// initializeSocialService creates the social content service, including
// optional Instagram integration when configured.
func initializeSocialService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	azureAIClient advisor.LLMProvider,
	aiCallRepo *sqlite.AICallRepository,
) (social.Service, *sqlite.SocialRepository, *igclient.Client, *sqlite.InstagramStore, scheduler.InstagramTokenRefresher) {
	socialRepo := sqlite.NewSocialRepository(db.DB)
	var socialOpts []social.ServiceOption
	socialOpts = append(socialOpts, social.WithLogger(logger))
	socialOpts = append(socialOpts, social.WithAITracker(aiCallRepo))

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
	if cfg.Adapters.ImageAIEnabled && cfg.Adapters.ImageAIDeployment != "" &&
		cfg.Adapters.AzureAIEndpoint != "" && cfg.Adapters.AzureAIKey != "" {
		imgClient, imgErr := azureai.NewImageClient(azureai.Config{
			Endpoint:       cfg.Adapters.AzureAIEndpoint,
			APIKey:         cfg.Adapters.AzureAIKey,
			DeploymentName: cfg.Adapters.ImageAIDeployment,
		}, azureai.WithImageLogger(logger))
		if imgErr != nil {
			logger.Warn(ctx, "image generation client init failed",
				observability.Err(imgErr))
		} else {
			mediaDir := os.Getenv("MEDIA_DIR")
			if mediaDir == "" {
				mediaDir = "./data/media"
			}
			baseURL := os.Getenv("BASE_URL")
			if baseURL == "" {
				logger.Warn(ctx, "BASE_URL not set; AI background generation disabled (cannot construct public URLs)")
			} else {
				socialOpts = append(socialOpts, social.WithImageGenerator(imgClient, cfg.Adapters.ImageAIQuality, mediaDir, baseURL))
				socialOpts = append(socialOpts, social.WithMediaStore(mediafs.NewStore(mediaDir)))
				logger.Info(ctx, "AI background generation enabled",
					observability.String("deployment", cfg.Adapters.ImageAIDeployment),
					observability.String("quality", cfg.Adapters.ImageAIQuality))
			}
		}
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

	socialService := social.NewService(socialRepo, socialOpts...)

	return socialService, socialRepo, igClient, igStore, igTokenRefresher
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

// schedulerDeps bundles all dependencies needed by initializeSchedulers.
type schedulerDeps struct {
	Config               *config.Config
	Logger               observability.Logger
	PriceRepo            *sqlite.PriceRepository
	PriceProvImpl        *fusionprice.FusionPriceProvider
	CardProvImpl         *tcgdex.TCGdex
	AuthService          auth.Service
	CardHedgerClientImpl *cardhedger.Client
	SyncStateRepo        *sqlite.SyncStateRepository
	CardIDMappingRepo    *sqlite.CardIDMappingRepository
	DiscoveryFailureRepo *sqlite.DiscoveryFailureRepository
	FavoritesRepo        *sqlite.FavoritesRepository
	CampaignsRepo        *sqlite.CampaignsRepository
	CampaignsService     campaigns.Service
	AdvisorService       advisor.Service
	AdvisorCacheRepo     *sqlite.AdvisorCacheRepository
	AICallRepo           *sqlite.AICallRepository
	SocialService        social.Service
	IGTokenRefresher     scheduler.InstagramTokenRefresher
	CertSweeper          scheduler.CertSweeper
	PicksService         picks.Service
	CardLadderClient     *cardladder.Client
	CardLadderStore      *sqlite.CardLadderStore
	CardLadderSalesStore *sqlite.CLSalesStore
	JustTCGClient        *justtcg.Client
	DHClient             *doubleholo.Client
	DHIntelligenceRepo   *sqlite.MarketIntelligenceRepository
	DHSuggestionsRepo    *sqlite.DHSuggestionsRepository
}

// initializeSchedulers builds and starts the scheduler group, returning the
// result (including CardDiscoverer) and a cancel function to shut them down.
func initializeSchedulers(ctx context.Context, deps schedulerDeps) (*scheduler.BuildResult, context.CancelFunc) {
	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	buildDeps := scheduler.BuildDeps{
		PriceRepo:                deps.PriceRepo,
		APITracker:               deps.PriceRepo,
		HealthChecker:            deps.PriceRepo,
		AccessTracker:            deps.PriceRepo,
		PriceProvider:            deps.PriceProvImpl,
		CardProvider:             deps.CardProvImpl,
		AuthService:              deps.AuthService,
		Logger:                   deps.Logger,
		CardHedgerClient:         deps.CardHedgerClientImpl,
		SyncStateStore:           deps.SyncStateRepo,
		CardIDMappingLookup:      deps.CardIDMappingRepo,
		CardIDMappingLister:      &cardIDMappingListAdapter{repo: deps.CardIDMappingRepo},
		CardIDMappingSaver:       deps.CardIDMappingRepo,
		DiscoveryFailureTracker:  deps.DiscoveryFailureRepo,
		FavoritesLister:          &favoritesListAdapter{repo: deps.FavoritesRepo},
		CampaignCardLister:       &campaignCardListAdapter{repo: deps.CampaignsRepo},
		NewSetsProvider:          deps.CardProvImpl.RegistryManager(),
		InventoryLister:          &inventoryListAdapter{repo: deps.CampaignsRepo},
		SnapshotRefresher:        &snapshotRefreshAdapter{svc: deps.CampaignsService},
		SnapshotEnrichService:    deps.CampaignsService,
		SnapshotHistoryLister:    deps.CampaignsRepo,
		SnapshotHistoryRecorder:  deps.CampaignsRepo,
		AdvisorCollector:         deps.AdvisorService,
		AdvisorCache:             deps.AdvisorCacheRepo,
		AICallTracker:            deps.AICallRepo,
		SocialContentDetector:    deps.SocialService,
		InstagramTokenRefresher:  deps.IGTokenRefresher,
		CertSweeper:              deps.CertSweeper,
		PicksGenerator:           deps.PicksService,
		CardLadderClient:         deps.CardLadderClient,
		CardLadderStore:          deps.CardLadderStore,
		CardLadderPurchaseLister: deps.CampaignsRepo,
		CardLadderValueUpdater:   deps.CampaignsRepo,
		CardLadderCLRecorder:     deps.CampaignsRepo,
		CardLadderSalesStore:     deps.CardLadderSalesStore,
	}
	// Nil-safe interface conversion: a nil *justtcg.Client assigned to an interface
	// produces a non-nil interface wrapping a nil pointer, which breaks nil checks.
	if deps.JustTCGClient != nil {
		buildDeps.JustTCGClient = deps.JustTCGClient
	}
	// Same nil-safety for DoubleHolo dependencies.
	if deps.DHClient != nil {
		buildDeps.DHClient = deps.DHClient
	}
	if deps.DHIntelligenceRepo != nil {
		buildDeps.DHIntelligenceRepo = deps.DHIntelligenceRepo
	}
	if deps.DHSuggestionsRepo != nil {
		buildDeps.DHSuggestionsRepo = deps.DHSuggestionsRepo
	}
	schedulerResult := scheduler.BuildGroup(deps.Config, buildDeps)
	schedulerResult.Group.StartAll(schedulerCtx)

	return &schedulerResult, cancelScheduler
}
