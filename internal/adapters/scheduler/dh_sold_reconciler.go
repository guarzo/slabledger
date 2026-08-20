package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHSoldReconcilerConfig controls the sold-status reconciler scheduler.
type DHSoldReconcilerConfig struct {
	Enabled  bool
	Interval time.Duration
}

// StaleDHStatusLister finds purchases with a linked sale but stale dh_status.
type StaleDHStatusLister interface {
	ListStaleDHStatusSoldPurchases(ctx context.Context) ([]string, error)
}

// DHStatusUpdater updates the dh_status column on a purchase.
type DHStatusUpdater interface {
	UpdatePurchaseDHStatus(ctx context.Context, id string, status string) error
}

// DHInventoryPurchaseResolver maps DH inventory IDs to their purchases. This is
// the sweep's identity key: the DH inventory ID names exactly one item on DH and
// exactly one purchase locally, whereas a cert number can match several
// purchases (sold, then re-acquired) with no way to tell which one DH means.
type DHInventoryPurchaseResolver interface {
	GetPurchasesByDHInventoryIDs(ctx context.Context, dhIDs []int) (map[int]*inventory.Purchase, error)
}

// dhSoldSweepStatuses are the DH-side statuses that mean "DH still believes
// this item is ours to sell". Anything sold locally but sitting in one of
// these on DH is drift, and `listed` in particular is live double-sale risk.
var dhSoldSweepStatuses = []string{dh.InventoryStatusListed, dh.InventoryStatusInStock}

// dhSaleRecoveryBatchSize bounds how many sales the recovery pass attempts per
// cycle, matching the paging bound the DH sweep already uses.
const dhSaleRecoveryBatchSize = 200

// DHSalesByPurchaseLister batch-loads sales for a set of purchases, keyed by
// purchase ID. The DH sweep uses it to find the sale behind an item DH still
// offers, so it can record a real price and date instead of the retired
// status PATCH.
type DHSalesByPurchaseLister interface {
	GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Sale, error)
}

// DHSaleHandleRecoveryLister finds sales DH has already accepted (or
// attempted) whose handle we failed to persist — the design §5b recovery
// scope. Keyed off local columns, not DH's inventory listing, so it keeps
// finding a row after DH has delisted the item.
type DHSaleHandleRecoveryLister interface {
	ListSalesNeedingDHRecord(ctx context.Context, limit int) ([]inventory.SaleNeedingDHRecord, error)
}

// DHSaleWriter persists the outcome of a DH sale-record call: the minted
// idempotency key (compare-and-set, so two workers can never send different
// keys for one sale — design §5a) and, on success, the DH-assigned id.
type DHSaleWriter interface {
	SetSaleIdempotencyKeyIfAbsent(ctx context.Context, saleID, key string) (string, error)
	SetSaleDHSaleID(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error
}

// DHSaleConflictSetter flags a purchase for human review when DH sale
// recording fails non-retryably, or succeeds without delisting the item.
type DHSaleConflictSetter interface {
	SetDHSaleConflict(ctx context.Context, purchaseID, reason string) error
}

// DHSoldReconcilerScheduler periodically fixes purchases that have a linked
// sale but whose dh_status is not 'sold'. This is a safety net for the
// best-effort dh_status update in CreateSale/CreateBulkSales.
//
// It reconciles in three directions. The local pass repairs our own column;
// the DH sweep (WithDHSoldSweep) records a real sale for items DH still shows
// as available even though we sold them locally; the handle-recovery pass
// (WithDHSaleHandleRecovery) completes sales DH already accepted whose handle
// we failed to persist. The sweep and recovery passes are complementary, not
// redundant — see the design note at the top of dh_sold_reconciler.go's
// sweepDH and recoverDHSaleHandles for why neither subsumes the other.
type DHSoldReconcilerScheduler struct {
	StopHandle
	lister  StaleDHStatusLister
	updater DHStatusUpdater
	logger  observability.Logger
	config  DHSoldReconcilerConfig

	// Optional DH-side sweep dependencies; the sweep is skipped unless all are
	// present. Wired together by WithDHSoldSweep.
	client      DHInventoryListClient
	resolver    DHInventoryPurchaseResolver
	salesLister DHSalesByPurchaseLister

	// Optional §5b recovery-pass dependency, wired by WithDHSaleHandleRecovery.
	recoveryLister DHSaleHandleRecoveryLister

	// Shared by both passes: recording, persisting, and flagging are identical
	// whichever pass found the row (see recordSale).
	recorder       inventory.DHSaleRecorder
	writer         DHSaleWriter
	conflictSetter DHSaleConflictSetter
}

// DHSoldReconcilerOption configures optional reconciler behavior.
type DHSoldReconcilerOption func(*DHSoldReconcilerScheduler)

// WithDHSoldSweep enables the DH-side sweep, which finds items DH still offers
// for a purchase we have already sold and records a proper sale for them
// (design §6) instead of the retired status PATCH. Without it the reconciler
// only repairs the local dh_status column and DH-side drift persists.
func WithDHSoldSweep(
	client DHInventoryListClient,
	resolver DHInventoryPurchaseResolver,
	salesLister DHSalesByPurchaseLister,
	recorder inventory.DHSaleRecorder,
	writer DHSaleWriter,
	conflictSetter DHSaleConflictSetter,
) DHSoldReconcilerOption {
	return func(s *DHSoldReconcilerScheduler) {
		s.client = client
		s.resolver = resolver
		s.salesLister = salesLister
		s.recorder = recorder
		s.writer = writer
		s.conflictSetter = conflictSetter
	}
}

// WithDHSaleHandleRecovery enables the design §5b recovery pass, which finds
// sales DH has already accepted whose dh_sale_id we failed to persist and
// completes them. Complementary to WithDHSoldSweep, not a substitute — see the
// package doc above for why neither subsumes the other.
func WithDHSaleHandleRecovery(
	lister DHSaleHandleRecoveryLister,
	recorder inventory.DHSaleRecorder,
	writer DHSaleWriter,
	conflictSetter DHSaleConflictSetter,
) DHSoldReconcilerOption {
	return func(s *DHSoldReconcilerScheduler) {
		s.recoveryLister = lister
		s.recorder = recorder
		s.writer = writer
		s.conflictSetter = conflictSetter
	}
}

// NewDHSoldReconcilerScheduler creates a new sold-status reconciler scheduler.
func NewDHSoldReconcilerScheduler(
	lister StaleDHStatusLister,
	updater DHStatusUpdater,
	logger observability.Logger,
	config DHSoldReconcilerConfig,
	opts ...DHSoldReconcilerOption,
) *DHSoldReconcilerScheduler {
	if config.Interval <= 0 {
		config.Interval = 1 * time.Hour
	}
	s := &DHSoldReconcilerScheduler{
		StopHandle: NewStopHandle(),
		lister:     lister,
		updater:    updater,
		logger:     logger.With(context.Background(), observability.String("component", "dh-sold-reconciler")),
		config:     config,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the reconciler loop.
func (s *DHSoldReconcilerScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "dh sold reconciler scheduler disabled")
		return
	}

	// State the DH sweep's status once at startup. Without this the sweep is
	// silently absent when DH is unwired, and the local pass still logs a
	// healthy "reconciler completed" every cycle — so the double-sale hole
	// would look closed while nothing was actually retiring items on DH.
	if s.sweepEnabled() {
		s.logger.Info(ctx, "dh sold reconciler: DH sweep enabled")
	} else {
		s.logger.Warn(ctx, "dh sold reconciler: DH sweep disabled, sold items will not be retired on DH")
	}
	if s.recoveryEnabled() {
		s.logger.Info(ctx, "dh sold reconciler: DH sale handle recovery pass enabled")
	} else {
		s.logger.Warn(ctx, "dh sold reconciler: DH sale handle recovery pass disabled, a lost dh_sale_id cannot be repaired")
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-sold-reconciler",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.reconcile)
}

// sweepEnabled reports whether the optional DH-side dependencies are wired.
func (s *DHSoldReconcilerScheduler) sweepEnabled() bool {
	return s.client != nil && s.resolver != nil && s.salesLister != nil &&
		s.recorder != nil && s.writer != nil && s.conflictSetter != nil
}

// recoveryEnabled reports whether the §5b dependencies are wired, independent
// of sweepEnabled.
func (s *DHSoldReconcilerScheduler) recoveryEnabled() bool {
	return s.recoveryLister != nil && s.recorder != nil && s.writer != nil && s.conflictSetter != nil
}

func (s *DHSoldReconcilerScheduler) reconcile(ctx context.Context) {
	// The local pass runs first so any purchase it flips to 'sold' is visible
	// to the DH sweep in the same cycle rather than a cycle later.
	//
	// Both passes are independent by design, so LIFO defer order between them
	// has no correctness effect.
	defer s.sweepDH(ctx)
	defer s.recoverDHSaleHandles(ctx)

	ids, err := s.lister.ListStaleDHStatusSoldPurchases(ctx)
	if err != nil {
		s.logger.Warn(ctx, "dh sold reconciler: failed to list stale purchases",
			observability.Err(err))
		return
	}
	if len(ids) == 0 {
		return
	}

	fixed := 0
	for _, id := range ids {
		if err := s.updater.UpdatePurchaseDHStatus(ctx, id, string(inventory.DHStatusSold)); err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to update purchase",
				observability.String("purchaseID", id),
				observability.Err(err))
			continue
		}
		fixed++
	}

	s.logger.Info(ctx, "dh sold reconciler completed",
		observability.Int("fixed", fixed),
		observability.Int("total", len(ids)))
}

// sweepDH finds items DH still offers even though we recorded the sale, and
// records a proper sale for them (design §6) — replacing the status PATCH DH
// rejects. It keys on DH inventory ID rather than cert number (PR #682): a
// cert can match several purchases across re-acquisitions, the inventory id
// cannot.
func (s *DHSoldReconcilerScheduler) sweepDH(ctx context.Context) {
	if !s.sweepEnabled() {
		return
	}

	recorded, failed := 0, 0
	for _, status := range dhSoldSweepStatuses {
		items, err := s.fetchInventoryByStatus(ctx, status)
		if err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to list DH inventory",
				observability.String("status", status), observability.Err(err))
			continue
		}

		byInventoryID, err := s.resolvePurchases(ctx, items)
		if err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to resolve purchases for DH inventory",
				observability.String("status", status), observability.Err(err))
			continue
		}

		soldPurchaseIDs := make([]string, 0, len(items))
		for _, item := range items {
			if p := byInventoryID[item.DHInventoryID]; p != nil && p.DHStatus == inventory.DHStatusSold {
				soldPurchaseIDs = append(soldPurchaseIDs, p.ID)
			}
		}
		if len(soldPurchaseIDs) == 0 {
			continue
		}
		salesByPurchase, err := s.salesLister.GetSalesByPurchaseIDs(ctx, soldPurchaseIDs)
		if err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to load sales for DH inventory",
				observability.String("status", status), observability.Err(err))
			continue
		}

		for _, item := range items {
			p := byInventoryID[item.DHInventoryID]
			if p == nil || p.DHStatus != inventory.DHStatusSold {
				continue
			}
			sale := salesByPurchase[p.ID]
			if sale == nil {
				// A sold purchase with no sale row is a data inconsistency the
				// sweep cannot repair by guessing a price. Skip it.
				continue
			}
			if err := s.recordSale(ctx, p, sale); err != nil {
				failed++
				s.logger.Warn(ctx, "dh sold reconciler: failed to record sale on DH",
					observability.String("purchaseID", p.ID),
					observability.String("cert", item.CertNumber),
					observability.Int("dhInventoryID", item.DHInventoryID),
					observability.Err(err))
				continue
			}
			recorded++
			s.logger.Info(ctx, "dh sold reconciler: recorded sale on DH for item still offered there",
				observability.String("purchaseID", p.ID),
				observability.String("cert", item.CertNumber),
				observability.String("dhStatus", status))
		}
	}

	if recorded > 0 || failed > 0 {
		s.logger.Info(ctx, "dh sold reconciler: DH sweep completed",
			observability.Int("recorded", recorded), observability.Int("failed", failed))
	}
}

// recordSale mints an idempotency key if the sale predates this feature (§5a),
// calls DH, and persists the result. Both passes call this — the
// mint -> call -> persist -> flag ordering (§5b) is identical regardless of
// which pass found the row, so a crash at any point leaves it in a state
// either pass can finish.
func (s *DHSoldReconcilerScheduler) recordSale(ctx context.Context, p *inventory.Purchase, sale *inventory.Sale) error {
	key := sale.DHIdempotencyKey
	if key == "" {
		effective, err := s.writer.SetSaleIdempotencyKeyIfAbsent(ctx, sale.ID, inventory.NewDHIdempotencyKey(uuid.NewString))
		if err != nil {
			return fmt.Errorf("mint idempotency key: %w", err)
		}
		key = effective
	}

	req := inventory.DHSaleRequest{
		DHInventoryID:  p.DHInventoryID,
		IdempotencyKey: key,
		SalePriceCents: sale.SalePriceCents,
		SoldAt:         inventory.DeriveDHSoldAt(sale.SaleDate, p.PurchaseDate, sale.CreatedAt),
	}

	result, err := s.recorder.RecordInventorySale(ctx, req)
	if err != nil {
		// A conflict flag is what stops a permanently-failed sale from being
		// retried forever: ListSalesNeedingDHRecord filters on
		// an empty dh_sale_conflict (§5b). A retryable error is left unflagged —
		// the key is persisted, so the next cycle's identical request IS the
		// retry.
		if !inventory.IsRetryableDHSaleError(err) {
			if cErr := s.conflictSetter.SetDHSaleConflict(ctx, p.ID, err.Error()); cErr != nil {
				s.logger.Warn(ctx, "dh sold reconciler: failed to flag conflict",
					observability.String("purchaseID", p.ID), observability.Err(cErr))
			}
		}
		return err
	}

	if setErr := s.writer.SetSaleDHSaleID(ctx, sale.ID, result.DHSaleID, time.Now()); setErr != nil {
		return fmt.Errorf("persist dh_sale_id: %w", setErr)
	}
	if !result.Delisted {
		if cErr := s.conflictSetter.SetDHSaleConflict(ctx, p.ID, "dh sale recorded but item not delisted"); cErr != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to flag conflict",
				observability.String("purchaseID", p.ID), observability.Err(cErr))
		}
	}
	return nil
}

// recoverDHSaleHandles is the design §5b pass: it finds sales DH has already
// accepted (or attempted) whose dh_sale_id we failed to persist, and completes
// them. Unlike sweepDH it is scoped by local columns, not DH's inventory
// listing, so it keeps finding a row after DH has delisted the item — the
// exact window this pass exists to close.
func (s *DHSoldReconcilerScheduler) recoverDHSaleHandles(ctx context.Context) {
	if !s.recoveryEnabled() {
		return
	}

	rows, err := s.recoveryLister.ListSalesNeedingDHRecord(ctx, dhSaleRecoveryBatchSize)
	if err != nil {
		s.logger.Warn(ctx, "dh sold reconciler: failed to list sales needing dh record", observability.Err(err))
		return
	}

	recovered, failed := 0, 0
	for _, row := range rows {
		sale := row.Sale
		p := &inventory.Purchase{ID: sale.PurchaseID, DHInventoryID: row.DHInventoryID, PurchaseDate: row.PurchaseDate}
		if err := s.recordSale(ctx, p, &sale); err != nil {
			failed++
			s.logger.Warn(ctx, "dh sold reconciler: recovery pass failed to record sale",
				observability.String("purchaseID", p.ID), observability.Err(err))
			continue
		}
		recovered++
	}

	if recovered > 0 || failed > 0 {
		s.logger.Info(ctx, "dh sold reconciler: handle recovery pass completed",
			observability.Int("recovered", recovered), observability.Int("failed", failed))
	}
}

// resolvePurchases batch-loads the purchases behind a page of DH inventory,
// keyed by DH inventory ID. One query per status beats a lookup per item, and
// items DH has but we do not are simply absent from the map.
func (s *DHSoldReconcilerScheduler) resolvePurchases(
	ctx context.Context, items []dh.InventoryListItem,
) (map[int]*inventory.Purchase, error) {
	ids := make([]int, 0, len(items))
	for _, item := range items {
		if item.DHInventoryID != 0 {
			ids = append(ids, item.DHInventoryID)
		}
	}
	if len(ids) == 0 {
		return map[int]*inventory.Purchase{}, nil
	}
	return s.resolver.GetPurchasesByDHInventoryIDs(ctx, ids)
}

// fetchInventoryByStatus pages through DH inventory for a single status. The
// sweep is deliberately unfiltered by UpdatedSince: drift is defined by a local
// sale, not by DH-side activity, so a stale item may not have changed on DH for
// weeks and a checkpointed scan would never revisit it.
func (s *DHSoldReconcilerScheduler) fetchInventoryByStatus(ctx context.Context, status string) ([]dh.InventoryListItem, error) {
	var allItems []dh.InventoryListItem
	page := 1
	for {
		if page > maxPagesPerPoll {
			return nil, fmt.Errorf("fetchInventoryByStatus(%s): exceeded max pages (%d)", status, maxPagesPerPoll)
		}
		resp, err := s.client.ListInventory(ctx, dh.InventoryFilters{
			Status:  status,
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, resp.Items...)
		if len(allItems) >= resp.Meta.TotalCount || len(resp.Items) == 0 {
			break
		}
		page++
	}
	return allItems, nil
}
