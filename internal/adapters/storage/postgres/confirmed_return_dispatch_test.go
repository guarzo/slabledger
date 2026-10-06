package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnDispatchAndPreflight(t *testing.T) {
	tests := []struct {
		name                        string
		target                      int
		order, channel, key, status string
		want                        string
		wantError                   bool
	}{
		{name: "unlinked local only", order: "", channel: "local", key: "historical-key", want: "local"},
		{name: "off-platform void seam", target: 364577, channel: "local", key: "local-sale-key", want: "legacy_void"},
		{name: "linked legacy recovery", target: 364577, channel: "ebay", want: "completed"},
		{name: "DH native excluded", target: 364577, order: "native-10", channel: "dh", wantError: true},
		{name: "other imported excluded", target: 364577, order: "ext-848", channel: "other", wantError: true},
		{name: "contradictory identities", target: 364577, order: "ext-848", channel: "ebay", key: "key", wantError: true},
		{name: "already in stock", target: 364577, status: "in_stock", want: "no_return_required"},
		{name: "already listed", target: 364577, status: "listed", want: "no_return_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, store, _, fake, id := setupReturnCore(t, "dispatch-cert", tt.target, "")
			ctx := context.Background()
			req := inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedReturnTarget("dispatch-cert", tt.target)}
			if tt.channel != "" {
				sale := "dispatch-sale"
				req.ExpectedSaleID = &sale
				_, err := db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,order_id,dh_idempotency_key,created_at,updated_at) VALUES($1,$2,$3,'2026-01-03',$4,$5,now(),now())`, sale, id, tt.channel, tt.order, tt.key)
				require.NoError(t, err)
			}
			fake.GetReturnInventoryStatusFn = func(context.Context, int, string) (string, error) { return tt.status, nil }
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				if tt.want != "completed" {
					t.Fatal("wrong source dispatched external return")
				}
				return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
			}
			legacy := func(c context.Context, p string, expected string) error {
				require.Equal(t, "legacy_void", tt.want)
				require.Equal(t, "dispatch-sale", expected)
				// Legacy integration can open its separate durable preparation, not a
				// second transaction nested inside confirmed-return execution ownership.
				require.NoError(t, store.AssertDHPreparationAllowed(c))
				return store.WithPurchaseMutation(c, p, func(owned context.Context) error {
					state, err := store.ResolveReturn(owned, p, req)
					if err != nil {
						return err
					}
					require.Equal(t, expected, state.Sale.ID)
					_, err = executor(owned, db.DB).ExecContext(owned, `DELETE FROM campaign_sales WHERE id=$1`, expected)
					return err
				})
			}
			svc := inventory.NewConfirmedReturnService(store, store, fake, legacy, func() string { return "dispatch-operation" })
			state, err := svc.ConfirmReturn(ctx, id, req)
			if tt.wantError {
				require.ErrorIs(t, err, inventory.ErrReturnConflict)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, state.Outcome)
			}
			if tt.want != "completed" {
				var n int
				require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM confirmed_dh_returns`).Scan(&n))
				require.Zero(t, n)
			}
		})
	}
}
