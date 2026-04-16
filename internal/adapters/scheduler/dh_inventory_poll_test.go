package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
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
						Status:            dh.InventoryStatusListed,
						ListingPriceCents: 7500,
						Channels: []dh.InventoryChannelStatus{
							{Name: "ebay", Status: dh.InventoryStatusListed},
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
		client, syncStore, updater, lookup, nil,
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
	require.Equal(t, dh.InventoryStatusListed, call.DHStatus)
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
						Status:            dh.InventoryStatusListed,
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
		client, syncStore, updater, lookup, nil,
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
		nil,
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

func TestDHInventoryPoll_RecordsEvents(t *testing.T) {
	client := &mocks.MockDHInventoryListClient{
		ListInventoryFn: func(_ context.Context, _ dh.InventoryFilters) (*dh.InventoryListResponse, error) {
			return &dh.InventoryListResponse{
				Items: []dh.InventoryListItem{
					{DHInventoryID: 1001, DHCardID: 101, CertNumber: "c-in-stock", Status: "in_stock", UpdatedAt: "2026-04-03T10:00:00Z"},
					{DHInventoryID: 1002, DHCardID: 102, CertNumber: "c-listed", Status: "listed", UpdatedAt: "2026-04-03T10:01:00Z"},
					{DHInventoryID: 1003, DHCardID: 103, CertNumber: "c-unknown", Status: "unknown", UpdatedAt: "2026-04-03T10:02:00Z"},
				},
				Meta: dh.PaginationMeta{Page: 1, PerPage: 100, TotalCount: 3},
			}, nil
		},
	}

	syncStore := newMockSyncStateStore()
	updater := &mocks.MockDHFieldsUpdater{}
	lookup := &mocks.MockPurchaseByCertLookup{
		Mapping: map[string]string{
			"c-in-stock": "pur-in-stock",
			"c-listed":   "pur-listed",
			"c-unknown":  "pur-unknown",
		},
	}
	recorder := &mocks.MockEventRecorder{}

	s := NewDHInventoryPollScheduler(
		client, syncStore, updater, lookup, recorder,
		mocks.NewMockLogger(),
		DHInventoryPollConfig{Enabled: true, Interval: 1 * time.Hour},
	)

	s.poll(context.Background())

	// Count events by type
	byType := make(map[dhevents.Type]int)
	for _, e := range recorder.Events {
		byType[e.Type]++
	}
	assert.Equal(t, 1, byType[dhevents.TypePushed], "one pushed event for in_stock status")
	assert.Equal(t, 1, byType[dhevents.TypeListed], "one listed event for listed status")
	assert.Equal(t, 0, byType[dhevents.Type("")], "no events for unknown status")
	assert.Equal(t, 2, len(recorder.Events), "exactly two events total")

	// Verify the pushed event has the expected fields
	var pushedEvent dhevents.Event
	for _, e := range recorder.Events {
		if e.Type == dhevents.TypePushed {
			pushedEvent = e
			break
		}
	}
	assert.Equal(t, "pur-in-stock", pushedEvent.PurchaseID)
	assert.Equal(t, "c-in-stock", pushedEvent.CertNumber)
	assert.Equal(t, "in_stock", pushedEvent.NewDHStatus)
	assert.Equal(t, 1001, pushedEvent.DHInventoryID)
	assert.Equal(t, 101, pushedEvent.DHCardID)
	assert.Equal(t, dhevents.SourceDHInventoryPoll, pushedEvent.Source)

	// Verify the listed event has the expected fields
	var listedEvent dhevents.Event
	for _, e := range recorder.Events {
		if e.Type == dhevents.TypeListed {
			listedEvent = e
			break
		}
	}
	assert.Equal(t, "pur-listed", listedEvent.PurchaseID)
	assert.Equal(t, "c-listed", listedEvent.CertNumber)
	assert.Equal(t, "listed", listedEvent.NewDHStatus)
	assert.Equal(t, 1002, listedEvent.DHInventoryID)
	assert.Equal(t, 102, listedEvent.DHCardID)
	assert.Equal(t, dhevents.SourceDHInventoryPoll, listedEvent.Source)
}

// Verify UpdatePurchaseDHFields is called via the DHFieldsUpdater interface.
var _ DHFieldsUpdater = (*mocks.MockDHFieldsUpdater)(nil)
var _ PurchaseByCertLookup = (*mocks.MockPurchaseByCertLookup)(nil)
var _ DHInventoryListClient = (*mocks.MockDHInventoryListClient)(nil)
