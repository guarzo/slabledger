//go:build integration

package confirmedreturns_test

import (
	"context"
	"fmt"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

func setupIntegrationReturn(t *testing.T, cert string) (*postgres.DB, *postgres.ConfirmedReturnStore, *inventory.ConfirmedReturnService, *mocks.DHReturnerMock, string) {
	t.Helper()
	ctx := context.Background()
	db := openConfirmedReturnsTestDB(t, ctx)
	require.NoError(t, postgres.RunMigrations(db, ""))
	_, e := db.ExecContext(ctx, `TRUNCATE campaigns CASCADE; TRUNCATE confirmed_dh_returns,dh_mutation_attempts,dh_target_watermarks; INSERT INTO campaigns(id,name,phase,created_at,updated_at) VALUES('return-c','Returns','pending',now(),now())`)
	require.NoError(t, e)
	_, e = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,dh_inventory_id,dh_status,dh_push_status,reviewed_price_cents,received_at,purchase_date,created_at,updated_at) VALUES('return-p','return-c','Card',$1,'PSA',42,'in_stock','matched',25000,now(),'2026-01-01',now(),now())`, cert)
	require.NoError(t, e)
	store := postgres.NewConfirmedReturnStore(db.DB)
	fake := &mocks.DHReturnerMock{GetReturnInventoryStatusFn: func(context.Context, int, string) (string, error) { return "sold", nil }}
	n := 0
	svc := inventory.NewConfirmedReturnService(store, store, fake, nil, func() string { n++; return fmt.Sprintf("op-%d", n) })
	return db, store, svc, fake, "return-p"
}
func observedTarget(cert string) *inventory.ReturnTargetIdentity {
	return &inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: cert, Grader: "PSA"}
}

func TestConfirmedReturnOrdersRunOnceIdentity(t *testing.T) {
	for _, external := range []int64{443, 848} {
		for _, oldFirst := range []bool{false, true} {
			for _, paged := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/oldFirst=%v/paged=%v", external, oldFirst, paged), func(t *testing.T) {
					db, store, returns, fake, id := setupIntegrationReturn(t, "orders-cert")
					ctx := context.Background()
					fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
						return &inventory.DHReturnResult{DHInventoryID: 42, ItemStatus: "in_stock", ExternalSaleID: external}, nil
					}
					_, err := returns.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedTarget("orders-cert")})
					require.NoError(t, err)
					p := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
					sa := postgres.NewSaleStore(db.DB, mocks.NewMockLogger())
					imports := csvimport.NewService(csvimport.Deps{Campaigns: postgres.NewCampaignStore(db.DB, mocks.NewMockLogger()), Purchases: p, Sales: sa, Finance: postgres.NewFinanceStore(db.DB, mocks.NewMockLogger()), IDGen: func() string { return "new-order-sale" }, Logger: mocks.NewMockLogger(), MutationGuards: store})
					old := dh.Order{OrderID: fmt.Sprintf("ext-%d", external), CertNumber: "orders-cert", SoldAt: "2026-01-03T01:00:00Z", Channel: "ebay", SalePriceCents: 9999, Grade: "10"}
					newer := dh.Order{OrderID: "ext-99999", CertNumber: "orders-cert", SoldAt: "2026-02-07T01:00:00Z", Channel: "ebay", SalePriceCents: 27273, Grade: "10"}
					orders := []dh.Order{newer, old}
					if oldFirst {
						orders = []dh.Order{old, newer}
					}
					remote := &mocks.MockDHOrdersClient{GetOrdersFn: func(_ context.Context, f dh.OrderFilters) (*dh.OrdersResponse, error) {
						r := &dh.OrdersResponse{Orders: orders}
						if paged {
							r.Orders = orders[f.Page-1 : f.Page]
						}
						r.Meta.TotalCount = 2
						return r, nil
					}}
					events := &mocks.MockEventRecorder{}
					poll := scheduler.NewDHOrdersPollScheduler(remote, nil, imports, events, mocks.NewMockLogger(), scheduler.DHOrdersPollConfig{})
					summary, err := poll.RunOnce(ctx, "2026-01-01")
					require.NoError(t, err)
					require.Equal(t, 1, summary.Matched)
					require.Equal(t, 1, summary.Returned)
					require.Zero(t, summary.Failed)
					sale, err := sa.GetSaleByPurchaseID(ctx, id)
					require.NoError(t, err)
					require.Equal(t, "ext-99999", sale.OrderID)
					require.Equal(t, 27273, sale.SalePriceCents)
					require.Equal(t, "2026-02-07", sale.SaleDate)
					sold := 0
					for _, event := range events.Events {
						if event.Type == dhevents.TypeSold {
							sold++
							require.Equal(t, "ext-99999", event.DHOrderID)
							require.Equal(t, 27273, event.SalePriceCents)
						}
					}
					require.Equal(t, 1, sold)
				})
			}
		}
	}
}
