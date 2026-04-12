package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
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

// PurchaseByCertLookup resolves a cert number to a purchase ID.
type PurchaseByCertLookup interface {
	GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error)
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
	logger    observability.Logger
	config    DHInventoryPollConfig
}

// NewDHInventoryPollScheduler creates a new inventory poll scheduler.
func NewDHInventoryPollScheduler(
	client DHInventoryListClient,
	syncState SyncStateStore,
	updater DHFieldsUpdater,
	lookup PurchaseByCertLookup,
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

		purchaseID, lookupErr := s.lookup.GetPurchaseIDByCertNumber(ctx, item.CertNumber)
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
