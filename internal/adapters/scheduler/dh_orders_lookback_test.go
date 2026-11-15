package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestOrdersLookbackCursor(t *testing.T) {
	const cursor = "2026-04-10T00:00:00Z"
	for _, tc := range []struct {
		name, cursor, sold, want string
		fail                     bool
	}{
		{"older", cursor, "2026-04-08T00:00:00Z", cursor, false},
		{"equal", cursor, cursor, cursor, false},
		{"newer", cursor, "2026-04-11T00:00:00Z", "2026-04-11T00:00:00Z", false},
		{"offset older", cursor, "2026-04-10T01:00:00+02:00", cursor, false},
		{"offset newer", cursor, "2026-04-09T23:30:00-02:00", "2026-04-09T23:30:00-02:00", false},
		{"invalid sold at", cursor, "invalid", cursor, false},
		{"fraction newer", cursor, "2026-04-10T00:00:00.1Z", "2026-04-10T00:00:00.1Z", false},
		{"empty batch", cursor, "", cursor, false},
		{"failed", cursor, "2026-04-11T00:00:00Z", cursor, true},
		{"invalid cursor", "invalid", "", "invalid", false},
		{"first seed", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := newMockSyncStateStore()
			store.values[syncStateKeyDHOrdersPoll] = tc.cursor
			client := &mocks.MockDHOrdersClient{GetOrdersFn: func(_ context.Context, f dh.OrderFilters) (*dh.OrdersResponse, error) {
				if tc.cursor == "" && store.values[syncStateKeyDHOrdersPoll] == f.Since {
					at, err := time.Parse(time.RFC3339, f.Since)
					require.NoError(t, err)
					require.WithinDuration(t, time.Now().Add(-24*time.Hour), at, 2*time.Second)
				} else {
					stored, err := time.Parse(time.RFC3339, store.values[syncStateKeyDHOrdersPoll])
					require.NoError(t, err)
					require.Equal(t, stored.Add(-7*24*time.Hour).UTC().Format(time.RFC3339Nano), f.Since)
				}
				if tc.fail {
					return nil, errors.New("fetch failed")
				}
				resp := &dh.OrdersResponse{}
				if tc.sold != "" && f.Page == 1 {
					resp.Orders = []dh.Order{{SoldAt: tc.sold, Grade: "10", Channel: "dh"}, {SoldAt: cursor, Grade: "10", Channel: "dh"}}
					resp.Meta.TotalCount = 3 // Fetch a second page with the same lookback.
				}
				return resp, nil
			}}
			s := NewDHOrdersPollScheduler(client, store, &mocks.MockImportService{}, nil, mocks.NewMockLogger(), DHOrdersPollConfig{})
			s.poll(ctx)
			if tc.cursor == "" {
				require.NotEmpty(t, store.values[syncStateKeyDHOrdersPoll])
				s.poll(ctx) // Subsequent empty passes replay before the persisted seed.
			} else {
				require.Equal(t, tc.want, store.values[syncStateKeyDHOrdersPoll])
			}
			if tc.cursor == "invalid" {
				require.Zero(t, client.CallCount)
				require.Empty(t, store.values[syncStateKeyDHOrdersClean])
			}
			if tc.fail {
				require.Empty(t, store.values[syncStateKeyDHOrdersClean])
			}
		})
	}
}

func TestOrdersLookbackReplay(t *testing.T) {
	for _, tc := range []struct {
		name, existing string
		removed        bool
	}{
		{"same order", "incoming", false}, {"manual sale", "", false}, {"removed sale", "incoming", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := mocks.NewInMemoryCampaignStore()
			repo.Campaigns["campaign"] = &inventory.Campaign{ID: "campaign"}
			purchase := inventory.Purchase{ID: "purchase", CampaignID: "campaign", CertNumber: "fixture", DHInventoryID: 1, DHStatus: inventory.DHStatusSold}
			repo.Purchases[purchase.ID] = &purchase
			sale := inventory.Sale{ID: "existing", PurchaseID: purchase.ID, OrderID: tc.existing, SalePriceCents: 500}
			require.NoError(t, repo.CreateSale(ctx, &sale))
			beforeSale, beforePurchase := sale, purchase
			if tc.removed {
				require.NoError(t, repo.DeleteSaleByPurchaseID(ctx, purchase.ID))
			}
			svc := csvimport.NewService(csvimport.Deps{Campaigns: repo, Purchases: repo, Sales: repo, Finance: repo, IDGen: func() string { return "new-sale" }})
			defer svc.Close()
			client := &mocks.MockDHOrdersClient{GetOrdersFn: func(_ context.Context, f dh.OrderFilters) (*dh.OrdersResponse, error) {
				require.Equal(t, "2026-04-03T00:00:00Z", f.Since)
				return &dh.OrdersResponse{Orders: []dh.Order{{OrderID: "incoming", CertNumber: "fixture", SoldAt: "2026-04-08T00:00:00Z", Channel: "ebay", Grade: "10", SalePriceCents: 1000}}, Meta: dh.PaginationMeta{TotalCount: 1}}, nil
			}}
			store := newMockSyncStateStore()
			store.values[syncStateKeyDHOrdersPoll] = "2026-04-10T00:00:00Z"
			recorder, logger := &mocks.MockEventRecorder{}, mocks.NewCapturingLogger()
			s := NewDHOrdersPollScheduler(client, store, svc, recorder, logger, DHOrdersPollConfig{})
			for range 2 {
				s.poll(ctx)
			}
			got, err := repo.GetSaleByPurchaseID(ctx, purchase.ID)
			require.NoError(t, err)
			require.Len(t, repo.Sales, 1)
			if tc.removed {
				require.Equal(t, "incoming", got.OrderID)
				require.Equal(t, 1000, got.SalePriceCents)
				require.Len(t, recorder.Events, 1) // A late new sale emits once even below the cursor.
			} else {
				require.Equal(t, beforeSale, *got)
				require.Equal(t, beforePurchase, *repo.Purchases[purchase.ID])
				require.Empty(t, recorder.Events)
			}
			entries := logger.Entries()
			require.Len(t, entries, 2) // One summary per pass, no per-order replay log.
			count, ok := entries[1].FindField("already_sold")
			require.True(t, ok)
			require.Equal(t, 1, count)
		})
	}
}

func TestOrdersLookbackNotifications(t *testing.T) {
	for _, tc := range []struct {
		name, sold       string
		events, warnings int
	}{
		{"older", "2026-04-08T00:00:00Z", 0, 0},
		{"equal", "2026-04-10T00:00:00Z", 0, 0},
		{"newer", "2026-04-11T00:00:00Z", 2, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &mocks.MockDHOrdersClient{GetOrdersFn: func(context.Context, dh.OrderFilters) (*dh.OrdersResponse, error) {
				return &dh.OrdersResponse{Orders: []dh.Order{{OrderID: "orphan", SoldAt: tc.sold, Grade: "invalid", Channel: "unknown"}, {OrderID: "sold", SoldAt: tc.sold, Grade: "invalid", Channel: "unknown"}}, Meta: dh.PaginationMeta{TotalCount: 2}}, nil
			}}
			svc := &mocks.MockImportService{ImportOrdersSalesFn: func(context.Context, []csvimport.OrdersExportRow) (*csvimport.OrdersImportResult, error) {
				return &csvimport.OrdersImportResult{NotFound: []csvimport.OrdersImportSkip{{OrderID: "orphan"}}, AlreadySold: []csvimport.OrdersImportSkip{{OrderID: "sold"}}}, nil
			}}
			store := newMockSyncStateStore()
			store.values[syncStateKeyDHOrdersPoll] = "2026-04-10T00:00:00Z"
			recorder, logger := &mocks.MockEventRecorder{}, mocks.NewCapturingLogger()
			s := NewDHOrdersPollScheduler(client, store, svc, recorder, logger, DHOrdersPollConfig{})
			for range 2 {
				s.poll(context.Background())
			}
			require.Len(t, recorder.Events, tc.events)
			warnings := 0
			for _, entry := range logger.Entries() {
				if entry.Level == "warn" {
					warnings++
				}
			}
			require.Equal(t, tc.warnings, warnings)
		})
	}
}
