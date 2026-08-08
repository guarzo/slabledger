package main

// init_services.go initializes optional external services: price providers and
// third-party integrations (Card Ladder). Core inventory/campaign services are
// initialized in init_inventory_services.go. Scheduler initialization is in
// init_schedulers.go.

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/dhprice"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// initializePriceProviders creates the DH price provider.
func initializePriceProviders(
	ctx context.Context,
	logger observability.Logger,
	cardIDMappingRepo *postgres.CardIDMappingRepository,
	dhClient *dh.Client,
	tombstoneRepo pricing.DHCardTombstoneRepo,
) (pricing.PriceProvider, error) {
	if dhClient == nil || !dhClient.EnterpriseAvailable() {
		logger.Warn(ctx, "DH client not available; price provider will be inactive")
		return dhprice.New(nil, nil), nil
	}
	opts := []dhprice.Option{dhprice.WithLogger(logger)}
	if tombstoneRepo != nil {
		opts = append(opts, dhprice.WithTombstoneRepo(tombstoneRepo))
	}
	provider := dhprice.New(dhClient, cardIDMappingRepo, opts...)
	logger.Info(ctx, "DH price provider initialized")
	return provider, nil
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
