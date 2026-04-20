package main

// init_services.go initializes optional external services: price providers, AI-powered services
// (advisor), and third-party integrations (Card Ladder, Market Movers). Core inventory/campaign
// services are initialized in init_inventory_services.go. Scheduler initialization is in
// init_schedulers.go.

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/azureai"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/dhprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	scoringadapter "github.com/guarzo/slabledger/internal/adapters/scoring"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// initializePriceProviders creates the DH price provider.
func initializePriceProviders(
	ctx context.Context,
	logger observability.Logger,
	cardIDMappingRepo *postgres.CardIDMappingRepository,
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

// initializeAdvisorService creates the Azure AI client and advisor service.
// All return values may be nil/zero if Azure AI is not configured. This is not
// an error.
func initializeAdvisorService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *postgres.DB,
	aiCallRepo *postgres.AICallRepository,
	campaignsService inventory.Service,
	scoringOpts []scoringadapter.ProviderOption,
	toolOpts ...advisortool.ExecutorOption,
) (llmProvider advisor.LLMProvider, advisorSvc advisor.Service, advisorCacheRepo *postgres.AdvisorCacheRepository, err error) {
	if cfg.Adapters.AzureAIEndpoint == "" || cfg.Adapters.AzureAIKey == "" {
		return nil, nil, nil, nil
	}

	client, err := azureai.NewClient(azureai.Config{
		Endpoint:       cfg.Adapters.AzureAIEndpoint,
		APIKey:         cfg.Adapters.AzureAIKey,
		DeploymentName: cfg.Adapters.AzureAIDeployment,
	}, azureai.WithLogger(logger), azureai.WithCompletionTimeout(cfg.Adapters.AzureAICompletionTimeout))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("initialize azure ai client: %w", err)
	}
	llmProvider = client

	toolExec := advisortool.NewCampaignToolExecutor(campaignsService, toolOpts...)
	advisorCacheRepo = postgres.NewAdvisorCacheRepository(db.DB, logger)
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
	advisorOpts = append(advisorOpts, advisor.WithGapStore(postgres.NewGapStore(db.DB)))

	advisorSvc = advisor.NewService(client, toolExec, advisorOpts...)
	logger.Info(ctx, "AI advisor initialized",
		observability.String("deployment", cfg.Adapters.AzureAIDeployment))

	return llmProvider, advisorSvc, advisorCacheRepo, nil
}

// initializeCardLadder creates the Card Ladder client, auth, and store.
// Returns nil values if encryption key is not configured.
func initializeCardLadder(
	ctx context.Context,
	logger observability.Logger,
	db *postgres.DB,
	encryptor crypto.Encryptor,
) (*cardladder.Client, *cardladder.FirebaseAuth, *postgres.CardLadderStore) {
	if encryptor == nil {
		logger.Info(ctx, "Card Ladder disabled: encryption key not configured")
		return nil, nil, nil
	}

	store := postgres.NewCardLadderStore(db.DB, encryptor)

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
	db *postgres.DB,
	encryptor crypto.Encryptor,
) (*marketmovers.Client, *postgres.MarketMoversStore) {
	if encryptor == nil {
		logger.Info(ctx, "Market Movers disabled: encryption key not configured")
		return nil, nil
	}

	store := postgres.NewMarketMoversStore(db.DB, encryptor)

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
