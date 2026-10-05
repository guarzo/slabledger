package postgres

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGenericUnsellVoid404IsNotWholeOutcomeProof(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "void-notfound", 42, "ext-848")
	ctx := context.Background()
	_, e := db.ExecContext(ctx, `UPDATE campaign_sales SET order_id='',dh_sale_id='handle' WHERE id='old-sale'; UPDATE campaign_purchases SET dh_status='sold' WHERE id='return-p'`)
	require.NoError(t, e)
	repo := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	sa := NewSaleStore(db.DB, mocks.NewMockLogger())
	remote := &mocks.DHSaleRecorderMock{VoidInventorySaleFn: func(context.Context, string, string) error { return inventory.ErrDHSaleNotFound }}
	svc := inventory.NewService(nil, repo, sa, nil, nil, nil, nil, inventory.WithDisableBackgroundWorkers(), inventory.WithIDGenerator(func() string { return "mint" }), inventory.WithDHSaleRecorder(remote), inventory.WithDHMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store, store))
	require.NoError(t, svc.DeleteSaleByPurchaseID(ctx, id))
	state, e := store.GetReturnState(ctx, id)
	require.NoError(t, e)
	require.Nil(t, state.Sale)
	require.NotNil(t, state.PrecedingAttempt)
}
func TestGenericUnsellCallerCancellationRetainsOpenFence(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "cancel-void", 42, "ext-848")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, e := db.ExecContext(ctx, `UPDATE campaign_sales SET order_id='',dh_sale_id='handle' WHERE id='old-sale'; UPDATE campaign_purchases SET dh_status='sold' WHERE id='return-p'`)
	require.NoError(t, e)
	repo := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	sa := NewSaleStore(db.DB, mocks.NewMockLogger())
	remote := &mocks.DHSaleRecorderMock{VoidInventorySaleFn: func(context.Context, string, string) error { cancel(); return context.Canceled }}
	svc := inventory.NewService(nil, repo, sa, nil, nil, nil, nil, inventory.WithDisableBackgroundWorkers(), inventory.WithIDGenerator(func() string { return "mint" }), inventory.WithDHSaleRecorder(remote), inventory.WithDHMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store, store))
	require.NoError(t, svc.DeleteSaleByPurchaseID(ctx, id))
	state, e := store.GetReturnState(context.Background(), id)
	require.NoError(t, e)
	require.Nil(t, state.Sale)
	require.NotNil(t, state.PrecedingAttempt)
	require.Len(t, remote.VoidedSales(), 1)
}
