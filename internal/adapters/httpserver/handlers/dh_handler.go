package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHCertResolver resolves PSA certs to DH card IDs via the enterprise API.
type DHCertResolver interface {
	ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}

// DHCardIDSaver reads and writes DH card ID mappings.
type DHCardIDSaver interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	GetMappedSet(ctx context.Context, provider string) (map[string]string, error)
}

// DHPurchaseLister lists and retrieves purchases for DH operations.
type DHPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error)
	GetPurchase(ctx context.Context, id string) (*inventory.Purchase, error)
}

// DHInventoryPusher pushes inventory items to DH.
type DHInventoryPusher interface {
	PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}

// DHFieldsUpdater persists DH tracking fields on local purchases.
type DHFieldsUpdater interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error
}

// DHPushStatusUpdater sets the DH push pipeline status on a purchase.
type DHPushStatusUpdater interface {
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
}

// DHCandidatesSaver stores DH cert resolution candidates on a purchase.
type DHCandidatesSaver interface {
	UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error
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

// DHMatchConfirmer confirms correct card matches with DH so the system learns.
type DHMatchConfirmer interface {
	ConfirmMatch(ctx context.Context, req dh.ConfirmMatchRequest) (*dh.ConfirmMatchResponse, error)
}

// DHApproveService approves held DH push items and manages push config.
type DHApproveService interface {
	ApproveDHPush(ctx context.Context, purchaseID string) error
	GetDHPushConfig(ctx context.Context) (*inventory.DHPushConfig, error)
	SaveDHPushConfig(ctx context.Context, cfg *inventory.DHPushConfig) error
}

// DHCountsFetcher retrieves inventory and order counts from DH.
type DHCountsFetcher interface {
	ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
	GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
}

// DHHandler handles DH bulk match, export, intelligence, and suggestions endpoints.
type DHHandler struct {
	certResolver      DHCertResolver
	cardIDSaver       DHCardIDSaver
	purchaseLister    DHPurchaseLister
	inventoryPusher   DHInventoryPusher   // optional: pushes matched cards to DH inventory
	dhFieldsUpdater   DHFieldsUpdater     // optional: persists DH inventory IDs after push
	pushStatusUpdater DHPushStatusUpdater // optional: sets dh_push_status after bulk match
	candidatesSaver   DHCandidatesSaver   // optional: stores ambiguous candidates
	statusCounter     DHStatusCounter     // optional: efficient push status counts
	intelRepo         intelligence.Repository
	suggestionsRepo   intelligence.SuggestionsRepository
	intelCounter      DHIntelligenceCounter
	suggestCounter    DHSuggestionsCounter
	logger            observability.Logger
	baseCtx           context.Context
	healthReporter    DHHealthReporter // optional: API health metrics
	countsFetcher     DHCountsFetcher  // optional: DH inventory/order counts
	dhApproveService  DHApproveService // optional: approve held pushes + push config
	matchConfirmer    DHMatchConfirmer // optional: confirms matches with DH for learning

	bgWG             sync.WaitGroup
	bulkMatchMu      sync.Mutex
	bulkMatchRunning atomic.Bool
	bulkMatchError   atomic.Value // stores last bulk match error string (or "")
	selectLocks      sync.Map     // per-purchaseID mutex for select-match serialization
}

// selectMatchLock returns a per-purchase mutex for serializing select-match requests.
func (h *DHHandler) selectMatchLock(purchaseID string) *sync.Mutex {
	v, _ := h.selectLocks.LoadOrStore(purchaseID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// DHHandlerDeps holds all dependencies for constructing a DHHandler.
type DHHandlerDeps struct {
	CertResolver      DHCertResolver
	CardIDSaver       DHCardIDSaver
	PurchaseLister    DHPurchaseLister
	InventoryPusher   DHInventoryPusher   // optional: pushes matched cards to DH inventory
	DHFieldsUpdater   DHFieldsUpdater     // optional: persists DH inventory IDs after push
	PushStatusUpdater DHPushStatusUpdater // optional: sets dh_push_status after bulk match
	CandidatesSaver   DHCandidatesSaver   // optional: stores ambiguous candidates
	StatusCounter     DHStatusCounter     // optional: efficient push status counts
	IntelRepo         intelligence.Repository
	SuggestionsRepo   intelligence.SuggestionsRepository
	IntelCounter      DHIntelligenceCounter
	SuggestCounter    DHSuggestionsCounter
	Logger            observability.Logger
	BaseCtx           context.Context
	HealthReporter    DHHealthReporter // optional: API health metrics
	CountsFetcher     DHCountsFetcher  // optional: DH inventory/order counts
	DHApproveService  DHApproveService // optional: approve held pushes + push config
	MatchConfirmer    DHMatchConfirmer // optional: confirms matches with DH for learning
}

// NewDHHandler creates a new DHHandler with the given dependencies.
// BaseCtx is a server-lifecycle context; background goroutines derive from it.
// HealthReporter, CountsFetcher, and DHApproveService are optional (nil-safe).
func NewDHHandler(deps DHHandlerDeps) *DHHandler {
	if deps.BaseCtx == nil {
		deps.BaseCtx = context.Background()
	}
	h := &DHHandler{
		certResolver:      deps.CertResolver,
		cardIDSaver:       deps.CardIDSaver,
		purchaseLister:    deps.PurchaseLister,
		inventoryPusher:   deps.InventoryPusher,
		dhFieldsUpdater:   deps.DHFieldsUpdater,
		pushStatusUpdater: deps.PushStatusUpdater,
		candidatesSaver:   deps.CandidatesSaver,
		statusCounter:     deps.StatusCounter,
		intelRepo:         deps.IntelRepo,
		suggestionsRepo:   deps.SuggestionsRepo,
		intelCounter:      deps.IntelCounter,
		suggestCounter:    deps.SuggestCounter,
		logger:            deps.Logger,
		baseCtx:           deps.BaseCtx,
		healthReporter:    deps.HealthReporter,
		countsFetcher:     deps.CountsFetcher,
		dhApproveService:  deps.DHApproveService,
		matchConfirmer:    deps.MatchConfirmer,
	}
	h.bulkMatchError.Store("")
	return h
}

// Wait blocks until all background goroutines (e.g. bulk match) have completed.
// Call during graceful shutdown to avoid writing to a closed database.
func (h *DHHandler) Wait() { h.bgWG.Wait() }

// pushAndPersistDH builds an InventoryItem, pushes it to DH, and persists the DH fields.
// Returns the DH inventory ID on success. Errors are classified for callers:
//   - errDHPushNoInventoryID: push succeeded but no inventory ID was returned
//   - errDHPersistFailed: push succeeded but local persistence failed
//   - other errors: push API failure
func (h *DHHandler) pushAndPersistDH(ctx context.Context, purchase *inventory.Purchase, dhCardID, marketValueCents int) (int, error) {
	item := dh.InventoryItem{
		DHCardID:         dhCardID,
		CertNumber:       purchase.CertNumber,
		GradingCompany:   dh.GraderPSA,
		Grade:            purchase.GradeValue,
		CostBasisCents:   purchase.BuyCostCents,
		MarketValueCents: dh.IntPtr(marketValueCents),
		Status:           dh.InventoryStatusInStock,
	}

	pushResp, err := h.inventoryPusher.PushInventory(ctx, []dh.InventoryItem{item})
	if err != nil {
		return 0, err
	}

	for _, result := range pushResp.Results {
		if result.Status != "failed" && result.DHInventoryID != 0 {
			if h.dhFieldsUpdater != nil {
				if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, purchase.ID, inventory.DHFieldsUpdate{
					CardID:            dhCardID,
					InventoryID:       result.DHInventoryID,
					CertStatus:        dh.CertStatusMatched,
					ListingPriceCents: result.AssignedPriceCents,
					ChannelsJSON:      dh.MarshalChannels(result.Channels),
					DHStatus:          inventory.DHStatus(result.Status),
				}); err != nil {
					return 0, fmt.Errorf("%w: %v", errDHPersistFailed, err)
				}
			}
			return result.DHInventoryID, nil
		}
	}

	return 0, errDHPushNoInventoryID
}

var (
	errDHPushNoInventoryID = errors.New("DH push failed — no inventory ID returned")
	errDHPersistFailed     = errors.New("DH push succeeded but failed to save local state")
)

// Compile-time checks.
var _ DHCertResolver = (*dh.Client)(nil)
var _ DHInventoryPusher = (*dh.Client)(nil)
var _ DHHealthReporter = (*dh.Client)(nil)
var _ DHCountsFetcher = (*dh.Client)(nil)
var _ DHMatchConfirmer = (*dh.Client)(nil)
