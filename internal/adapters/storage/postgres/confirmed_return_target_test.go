package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnFreshCompletionIdentity(t *testing.T) {
	for _, tt := range []struct{ name, change string }{{"changed sale", `UPDATE campaign_sales SET id='replacement-sale' WHERE id='old-sale'`}, {"changed inventory linkage", `UPDATE campaign_purchases SET dh_inventory_id=99 WHERE id='return-p'`}} {
		t.Run(tt.name, func(t *testing.T) {
			db, _, svc, fake, id := setupReturnCore(t, "target-race-cert", 364577, "ext-848")
			ctx := context.Background()
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				_, err := db.ExecContext(ctx, tt.change)
				require.NoError(t, err)
				return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
			}
			sale := "old-sale"
			state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale})
			require.ErrorIs(t, err, inventory.ErrReturnConflict)
			require.Equal(t, "conflicted", state.Operation.State)
			require.NotNil(t, state.Sale)
			require.Empty(t, state.Operation.ReturnedOrderID)
			require.NotNil(t, state.PrecedingAttempt)
		})
	}
}
