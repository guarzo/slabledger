package postgres

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReconcilerDelayedObservationAfterExplicitList(t *testing.T) {
	db, store, returns, fake, id := setupReturnCore(t, "reconcile-cert", 42, "")
	ctx := context.Background()
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		return &inventory.DHReturnResult{DHInventoryID: 42, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	state, e := returns.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedReturnTarget("reconcile-cert", 42)})
	require.NoError(t, e)
	fields := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	listing, e := dhlisting.NewDHListingService(fields, mocks.NewMockLogger(), dhlisting.WithDHListingLister(&mocks.DHInventoryListerMock{}), dhlisting.WithDHListingFieldsUpdater(fields), dhlisting.WithDHListingMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store), dhlisting.WithDHListingConfigLoader(NewDHStore(db.DB, mocks.NewMockLogger())))
	require.NoError(t, e)
	snapshot := &mocks.DHInventorySnapshotFetcherMock{FetchAllInventoryFn: func(c context.Context) (map[int]string, error) {
		r := listing.(dhlisting.ExplicitListingService).ListPurchaseExplicit(c, id, "reconcile-cert", state.Operation.ID)
		require.Equal(t, 1, r.Listed)
		return map[int]string{42: "in_stock"}, nil
	}}
	reconciler, e := dhlisting.NewReconciler(snapshot, fields, fields, mocks.NewMockLogger(), dhlisting.WithReconcileStatusRepairer(fields), dhlisting.WithReconcileMutationGuards(store, store))
	require.NoError(t, e)
	result, e := reconciler.Reconcile(ctx)
	require.NoError(t, e)
	require.Zero(t, result.StatusRepaired)
	p, e := fields.GetPurchase(ctx, id)
	require.NoError(t, e)
	require.Equal(t, "listed", p.DHStatus)
}
