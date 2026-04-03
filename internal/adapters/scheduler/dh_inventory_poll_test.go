package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// mockDHInventoryClient implements DHInventoryListClient for testing.
type mockDHInventoryClient struct {
	resp *dh.InventoryListResponse
	err  error
}

func (m *mockDHInventoryClient) ListInventory(_ context.Context, _ dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	return m.resp, m.err
}

// mockDHFieldsUpdater implements DHFieldsUpdater and captures calls for verification.
type mockDHFieldsUpdater struct {
	calls []dhFieldsUpdate
	err   error
}

type dhFieldsUpdate struct {
	id                string
	cardID            int
	inventoryID       int
	certStatus        string
	listingPriceCents int
	channelsJSON      string
}

func (m *mockDHFieldsUpdater) UpdatePurchaseDHFields(_ context.Context, id string, cardID, inventoryID int, certStatus string, listingPriceCents int, channelsJSON string) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, dhFieldsUpdate{
		id:                id,
		cardID:            cardID,
		inventoryID:       inventoryID,
		certStatus:        certStatus,
		listingPriceCents: listingPriceCents,
		channelsJSON:      channelsJSON,
	})
	return nil
}

// mockPurchaseByCertLookup implements PurchaseByCertLookup for testing.
type mockPurchaseByCertLookup struct {
	mapping map[string]string // certNumber -> purchaseID
	err     error
}

func (m *mockPurchaseByCertLookup) GetPurchaseIDByCertNumber(_ context.Context, certNumber string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.mapping[certNumber], nil
}

func TestDHInventoryPoll_UpdatesPurchase(t *testing.T) {
	client := &mockDHInventoryClient{
		resp: &dh.InventoryListResponse{
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
		},
	}

	syncStore := newMockSyncStateStore()
	updater := &mockDHFieldsUpdater{}
	lookup := &mockPurchaseByCertLookup{
		mapping: map[string]string{"12345678": "purchase-1"},
	}

	s := NewDHInventoryPollScheduler(
		client, syncStore, updater, lookup,
		mocks.NewMockLogger(),
		DHInventoryPollConfig{Enabled: true, Interval: 1 * time.Hour},
	)

	s.poll(context.Background())

	require.Len(t, updater.calls, 1)
	call := updater.calls[0]
	require.Equal(t, "purchase-1", call.id)
	require.Equal(t, 111, call.cardID)
	require.Equal(t, 98765, call.inventoryID)
	require.Equal(t, "matched", call.certStatus)
	require.Equal(t, 7500, call.listingPriceCents)
	require.Contains(t, call.channelsJSON, "ebay")

	// Verify checkpoint was updated
	require.Equal(t, "2026-04-03T10:00:00Z", syncStore.values[syncStateKeyDHInventoryPoll])
}

func TestDHInventoryPoll_SkipUnknownCert(t *testing.T) {
	client := &mockDHInventoryClient{
		resp: &dh.InventoryListResponse{
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
		},
	}

	syncStore := newMockSyncStateStore()
	updater := &mockDHFieldsUpdater{}
	lookup := &mockPurchaseByCertLookup{
		mapping: map[string]string{}, // cert not in our system
	}

	s := NewDHInventoryPollScheduler(
		client, syncStore, updater, lookup,
		mocks.NewMockLogger(),
		DHInventoryPollConfig{Enabled: true, Interval: 1 * time.Hour},
	)

	s.poll(context.Background())

	require.Empty(t, updater.calls, "updater should not be called for unknown cert")
}

func TestDHInventoryPoll_Disabled(t *testing.T) {
	s := NewDHInventoryPollScheduler(
		&mockDHInventoryClient{},
		newMockSyncStateStore(),
		&mockDHFieldsUpdater{},
		&mockPurchaseByCertLookup{},
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
