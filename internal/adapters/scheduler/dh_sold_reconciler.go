package scheduler

import (
	"context"
	"fmt"
	"time"

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

// PurchaseGetter loads a single purchase, so the sweep can confirm the sold
// purchase it matched actually owns the DH inventory item it is about to retire.
type PurchaseGetter interface {
	GetPurchase(ctx context.Context, id string) (*inventory.Purchase, error)
}

// dhSoldSweepStatuses are the DH-side statuses that mean "DH still believes
// this item is ours to sell". Anything sold locally but sitting in one of
// these on DH is drift, and `listed` in particular is live double-sale risk.
var dhSoldSweepStatuses = []string{dh.InventoryStatusListed, dh.InventoryStatusInStock}

// DHSoldReconcilerScheduler periodically fixes purchases that have a linked
// sale but whose dh_status is not 'sold'. This is a safety net for the
// best-effort dh_status update in CreateSale/CreateBulkSales.
//
// It reconciles in both directions. The local pass repairs our own column; the
// DH pass (enabled by WithDHSoldSweep) retires items DH still shows as
// available even though we recorded the sale. Only the second pass closes the
// double-sale hole: local dh_status is bookkeeping DH never sees.
type DHSoldReconcilerScheduler struct {
	StopHandle
	lister  StaleDHStatusLister
	updater DHStatusUpdater
	logger  observability.Logger
	config  DHSoldReconcilerConfig

	// Optional DH-side sweep dependencies; all three are set together by
	// WithDHSoldSweep, and the sweep is skipped unless all are present.
	client   DHInventoryListClient
	lookup   PurchaseByCertLookup
	getter   PurchaseGetter
	notifier inventory.DHSoldNotifier
}

// DHSoldReconcilerOption configures optional reconciler behavior.
type DHSoldReconcilerOption func(*DHSoldReconcilerScheduler)

// WithDHSoldSweep enables the DH-side pass, which finds items DH still offers
// for a purchase we have already sold and retires them on DH. Without it the
// reconciler only repairs the local column and DH drift persists indefinitely.
func WithDHSoldSweep(
	client DHInventoryListClient,
	lookup PurchaseByCertLookup,
	getter PurchaseGetter,
	notifier inventory.DHSoldNotifier,
) DHSoldReconcilerOption {
	return func(s *DHSoldReconcilerScheduler) {
		s.client = client
		s.lookup = lookup
		s.getter = getter
		s.notifier = notifier
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
	return s.client != nil && s.lookup != nil && s.getter != nil && s.notifier != nil
}

func (s *DHSoldReconcilerScheduler) reconcile(ctx context.Context) {
	// The local pass runs first so any purchase it flips to 'sold' is visible
	// to the DH sweep in the same cycle rather than a cycle later.
	defer s.sweepDH(ctx)

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

// sweepDH retires items that DH still offers even though we recorded the sale.
//
// It trusts local dh_status='sold', which is only ever written alongside a
// campaign_sales row (the sale paths and the local pass above), and retires an
// item only after confirming that sold purchase owns the DH inventory ID — so a
// cert with several purchases (sold, then re-acquired and re-listed) cannot lose
// its live listing to the stale row.
func (s *DHSoldReconcilerScheduler) sweepDH(ctx context.Context) {
	if !s.sweepEnabled() {
		return
	}

	retired, failed := 0, 0
	for _, status := range dhSoldSweepStatuses {
		items, err := s.fetchInventoryByStatus(ctx, status)
		if err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to list DH inventory",
				observability.String("status", status),
				observability.Err(err))
			continue
		}
		for _, item := range items {
			if item.DHInventoryID == 0 {
				continue
			}
			purchaseID, localStatus, lookupErr := s.lookup.GetDHStatusByCertNumber(ctx, item.CertNumber)
			if lookupErr != nil {
				s.logger.Warn(ctx, "dh sold reconciler: cert lookup error",
					observability.String("cert", item.CertNumber),
					observability.Err(lookupErr))
				continue
			}
			// Unknown cert, or one we also believe is still available: not drift.
			if purchaseID == "" || localStatus != string(inventory.DHStatusSold) {
				continue
			}
			// Confirm the sold purchase actually owns this DH item before
			// retiring it. GetDHStatusByCertNumber returns an arbitrary row when
			// a cert has several purchases, so without this a card sold and
			// later re-acquired could have its new listing pulled.
			p, getErr := s.getter.GetPurchase(ctx, purchaseID)
			if getErr != nil {
				s.logger.Warn(ctx, "dh sold reconciler: purchase load error",
					observability.String("purchaseID", purchaseID),
					observability.Err(getErr))
				continue
			}
			if p == nil || p.DHInventoryID != item.DHInventoryID {
				s.logger.Debug(ctx, "dh sold reconciler: sold purchase does not own this DH item, skipping",
					observability.String("purchaseID", purchaseID),
					observability.String("cert", item.CertNumber),
					observability.Int("dhInventoryID", item.DHInventoryID))
				continue
			}
			if err := s.notifier.MarkInventorySold(ctx, item.DHInventoryID); err != nil {
				failed++
				s.logger.Warn(ctx, "dh sold reconciler: failed to retire sold item on DH",
					observability.String("purchaseID", purchaseID),
					observability.String("cert", item.CertNumber),
					observability.Int("dhInventoryID", item.DHInventoryID),
					observability.Err(err))
				continue
			}
			retired++
			s.logger.Info(ctx, "dh sold reconciler: retired sold item still offered on DH",
				observability.String("purchaseID", purchaseID),
				observability.String("cert", item.CertNumber),
				observability.String("dhStatus", status))
		}
	}

	if retired > 0 || failed > 0 {
		s.logger.Info(ctx, "dh sold reconciler: DH sweep completed",
			observability.Int("retired", retired),
			observability.Int("failed", failed))
	}
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
