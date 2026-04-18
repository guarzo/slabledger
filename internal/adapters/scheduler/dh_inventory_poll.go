package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const syncStateKeyDHInventoryPoll = "dh_inventory_last_poll"

// maxPagesPerPoll prevents unbounded pagination if the DH API misreports totals.
const maxPagesPerPoll = 100

// DHInventoryListClient is the subset of dh.Client used by the inventory poll scheduler.
type DHInventoryListClient interface {
	ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
}

// DHFieldsUpdater updates DH tracking fields on a purchase.
type DHFieldsUpdater interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error
}

// PurchaseByCertLookup resolves a cert number to its local purchase ID and
// current dh_status in a single round trip, so the poll loop can detect
// DH-side transitions (e.g. listed → in_stock).
type PurchaseByCertLookup interface {
	GetDHStatusByCertNumber(ctx context.Context, certNumber string) (purchaseID string, dhStatus string, err error)
}

// DHInventoryPollConfig controls the inventory poll scheduler.
type DHInventoryPollConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHInventoryPollScheduler polls DH for inventory status updates.
type DHInventoryPollScheduler struct {
	StopHandle
	client    DHInventoryListClient
	syncState SyncStateStore
	updater   DHFieldsUpdater
	lookup    PurchaseByCertLookup
	eventRec  dhevents.Recorder // may be nil
	logger    observability.Logger
	config    DHInventoryPollConfig
}

// NewDHInventoryPollScheduler creates a new inventory poll scheduler.
func NewDHInventoryPollScheduler(
	client DHInventoryListClient,
	syncState SyncStateStore,
	updater DHFieldsUpdater,
	lookup PurchaseByCertLookup,
	eventRec dhevents.Recorder, // may be nil for tests / unwired
	logger observability.Logger,
	config DHInventoryPollConfig,
) *DHInventoryPollScheduler {
	if config.Interval <= 0 {
		config.Interval = 2 * time.Hour
	}
	return &DHInventoryPollScheduler{
		StopHandle: NewStopHandle(),
		client:     client,
		syncState:  syncState,
		updater:    updater,
		lookup:     lookup,
		eventRec:   eventRec,
		logger:     logger.With(context.Background(), observability.String("component", "dh-inventory-poll")),
		config:     config,
	}
}

// Start begins the inventory poll loop.
func (s *DHInventoryPollScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "dh inventory poll scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-inventory-poll",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.poll)
}

// recordEvent emits an event to the recorder if present. Failures are logged but do not abort.
func (s *DHInventoryPollScheduler) recordEvent(ctx context.Context, e dhevents.Event) {
	if s.eventRec == nil {
		return
	}
	if err := s.eventRec.Record(ctx, e); err != nil {
		s.logger.Warn(ctx, "dh inventory poll: record event failed",
			observability.String("type", string(e.Type)),
			observability.Err(err))
	}
}

// poll fetches inventory status from DH and writes updates back to local purchase records.
func (s *DHInventoryPollScheduler) poll(ctx context.Context) {
	since, err := s.syncState.Get(ctx, syncStateKeyDHInventoryPoll)
	if err != nil {
		s.logger.Warn(ctx, "failed to read dh inventory sync state, defaulting to no filter",
			observability.Err(err))
	}

	allItems, err := s.fetchAllPages(ctx, since)
	if err != nil {
		s.logger.Warn(ctx, "dh inventory poll failed",
			observability.String("since", since),
			observability.Err(err))
		return
	}

	if len(allItems) == 0 {
		s.logger.Debug(ctx, "dh inventory poll: no items",
			observability.String("since", since))
		return
	}

	updated := 0
	skipped := 0
	var latestUpdatedAt string

	for _, item := range allItems {
		// Always advance the checkpoint so persistent failures don't block progress.
		if item.UpdatedAt > latestUpdatedAt {
			latestUpdatedAt = item.UpdatedAt
		}

		purchaseID, prevDHStatus, lookupErr := s.lookup.GetDHStatusByCertNumber(ctx, item.CertNumber)
		if lookupErr != nil {
			s.logger.Warn(ctx, "dh inventory poll: cert lookup error",
				observability.String("cert", item.CertNumber),
				observability.Err(lookupErr))
			skipped++
			continue
		}
		if purchaseID == "" {
			s.logger.Debug(ctx, "dh inventory poll: cert not found in local system",
				observability.String("cert", item.CertNumber))
			skipped++
			continue
		}

		// Sold items are owned by the orders poll, which also creates the
		// campaign_sales row. Writing dh_status='sold' here without a sale
		// makes the UI show the cert as on-hand.
		if item.Status == dh.InventoryStatusSold {
			s.logger.Debug(ctx, "dh inventory poll: skipping sold item, orders poll will record sale",
				observability.String("purchaseID", purchaseID),
				observability.String("cert", item.CertNumber))
			skipped++
			continue
		}

		if updateErr := s.updater.UpdatePurchaseDHFields(ctx, purchaseID, inventory.DHFieldsUpdate{
			CardID:            item.DHCardID,
			InventoryID:       item.DHInventoryID,
			CertStatus:        dh.CertStatusMatched,
			ListingPriceCents: item.ListingPriceCents,
			ChannelsJSON:      dh.MarshalChannels(item.Channels),
			DHStatus:          item.Status,
			LastSyncedAt:      time.Now().UTC().Format(time.RFC3339),
		}); updateErr != nil {
			s.logger.Warn(ctx, "dh inventory poll: failed to update purchase",
				observability.String("purchaseID", purchaseID),
				observability.String("cert", item.CertNumber),
				observability.Err(updateErr))
			skipped++
			continue
		}

		updated++

		// Emit an observation event. A listed → in_stock drop is the only
		// "unlisted" shape DH's inventory API exposes (it reports in_stock /
		// listed / sold only), so classify as TypeUnlisted rather than the
		// generic TypePushed to avoid double-counting in downstream aggregations.
		var eventType dhevents.Type
		switch item.Status {
		case dh.InventoryStatusInStock:
			if prevDHStatus == inventory.DHStatusListed {
				eventType = dhevents.TypeUnlisted
			} else {
				eventType = dhevents.TypePushed
			}
		case dh.InventoryStatusListed:
			eventType = dhevents.TypeListed
		}
		if eventType != "" {
			evt := dhevents.Event{
				PurchaseID:    purchaseID,
				CertNumber:    item.CertNumber,
				Type:          eventType,
				NewDHStatus:   item.Status,
				DHInventoryID: item.DHInventoryID,
				DHCardID:      item.DHCardID,
				Source:        dhevents.SourceDHInventoryPoll,
			}
			if eventType == dhevents.TypeUnlisted {
				evt.PrevDHStatus = prevDHStatus
			}
			s.recordEvent(ctx, evt)
		}
	}

	s.logger.Info(ctx, "dh inventory poll completed",
		observability.Int("updated", updated),
		observability.Int("skipped", skipped))

	if latestUpdatedAt != "" {
		if setErr := s.syncState.Set(ctx, syncStateKeyDHInventoryPoll, latestUpdatedAt); setErr != nil {
			s.logger.Warn(ctx, "failed to update dh inventory sync state",
				observability.String("timestamp", latestUpdatedAt),
				observability.Err(setErr))
		}
	}
}

// fetchAllPages retrieves all inventory pages from DH.
func (s *DHInventoryPollScheduler) fetchAllPages(ctx context.Context, since string) ([]dh.InventoryListItem, error) {
	var allItems []dh.InventoryListItem
	page := 1
	for {
		if page > maxPagesPerPoll {
			return nil, fmt.Errorf("fetchAllPages: exceeded max pages (%d), possible API total miscount", maxPagesPerPoll)
		}
		resp, err := s.client.ListInventory(ctx, dh.InventoryFilters{
			UpdatedSince: since,
			Page:         page,
			PerPage:      100,
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

// Compile-time check that dh.Client satisfies DHInventoryListClient.
var _ DHInventoryListClient = (*dh.Client)(nil)
