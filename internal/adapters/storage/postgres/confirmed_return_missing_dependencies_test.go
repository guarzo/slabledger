package postgres

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPriceWriterCannotBypassReturnedHold(t *testing.T) {
	db, store, returns, fake, id := setupReturnCore(t, "price-writer", 42, "")
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		return &inventory.DHReturnResult{DHInventoryID: 42, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	_, e := returns.ConfirmReturn(context.Background(), id, inventory.ConfirmReturnRequest{ReturnConfirmed: true})
	require.NoError(t, e)
	repo := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	at, e := store.ObservationTime(context.Background())
	require.NoError(t, e)
	require.ErrorIs(t, repo.UpdatePurchaseDHPriceSync(context.Background(), id, 100, at), inventory.ErrReturnConflict)
}
func TestConfiguredListingMissingDependenciesFailsClosed(t *testing.T) {
	for _, missing := range []string{"fields", "config", "typed-nil-guards", "typed-nil-reader"} {
		t.Run(missing, func(t *testing.T) {
			db, store, _, _, _ := setupReturnCore(t, "missing-cert", 42, "")
			p := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			calls := 0
			lister := &mocks.DHInventoryListerMock{UpdateInventoryStatusFn: func(context.Context, int, inventory.DHInventoryStatusUpdate) (int, error) { calls++; return 25000, nil }}
			var guards inventory.DHMutationGuards = store
			var reader dhlisting.ReturnStateReader = store
			var absent *ConfirmedReturnStore
			if missing == "typed-nil-guards" {
				guards = absent
			}
			if missing == "typed-nil-reader" {
				reader = absent
			}
			opts := []dhlisting.DHListingServiceOption{dhlisting.WithDHListingLister(lister), dhlisting.WithDHListingMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), guards, reader)}
			if missing != "fields" {
				opts = append(opts, dhlisting.WithDHListingFieldsUpdater(p))
			}
			if missing != "config" {
				opts = append(opts, dhlisting.WithDHListingConfigLoader(NewDHStore(db.DB, mocks.NewMockLogger())))
			}
			svc, e := dhlisting.NewDHListingService(p, mocks.NewMockLogger(), opts...)
			require.NoError(t, e)
			require.NotPanics(t, func() {
				result := svc.ListPurchases(context.Background(), []string{"missing-cert"})
				require.Zero(t, result.Listed)
				require.ErrorIs(t, result.FailedCerts["missing-cert"], inventory.ErrReturnConflict)
			})
			require.Zero(t, calls)
		})
	}
}
