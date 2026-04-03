package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const syncStateKeyDHInventoryPoll = "dh_inventory_last_poll"

// DHInventoryListClient is the subset of dh.Client used by the inventory poll scheduler.
type DHInventoryListClient interface {
	ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
}

// DHFieldsUpdater updates DH tracking fields on a purchase.
type DHFieldsUpdater interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, cardID, inventoryID int, certStatus string, listingPriceCents int, channelsJSON string) error
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
	// 1. Read checkpoint from sync_state
	since, err := s.syncState.Get(ctx, syncStateKeyDHInventoryPoll)
	if err != nil {
		s.logger.Warn(ctx, "failed to read dh inventory sync state, defaulting to no filter",
			observability.Err(err))
	}

	// 2. Fetch inventory from DH
	resp, err := s.client.ListInventory(ctx, dh.InventoryFilters{
		Status:       "active",
		UpdatedSince: since,
		PerPage:      100,
	})
	if err != nil {
		s.logger.Warn(ctx, "dh inventory poll failed",
			observability.String("since", since),
			observability.Err(err))
		return
	}

	// 3. No items — nothing to do
	if len(resp.Items) == 0 {
		s.logger.Debug(ctx, "dh inventory poll: no items",
			observability.String("since", since))
		return
	}

	// 4. Process each item
	updated := 0
	skipped := 0
	var latestUpdatedAt string

	for _, item := range resp.Items {
		// a. Look up purchase ID by cert number
		purchaseID, lookupErr := s.lookup.GetPurchaseIDByCertNumber(ctx, item.CertNumber)
		if lookupErr != nil {
			s.logger.Warn(ctx, "dh inventory poll: cert lookup error",
				observability.String("cert", item.CertNumber),
				observability.Err(lookupErr))
			skipped++
			continue
		}
		if purchaseID == "" {
			skipped++
			continue
		}

		// c. Marshal channels to JSON string
		channelsJSON := "[]"
		if len(item.Channels) > 0 {
			b, marshalErr := json.Marshal(item.Channels)
			if marshalErr != nil {
				s.logger.Warn(ctx, "dh inventory poll: failed to marshal channels",
					observability.String("cert", item.CertNumber),
					observability.Err(marshalErr))
				channelsJSON = "[]"
			} else {
				channelsJSON = string(b)
			}
		}

		// d. Update purchase DH fields
		if updateErr := s.updater.UpdatePurchaseDHFields(
			ctx,
			purchaseID,
			item.DHCardID,
			item.DHInventoryID,
			"matched",
			item.ListingPriceCents,
			channelsJSON,
		); updateErr != nil {
			s.logger.Warn(ctx, "dh inventory poll: failed to update purchase",
				observability.String("purchaseID", purchaseID),
				observability.String("cert", item.CertNumber),
				observability.Err(updateErr))
			skipped++
			continue
		}

		updated++

		// e. Track the latest UpdatedAt
		if item.UpdatedAt > latestUpdatedAt {
			latestUpdatedAt = item.UpdatedAt
		}
	}

	// 5. Log counts
	s.logger.Info(ctx, "dh inventory poll completed",
		observability.Int("updated", updated),
		observability.Int("skipped", skipped))

	// 6. Update checkpoint to latest UpdatedAt
	if latestUpdatedAt != "" {
		if setErr := s.syncState.Set(ctx, syncStateKeyDHInventoryPoll, latestUpdatedAt); setErr != nil {
			s.logger.Warn(ctx, "failed to update dh inventory sync state",
				observability.String("timestamp", latestUpdatedAt),
				observability.Err(setErr))
		}
	}
}

// Compile-time check that dh.Client satisfies DHInventoryListClient.
var _ DHInventoryListClient = (*dh.Client)(nil)
