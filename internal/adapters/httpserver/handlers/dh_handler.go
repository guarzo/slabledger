package handlers

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHMatchClient is the subset of the DH client needed for card matching.
type DHMatchClient interface {
	Match(ctx context.Context, title, sku string) (*dh.MatchResponse, error)
	Available() bool
}

// DHCardIDSaver reads and writes DH card ID mappings.
type DHCardIDSaver interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	GetMappedSet(ctx context.Context, provider string) (map[string]string, error)
}

// DHPurchaseLister lists and retrieves purchases for DH operations.
type DHPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
	GetPurchase(ctx context.Context, id string) (*campaigns.Purchase, error)
}

// DHInventoryPusher pushes inventory items to DH.
type DHInventoryPusher interface {
	PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}

// DHFieldsUpdater persists DH tracking fields on local purchases.
type DHFieldsUpdater interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, update campaigns.DHFieldsUpdate) error
}

// DHPushStatusUpdater sets the DH push pipeline status on a purchase.
type DHPushStatusUpdater interface {
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
}

// DHIntelligenceCounter returns aggregate stats for market intelligence.
type DHIntelligenceCounter interface {
	CountAll(ctx context.Context) (int, error)
	LatestFetchedAt(ctx context.Context) (string, error)
}

// DHSuggestionsCounter returns aggregate stats for DH suggestions.
type DHSuggestionsCounter interface {
	CountLatest(ctx context.Context) (int, error)
	LatestFetchedAt(ctx context.Context) (string, error)
}

// DHStatusCounter returns DH push status counts without loading full purchase data.
type DHStatusCounter interface {
	CountUnsoldByDHPushStatus(ctx context.Context) (map[string]int, error)
}

// DHHealthReporter provides API health metrics.
type DHHealthReporter interface {
	Health() *dh.HealthTracker
}

// DHCountsFetcher retrieves inventory and order counts from DH.
type DHCountsFetcher interface {
	ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
	GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
}

// DHHandler handles DH bulk match, export, intelligence, and suggestions endpoints.
type DHHandler struct {
	matchClient       DHMatchClient
	cardIDSaver       DHCardIDSaver
	purchaseLister    DHPurchaseLister
	inventoryPusher   DHInventoryPusher   // optional: pushes matched cards to DH inventory
	dhFieldsUpdater   DHFieldsUpdater     // optional: persists DH inventory IDs after push
	pushStatusUpdater DHPushStatusUpdater // optional: sets dh_push_status after bulk match
	statusCounter     DHStatusCounter     // optional: efficient push status counts
	intelRepo         intelligence.Repository
	suggestionsRepo   intelligence.SuggestionsRepository
	intelCounter      DHIntelligenceCounter
	suggestCounter    DHSuggestionsCounter
	logger            observability.Logger
	baseCtx           context.Context
	healthReporter    DHHealthReporter // optional: API health metrics
	countsFetcher     DHCountsFetcher  // optional: DH inventory/order counts

	bgWG             sync.WaitGroup
	bulkMatchMu      sync.Mutex
	bulkMatchRunning atomic.Bool
}

// NewDHHandler creates a new DHHandler with the given dependencies.
// baseCtx is a server-lifecycle context; background goroutines derive from it.
// healthReporter and countsFetcher are optional (nil-safe).
func NewDHHandler(
	matchClient DHMatchClient,
	cardIDSaver DHCardIDSaver,
	purchaseLister DHPurchaseLister,
	inventoryPusher DHInventoryPusher,
	dhFieldsUpdater DHFieldsUpdater,
	pushStatusUpdater DHPushStatusUpdater,
	statusCounter DHStatusCounter,
	intelRepo intelligence.Repository,
	suggestionsRepo intelligence.SuggestionsRepository,
	intelCounter DHIntelligenceCounter,
	suggestCounter DHSuggestionsCounter,
	logger observability.Logger,
	baseCtx context.Context,
	healthReporter DHHealthReporter,
	countsFetcher DHCountsFetcher,
) *DHHandler {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	return &DHHandler{
		matchClient:       matchClient,
		cardIDSaver:       cardIDSaver,
		purchaseLister:    purchaseLister,
		inventoryPusher:   inventoryPusher,
		dhFieldsUpdater:   dhFieldsUpdater,
		pushStatusUpdater: pushStatusUpdater,
		statusCounter:     statusCounter,
		intelRepo:         intelRepo,
		suggestionsRepo:   suggestionsRepo,
		intelCounter:      intelCounter,
		suggestCounter:    suggestCounter,
		logger:            logger,
		baseCtx:           baseCtx,
		healthReporter:    healthReporter,
		countsFetcher:     countsFetcher,
	}
}

// Wait blocks until all background goroutines (e.g. bulk match) have completed.
// Call during graceful shutdown to avoid writing to a closed database.
func (h *DHHandler) Wait() { h.bgWG.Wait() }

// Compile-time checks.
var _ DHInventoryPusher = (*dh.Client)(nil)
var _ DHHealthReporter = (*dh.Client)(nil)
var _ DHCountsFetcher = (*dh.Client)(nil)
