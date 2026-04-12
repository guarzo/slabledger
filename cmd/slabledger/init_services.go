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
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	scoringadapter "github.com/guarzo/slabledger/internal/adapters/scoring"
	"github.com/guarzo/slabledger/internal/adapters/storage/mediafs"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/social"
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

// initializeCampaignsService creates the campaigns service with all options
// wired, including price lookup and PSA cert lookup.
func initializeCampaignsService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	priceProvImpl pricing.PriceProvider,
	intelRepo *sqlite.MarketIntelligenceRepository,
	mmStore *sqlite.MarketMoversStore,
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

	// Card request repository (tracks certs without linked cards)
	cardRequestRepo := sqlite.NewCardRequestRepository(db.DB)

	// History recorders — track CL values and population changes during CSV imports.
	campaignOpts = append(campaignOpts,
		campaigns.WithCLValueRecorder(campaignsRepo),
		campaigns.WithPopulationRecorder(campaignsRepo),
	)

	if intelRepo != nil {
		campaignOpts = append(campaignOpts, campaigns.WithIntelligenceRepo(intelRepo))
	}

	// Card Ladder comp analytics — CLSalesStore only needs *sql.DB (always available).
	clSalesStore := sqlite.NewCLSalesStore(db.DB)
	campaignOpts = append(campaignOpts, campaigns.WithCompSummaryProvider(clSalesStore))

	// Market Movers search title enrichment for CSV export (optional).
	if mmStore != nil {
		mmAdapter := campaigns.MMMappingFunc(func(ctx context.Context) (map[string]string, error) {
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
		campaignOpts = append(campaignOpts, campaigns.WithMMMappings(mmAdapter))
	}

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

	// Scoring engine: pre-compute factor scores for advisor flows
	scoringProvider := scoringadapter.NewProvider(campaignsService)
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
