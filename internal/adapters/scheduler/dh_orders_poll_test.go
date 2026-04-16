package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDHOrdersPoll_NoOrders(t *testing.T) {
	client := &mocks.MockDHOrdersClient{
		GetOrdersFn: func(_ context.Context, _ dh.OrderFilters) (*dh.OrdersResponse, error) {
			return &dh.OrdersResponse{Orders: nil, Meta: dh.PaginationMeta{Page: 1, PerPage: 100, TotalCount: 0}}, nil
		},
	}
	syncStore := newMockSyncStateStore()
	svc := &mocks.MockInventoryService{}

	s := NewDHOrdersPollScheduler(client, syncStore, svc, mocks.NewMockLogger(), DHOrdersPollConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	})

	s.poll(context.Background())

	assert.Equal(t, 1, client.CallCount, "client should be called once")
}

func TestDHOrdersPoll_RecordsSale(t *testing.T) {
	client := &mocks.MockDHOrdersClient{
		GetOrdersFn: func(_ context.Context, _ dh.OrderFilters) (*dh.OrdersResponse, error) {
			return &dh.OrdersResponse{
				Orders: []dh.Order{
					{
						OrderID:        "dh-12345",
						CertNumber:     "99998888",
						CardName:       "Charizard PSA 10",
						SalePriceCents: 7500,
						Channel:        "ebay",
						SoldAt:         "2026-04-02T14:30:00Z",
						Grade:          10,
						Fees: dh.OrderFees{
							ChannelFeeCents: intPtr(994),
						},
					},
				},
				Meta: dh.PaginationMeta{Page: 1, PerPage: 100, TotalCount: 1},
			}, nil
		},
	}
	syncStore := newMockSyncStateStore()

	var capturedConfirmItems []inventory.OrdersConfirmItem

	svc := &mocks.MockInventoryService{
		ImportOrdersSalesFn: func(_ context.Context, rows []inventory.OrdersExportRow) (*inventory.OrdersImportResult, error) {
			require.Len(t, rows, 1)
			assert.Equal(t, "99998888", rows[0].CertNumber)
			assert.Equal(t, "2026-04-02", rows[0].Date)
			assert.Equal(t, inventory.SaleChannelEbay, rows[0].SalesChannel)
			assert.Equal(t, 75.0, rows[0].UnitPrice)
			assert.Equal(t, "PSA", rows[0].Grader)
			assert.Equal(t, float64(10), rows[0].Grade)

			return &inventory.OrdersImportResult{
				Matched: []inventory.OrdersImportMatch{
					{
						CertNumber:     "99998888",
						ProductTitle:   "Charizard PSA 10",
						SaleChannel:    inventory.SaleChannelEbay,
						SaleDate:       "2026-04-02",
						SalePriceCents: 7500,
						SaleFeeCents:   994,
						PurchaseID:     "pur-001",
						CampaignID:     "camp-001",
						CardName:       "Charizard",
						BuyCostCents:   5000,
						NetProfitCents: 1506,
					},
				},
			}, nil
		},
		ConfirmOrdersSalesFn: func(_ context.Context, items []inventory.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
			capturedConfirmItems = items
			return &inventory.BulkSaleResult{Created: len(items)}, nil
		},
	}

	s := NewDHOrdersPollScheduler(client, syncStore, svc, mocks.NewMockLogger(), DHOrdersPollConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	})

	s.poll(context.Background())

	require.Len(t, capturedConfirmItems, 1)
	assert.Equal(t, "pur-001", capturedConfirmItems[0].PurchaseID)
	assert.Equal(t, inventory.SaleChannelEbay, capturedConfirmItems[0].SaleChannel)
	assert.Equal(t, "2026-04-02", capturedConfirmItems[0].SaleDate)
	assert.Equal(t, 7500, capturedConfirmItems[0].SalePriceCents)
	assert.Equal(t, "dh-12345", capturedConfirmItems[0].OrderID, "OrderID should be set from DH order")

	// Verify checkpoint was updated
	assert.Equal(t, "2026-04-02T14:30:00Z", syncStore.values[syncStateKeyDHOrdersPoll])
}

func TestDHOrdersPoll_Disabled(t *testing.T) {
	s := NewDHOrdersPollScheduler(
		&mocks.MockDHOrdersClient{},
		newMockSyncStateStore(),
		&mocks.MockInventoryService{},
		mocks.NewMockLogger(),
		DHOrdersPollConfig{Enabled: false},
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

func TestMapDHChannel(t *testing.T) {
	logger := mocks.NewMockLogger()
	ctx := context.Background()
	cases := []struct {
		name string
		in   string
		want inventory.SaleChannel
	}{
		{"dh native", "dh", inventory.SaleChannelDoubleHolo},
		{"ebay", "ebay", inventory.SaleChannelEbay},
		{"shopify", "shopify", inventory.SaleChannelWebsite},
		{"unknown", "flea_market", inventory.SaleChannelOther},
		{"empty", "", inventory.SaleChannelOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapDHChannel(ctx, tc.in, logger)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDHOrdersPoll_RunOnce_HonorsCallerSince(t *testing.T) {
	var capturedSince string
	client := &mocks.MockDHOrdersClient{
		GetOrdersFn: func(_ context.Context, f dh.OrderFilters) (*dh.OrdersResponse, error) {
			capturedSince = f.Since
			return &dh.OrdersResponse{Meta: dh.PaginationMeta{TotalCount: 0}}, nil
		},
	}
	syncStore := newMockSyncStateStore()
	// Pre-seed a different value in sync state to prove RunOnce ignores it.
	require.NoError(t, syncStore.Set(context.Background(), syncStateKeyDHOrdersPoll, "2020-01-01T00:00:00Z"))

	s := NewDHOrdersPollScheduler(client, syncStore, &mocks.MockInventoryService{},
		mocks.NewMockLogger(), DHOrdersPollConfig{Enabled: true, Interval: 1 * time.Hour})

	summary, err := s.RunOnce(context.Background(), "2025-06-01T00:00:00Z")
	require.NoError(t, err)
	assert.Equal(t, "2025-06-01T00:00:00Z", capturedSince,
		"RunOnce should pass through the caller-supplied since, not read from sync state")
	assert.Equal(t, "2025-06-01T00:00:00Z", summary.Since)
	assert.Equal(t, 0, summary.OrdersFetched)

	// RunOnce must NOT advance the checkpoint — it's a one-off manual call.
	stored, _ := syncStore.Get(context.Background(), syncStateKeyDHOrdersPoll)
	assert.Equal(t, "2020-01-01T00:00:00Z", stored,
		"RunOnce should not touch the sync-state checkpoint")
}

func intPtr(v int) *int { return &v }

// Compile-time interface check.
var _ DHOrdersClient = (*mocks.MockDHOrdersClient)(nil)
