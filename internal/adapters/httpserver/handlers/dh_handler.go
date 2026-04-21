package handlers

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
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

// DHMappingDeleter removes auto-discovered card_id_mappings rows so a fresh
// DH resolve doesn't reuse a known-bad mapping on the next push cycle.
type DHMappingDeleter interface {
	DeleteAutoMapping(ctx context.Context, cardName, setName, collectorNumber, provider string) (int64, error)
}

// DHChannelDelister removes a DH inventory item from external sales channels.
// Used during unmatch to take down live eBay/Shopify listings before clearing
// local DH state.
type DHChannelDelister interface {
	DelistChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
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
	CountDHPipelineHealth(ctx context.Context) (inventory.DHPipelineHealth, error)
}

// DHPendingLister returns the list of received, unsold purchases pending DH push.
type DHPendingLister interface {
	ListDHPendingItems(ctx context.Context) ([]inventory.DHPendingItem, error)
}

// DHHealthReporter provides API health metrics.
type DHHealthReporter interface {
	Health() *dh.HealthTracker
}

// DHMatchConfirmer confirms correct card matches with DH so the system learns.
type DHMatchConfirmer interface {
	ConfirmMatch(ctx context.Context, req dh.ConfirmMatchRequest) (*dh.ConfirmMatchResponse, error)
}

// DHPSAImporter calls the DH PSA import endpoint for off-catalog certs.
// Satisfied by *dh.Client.
type DHPSAImporter interface {
	PSAImport(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error)
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

// DHOrdersIngester performs a one-shot ingest pass against DH /orders.
// Satisfied by *scheduler.DHOrdersPollScheduler.
type DHOrdersIngester interface {
	RunOnce(ctx context.Context, since string) (*scheduler.DHOrdersPollSummary, error)
}

// SyncStateReader reads sync state values. Satisfied by *postgres.SyncStateRepository.
type SyncStateReader interface {
	Get(ctx context.Context, key string) (string, error)
}

// EventCountsStore provides aggregate event counts. Satisfied by *postgres.DHEventStore.
type EventCountsStore interface {
	CountByTypeSince(ctx context.Context, t dhevents.Type, since time.Time) (int, error)
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
	mappingDeleter    DHMappingDeleter    // optional: removes auto card_id_mappings on unmatch
	channelDelister   DHChannelDelister   // optional: takes down live channel listings on unmatch
	statusCounter     DHStatusCounter     // optional: efficient push status counts
	pendingLister     DHPendingLister     // optional: lists DH pending pipeline items
	intelRepo         intelligence.Repository
	trajectoryRepo    intelligence.TrajectoryRepository // optional: weekly CL-lag trajectory
	suggestionsRepo   intelligence.SuggestionsRepository
	intelCounter      DHIntelligenceCounter
	suggestCounter    DHSuggestionsCounter
	logger            observability.Logger
	baseCtx           context.Context
	healthReporter    DHHealthReporter  // optional: API health metrics
	countsFetcher     DHCountsFetcher   // optional: DH inventory/order counts
	dhApproveService  DHApproveService  // optional: approve held pushes + push config
	matchConfirmer    DHMatchConfirmer  // optional: confirms matches with DH for learning
	psaImporter       DHPSAImporter     // optional: PSA import fallback for retry-match
	ordersIngester    DHOrdersIngester  // optional: POST /api/dh/ingest-orders manual trigger
	eventRec          dhevents.Recorder // optional: records DH state-change events
	syncStateReader   SyncStateReader   // optional: reads dh_orders_last_poll timestamp
	eventCountsStore  EventCountsStore  // optional: 24h event counts for orders ingest health

	reconciler dhlisting.Reconciler // optional: DH inventory reconciliation

	bgWG             sync.WaitGroup
	bulkMatchMu      sync.Mutex
	reconcileMu      sync.Mutex
	bulkMatchRunning atomic.Bool
	bulkMatchError   atomic.Value // stores last bulk match error string (or "")
	bulkMatchFailed  atomic.Int64 // failed count from last completed bulk match run
	bulkMatchMatched atomic.Int64 // matched count from last completed bulk match run
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
	MappingDeleter    DHMappingDeleter    // optional: removes auto card_id_mappings on unmatch
	ChannelDelister   DHChannelDelister   // optional: takes down live channel listings on unmatch
	StatusCounter     DHStatusCounter     // optional: efficient push status counts
	PendingLister     DHPendingLister     // optional: lists DH pending pipeline items
	IntelRepo         intelligence.Repository
	TrajectoryRepo    intelligence.TrajectoryRepository // optional: weekly CL-lag trajectory
	SuggestionsRepo   intelligence.SuggestionsRepository
	IntelCounter      DHIntelligenceCounter
	SuggestCounter    DHSuggestionsCounter
	Logger            observability.Logger
	BaseCtx           context.Context
	HealthReporter    DHHealthReporter     // optional: API health metrics
	CountsFetcher     DHCountsFetcher      // optional: DH inventory/order counts
	DHApproveService  DHApproveService     // optional: approve held pushes + push config
	MatchConfirmer    DHMatchConfirmer     // optional: confirms matches with DH for learning
	PSAImporter       DHPSAImporter        // optional: PSA import fallback for retry-match
	Reconciler        dhlisting.Reconciler // optional: DH inventory reconciliation
	OrdersIngester    DHOrdersIngester     // optional: enables POST /api/dh/ingest-orders
	EventRecorder     dhevents.Recorder    // optional: records DH state-change events
	SyncStateReader   SyncStateReader      // optional: reads dh_orders_last_poll timestamp
	EventCountsStore  EventCountsStore     // optional: 24h event counts for orders ingest health
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
		mappingDeleter:    deps.MappingDeleter,
		channelDelister:   deps.ChannelDelister,
		statusCounter:     deps.StatusCounter,
		pendingLister:     deps.PendingLister,
		intelRepo:         deps.IntelRepo,
		trajectoryRepo:    deps.TrajectoryRepo,
		suggestionsRepo:   deps.SuggestionsRepo,
		intelCounter:      deps.IntelCounter,
		suggestCounter:    deps.SuggestCounter,
		logger:            deps.Logger,
		baseCtx:           deps.BaseCtx,
		healthReporter:    deps.HealthReporter,
		countsFetcher:     deps.CountsFetcher,
		dhApproveService:  deps.DHApproveService,
		matchConfirmer:    deps.MatchConfirmer,
		psaImporter:       deps.PSAImporter,
		reconciler:        deps.Reconciler,
		ordersIngester:    deps.OrdersIngester,
		eventRec:          deps.EventRecorder,
		syncStateReader:   deps.SyncStateReader,
		eventCountsStore:  deps.EventCountsStore,
	}
	h.bulkMatchError.Store("")
	return h
}

// Wait blocks until all background goroutines (bulk match, best-effort DH
// follow-ups dispatched via dispatchBackground) have completed. Call during
// graceful shutdown to avoid writing to a closed database.
func (h *DHHandler) Wait() { h.bgWG.Wait() }

// DHBackgroundTimeout is the per-goroutine timeout applied to best-effort DH
// follow-ups (ConfirmMatch, DelistChannels) dispatched via dispatchBackground.
// Exported so the shutdown path can bound its Wait() call no shorter than the
// longest possible pending goroutine, preventing premature abandonment of
// in-flight work when the scheduler shutdown budget is smaller than this.
const DHBackgroundTimeout = 60 * time.Second

// dispatchBackground runs fn on a tracked goroutine with a decoupled context,
// so callers can return a response without waiting on best-effort follow-ups.
// The original ctx is used only for its values (logger/request scope) via
// context.WithoutCancel — cancellation from r.Context() does NOT propagate,
// since the request is already over by the time fn runs. DHBackgroundTimeout
// caps the detached work so a hung DH can't leak goroutines. Panics are
// recovered and logged. The WaitGroup is incremented synchronously before
// return so shutdown via Wait() always sees the pending goroutine.
func (h *DHHandler) dispatchBackground(ctx context.Context, op string, fn func(context.Context)) {
	bgCtx := context.WithoutCancel(ctx)
	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				h.logger.Error(bgCtx, "dh handler: background panic",
					observability.String("op", op),
					observability.String("panic", fmt.Sprintf("%v", r)),
					observability.String("stack", string(stack)))
			}
		}()
		runCtx, cancel := context.WithTimeout(bgCtx, DHBackgroundTimeout)
		defer cancel()
		fn(runCtx)
	}()
}

// recordEvent emits a DH state-change event to the recorder if present.
// Failures are logged but do not abort the calling operation.
func (h *DHHandler) recordEvent(ctx context.Context, e dhevents.Event) {
	if h.eventRec == nil {
		return
	}
	if err := h.eventRec.Record(ctx, e); err != nil {
		h.logger.Warn(ctx, "dh handler: record event failed",
			observability.String("type", string(e.Type)),
			observability.Err(err))
	}
}

// pushAndPersistDH builds an InventoryItem, pushes it to DH, and persists the DH fields.
// Returns the DH inventory ID on success. Errors are classified for callers:
//   - errDHPushNoInventoryID: push succeeded but no inventory ID was returned
//   - errDHPersistFailed: push succeeded but local persistence failed
//   - other errors: push API failure
func (h *DHHandler) pushAndPersistDH(ctx context.Context, purchase *inventory.Purchase, dhCardID, listingPriceCents int) (int, error) {
	if h.inventoryPusher == nil {
		return 0, fmt.Errorf("DH inventory pusher not configured")
	}
	item := dh.NewInStockItem(dhCardID, purchase.CertNumber, purchase.GradeValue, purchase.BuyCostCents, listingPriceCents)

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
var _ DHPSAImporter = (*dh.Client)(nil)
var _ DHChannelDelister = (*dh.Client)(nil)
