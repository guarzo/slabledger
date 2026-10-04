package postgres

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoordinatedGenericUnsellRetainsUncertainty(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "uncertain"}[failed], func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "unsell-cert", 42, "ext-848")
			ctx := context.Background()
			_, e := db.ExecContext(ctx, `UPDATE campaign_sales SET order_id='',dh_sale_id='handle' WHERE id='old-sale'; UPDATE campaign_purchases SET dh_status='sold' WHERE id='return-p'`)
			require.NoError(t, e)
			p := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			sa := NewSaleStore(db.DB, mocks.NewMockLogger())
			remote := &mocks.DHSaleRecorderMock{VoidInventorySaleFn: func(context.Context, string, string) error {
				if failed {
					return context.DeadlineExceeded
				}
				return nil
			}}
			svc := inventory.NewService(NewCampaignStore(db.DB, mocks.NewMockLogger()), p, sa, nil, NewFinanceStore(db.DB, mocks.NewMockLogger()), nil, nil, inventory.WithIDGenerator(func() string { return "mint" }), inventory.WithDisableBackgroundWorkers(), inventory.WithDHSaleRecorder(remote), inventory.WithDHMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store, store))
			require.NoError(t, svc.DeleteSaleByPurchaseID(ctx, id))
			state, e := store.GetReturnState(ctx, id)
			require.NoError(t, e)
			require.Nil(t, state.Sale)
			if failed {
				require.NotNil(t, state.PrecedingAttempt)
				require.ErrorIs(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }), inventory.ErrReturnConflict)
			} else {
				require.Nil(t, state.PrecedingAttempt)
			}
		})
	}
}
