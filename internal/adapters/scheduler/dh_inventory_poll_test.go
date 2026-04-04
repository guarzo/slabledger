package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDHInventoryPoll_UpdatesPurchase(t *testing.T) {
	client := &mocks.MockDHInventoryListClient{
		ListInventoryFn: func(_ context.Context, _ dh.InventoryFilters) (*dh.InventoryListResponse, error) {
			return &dh.InventoryListResponse{
				Items: []dh.InventoryListItem{
					{
						DHInventoryID:     98765,
						DHCardID:          111,
						CertNumber:        "12345678",
						Status:            "active",
						ListingPriceCents: 7500,
						Channels: []dh.InventoryChannelStatus{
							{Name: "ebay", Status: "active"},
						},
						UpdatedAt: "2026-04-03T10:00:00Z",
					},
				},
				Meta: dh.PaginationMeta{Page: 1, PerPage: 100, TotalCount: 1},
			}, nil
		},
	}

	syncStore := newMockSyncStateStore()
	updater := &mocks.MockDHFieldsUpdater{}
	lookup := &mocks.MockPurchaseByCertLookup{
		Mapping: map[string]string{"12345678": "purchase-1"},
	}

	s := NewDHInventoryPollScheduler(
		client, syncStore, updater, lookup,
		mocks.NewMockLogger(),
		DHInventoryPollConfig{Enabled: true, Interval: 1 * time.Hour},
	)

	s.poll(context.Background())

	require.Len(t, updater.Calls, 1)
	require.Equal(t, "purchase-1", updater.IDs[0])
	call := updater.Calls[0]
	require.Equal(t, 111, call.CardID)
	require.Equal(t, 98765, call.InventoryID)
	require.Equal(t, dh.CertStatusMatched, call.CertStatus)
	require.Equal(t, 7500, call.ListingPriceCents)
	require.Contains(t, call.ChannelsJSON, "ebay")

	// Verify checkpoint was updated
	require.Equal(t, "2026-04-03T10:00:00Z", syncStore.values[syncStateKeyDHInventoryPoll])
}

func TestDHInventoryPoll_SkipUnknownCert(t *testing.T) {
	client := &mocks.MockDHInventoryListClient{
		ListInventoryFn: func(_ context.Context, _ dh.InventoryFilters) (*dh.InventoryListResponse, error) {
			return &dh.InventoryListResponse{
				Items: []dh.InventoryListItem{
					{
						DHInventoryID:     98765,
						DHCardID:          111,
						CertNumber:        "99999999",
						Status:            "active",
						ListingPriceCents: 5000,
						UpdatedAt:         "2026-04-03T10:00:00Z",
					},
				},
				Meta: dh.PaginationMeta{Page: 1, PerPage: 100, TotalCount: 1},
			}, nil
		},
	}

	syncStore := newMockSyncStateStore()
	updater := &mocks.MockDHFieldsUpdater{}
	lookup := &mocks.MockPurchaseByCertLookup{
		Mapping: map[string]string{}, // cert not in our system
	}

	s := NewDHInventoryPollScheduler(
		client, syncStore, updater, lookup,
		mocks.NewMockLogger(),
		DHInventoryPollConfig{Enabled: true, Interval: 1 * time.Hour},
	)

	s.poll(context.Background())

	require.Empty(t, updater.Calls, "updater should not be called for unknown cert")
}

func TestDHInventoryPoll_Disabled(t *testing.T) {
	s := NewDHInventoryPollScheduler(
		&mocks.MockDHInventoryListClient{},
		newMockSyncStateStore(),
		&mocks.MockDHFieldsUpdater{},
		&mocks.MockPurchaseByCertLookup{},
		mocks.NewMockLogger(),
		DHInventoryPollConfig{Enabled: false},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler should return immediately when disabled")
	}
}

// Verify UpdatePurchaseDHFields is called via the DHFieldsUpdater interface.
var _ DHFieldsUpdater = (*mocks.MockDHFieldsUpdater)(nil)
var _ PurchaseByCertLookup = (*mocks.MockPurchaseByCertLookup)(nil)
var _ DHInventoryListClient = (*mocks.MockDHInventoryListClient)(nil)
