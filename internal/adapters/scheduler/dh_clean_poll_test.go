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

func TestDHCleanPollCursorReplay(t *testing.T) {
	for _, feed := range []string{"orders", "inventory"} {
		for _, mode := range []string{"empty", "clean", "skip", "apply", "lookup", "read", "write", "page", "cancel", "seed", "seed-write"} {
			t.Run(feed+"/"+mode, func(t *testing.T) {
				const old = "2026-04-01T00:00:00Z"
				const next = "2026-04-02T00:00:00Z"
				key, marker := "dh_"+feed+"_last_poll", "feed_dh_"+feed+"_clean"
				store := newMockSyncStateStore()
				store.values[key] = old
				if mode == "seed" || mode == "seed-write" {
					delete(store.values, key)
				}
				boom := errors.New("private store or provider detail")
				if mode == "read" {
					store.GetFn = func(context.Context, string) (string, error) { return "", boom }
				}
				store.SetFn = func(_ context.Context, k, v string) error {
					if (mode == "write" || mode == "seed-write") && k == key {
						return boom
					}
					store.values[k] = v
					return nil
				}
				retry, applied := false, 0
				var seen []string
				fetch := func(since string, page int) (int, error) {
					seen = append(seen, since)
					if mode == "page" && page == 2 {
						return 0, boom
					}
					if mode == "empty" {
						return 0, nil
					}
					if mode == "page" {
						return 2, nil
					}
					return 1, nil
				}
				apply := func() error {
					if !retry && (mode == "apply" || mode == "seed") {
						return boom
					}
					applied++
					return nil
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if mode == "cancel" {
					cancel()
				}
				run := func() {
					if feed == "orders" {
						client := &mocks.MockDHOrdersClient{GetOrdersFn: func(_ context.Context, f dh.OrderFilters) (*dh.OrdersResponse, error) {
							n, err := fetch(f.Since, f.Page)
							r := &dh.OrdersResponse{Meta: dh.PaginationMeta{TotalCount: n}}
							if n > 0 {
								r.Orders = []dh.Order{{OrderID: "o", CertNumber: "c", SoldAt: next, Channel: "dh", Grade: "10"}}
							}
							return r, err
						}}
						svc := &mocks.MockImportService{
							ImportOrdersSalesFn: func(context.Context, []csvimport.OrdersExportRow) (*csvimport.OrdersImportResult, error) {
								if mode == "skip" {
									return &csvimport.OrdersImportResult{NotFound: []csvimport.OrdersImportSkip{{CertNumber: "c"}}}, nil
								}
								return &csvimport.OrdersImportResult{Matched: []csvimport.OrdersImportMatch{{PurchaseID: "p"}}}, nil
							},
							ConfirmOrdersSalesFn: func(context.Context, []csvimport.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
								if apply() != nil {
									return &inventory.BulkSaleResult{Failed: 1}, nil
								}
								return &inventory.BulkSaleResult{Created: 1}, nil
							},
						}
						NewDHOrdersPollScheduler(client, store, svc, nil, mocks.NewMockLogger(), DHOrdersPollConfig{}).poll(ctx)
					} else {
						client := &mocks.MockDHInventoryListClient{ListInventoryFn: func(_ context.Context, f dh.InventoryFilters) (*dh.InventoryListResponse, error) {
							n, err := fetch(f.UpdatedSince, f.Page)
							r := &dh.InventoryListResponse{Meta: dh.PaginationMeta{TotalCount: n}}
							if n > 0 {
								r.Items = []dh.InventoryListItem{{CertNumber: "c", Status: "listed", UpdatedAt: next}}
							}
							return r, err
						}}
						lookup := &mocks.MockPurchaseByCertLookup{GetDHStatusByCertNumberFn: func(context.Context, string) (string, string, error) {
							if mode == "lookup" {
								return "", "", boom
							}
							if mode == "skip" {
								return "", "", nil
							}
							return "p", "", nil
						}}
						updater := &mocks.MockDHFieldsUpdater{UpdatePurchaseDHFieldsFn: func(context.Context, string, inventory.DHFieldsUpdate) error { return apply() }}
						NewDHInventoryPollScheduler(client, store, updater, lookup, nil, mocks.NewMockLogger(), DHInventoryPollConfig{}).poll(ctx)
					}
				}
				run()
				clean := mode == "empty" || mode == "clean" || mode == "skip" || (feed == "orders" && mode == "lookup")
				if clean {
					stamp, err := time.Parse(time.RFC3339, store.values[marker])
					require.NoError(t, err)
					require.WithinDuration(t, time.Now(), stamp, 2*time.Second)
				} else {
					require.Empty(t, store.values[marker])
				}
				if !clean && mode != "seed" && mode != "seed-write" {
					require.Equal(t, old, store.values[key])
				}
				if mode == "read" || (feed == "orders" && mode == "seed-write") {
					require.Empty(t, seen)
				}
				if mode == "apply" || mode == "seed" {
					bound := store.values[key]
					if mode == "apply" {
						require.Equal(t, old, bound)
					} else if feed == "orders" {
						require.NotEmpty(t, bound)
					}
					require.Zero(t, applied)
					retry = true
					run() // A new scheduler instance must replay the persisted window.
					require.Equal(t, seen[0], seen[len(seen)-1])
					require.Equal(t, next, store.values[key])
					require.NotEmpty(t, store.values[marker])
					require.Equal(t, 1, applied)
				}
			})
		}
	}
}
