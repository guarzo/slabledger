package postgres

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGenericUnsellWithPrecedingUncertainSale(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "prior-sale-cert", 42, "ext-848")
	ctx := context.Background()
	_, e := db.ExecContext(ctx, `UPDATE campaign_sales SET order_id='',dh_idempotency_key='prior-key' WHERE id='old-sale'; UPDATE campaign_purchases SET dh_status='sold' WHERE id='return-p'`)
	require.NoError(t, e)
	p := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	sa := NewSaleStore(db.DB, mocks.NewMockLogger())
	coord := inventory.NewDHMutationCoordinator(store, store)
	remote := &mocks.DHSaleRecorderMock{RecordInventorySaleFn: func(context.Context, inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
		return nil, context.DeadlineExceeded
	}}
	e = inventory.RecordCoordinatedDHSale(ctx, coord, p, sa, remote, id, "old-sale", func() string { return "never-mint" })
	require.Error(t, e)
	svc := inventory.NewService(nil, p, sa, nil, nil, nil, nil, inventory.WithIDGenerator(func() string { return "never-mint" }), inventory.WithDisableBackgroundWorkers(), inventory.WithDHSaleRecorder(remote), inventory.WithDHMutationCoordinator(coord, store, store, store))
	require.NoError(t, svc.DeleteSaleByPurchaseID(ctx, id))
	state, e := store.GetReturnState(ctx, id)
	require.NoError(t, e)
	require.Nil(t, state.Sale)
	require.NotNil(t, state.PrecedingAttempt)
	require.Equal(t, "prior-key", state.PrecedingAttempt.Key)
	require.Empty(t, remote.VoidedSales())
}
