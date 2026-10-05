package postgres

import (
	"context"
	"errors"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestConfirmedReturnListingIntegration(t *testing.T) {
	for _, target := range []int{147840, 364577} {
		t.Run(string(rune(target)), func(t *testing.T) {
			db, store, returns, fake, id := setupReturnCore(t, "listing-cert", target, "")
			ctx := context.Background()
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				return &inventory.DHReturnResult{DHInventoryID: target, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
			}
			state, err := returns.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true})
			require.NoError(t, err)
			fields := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			calls := 0
			lister := &mocks.DHInventoryListerMock{UpdateInventoryStatusFn: func(_ context.Context, n int, u inventory.DHInventoryStatusUpdate) (int, error) {
				calls++
				require.Equal(t, target, n)
				require.Equal(t, 25000, u.ListingPriceCents)
				return 25000, nil
			}}
			coord := inventory.NewDHMutationCoordinator(store, store)
			svc, err := dhlisting.NewDHListingService(fields, mocks.NewMockLogger(), dhlisting.WithDHListingLister(lister), dhlisting.WithDHListingFieldsUpdater(fields), dhlisting.WithDHListingMutationCoordinator(coord, store, store), dhlisting.WithDHListingConfigLoader(NewDHStore(db.DB, mocks.NewMockLogger())))
			require.NoError(t, err)
			auto := svc.ListPurchases(ctx, []string{"listing-cert"})
			require.Zero(t, auto.Listed)
			require.Zero(t, calls)
			require.ErrorIs(t, auto.FailedCerts["listing-cert"], inventory.ErrReturnConflict)
			price := dhpricing.NewService(fields, lister, fields, fields, mocks.NewMockLogger(), dhpricing.WithMutationCoordinator(coord, store, store))
			require.Equal(t, dhpricing.OutcomeError, price.SyncPurchasePrice(ctx, id).Outcome)
			require.Zero(t, calls)
			fetched, err := store.ObservationTime(ctx)
			require.NoError(t, err)
			explicit := svc.(dhlisting.ExplicitListingService).ListPurchaseExplicit(ctx, id, "listing-cert", state.Operation.ID)
			require.Equal(t, 1, explicit.Listed)
			require.Equal(t, 1, calls)
			applied, err := store.ApplyDHObservation(ctx, id, target, fetched, func(c context.Context) error { return fields.UpdatePurchaseDHStatus(c, id, "in_stock") })
			require.NoError(t, err)
			require.False(t, applied)
			p, err := fields.GetPurchase(ctx, id)
			require.NoError(t, err)
			require.Equal(t, "listed", p.DHStatus)
		})
	}
}
func TestListingUnknownSyncNeverReverts(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "sync-cert", 42, "")
	ctx := context.Background()
	fields := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	patches := 0
	lister := &mocks.DHInventoryListerMock{UpdateInventoryStatusFn: func(context.Context, int, inventory.DHInventoryStatusUpdate) (int, error) {
		patches++
		return 25000, nil
	}, SyncChannelsFn: func(context.Context, int, []string) error { return context.DeadlineExceeded }}
	svc, err := dhlisting.NewDHListingService(fields, mocks.NewMockLogger(), dhlisting.WithDHListingLister(lister), dhlisting.WithDHListingFieldsUpdater(fields), dhlisting.WithDHListingMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store), dhlisting.WithDHListingConfigLoader(NewDHStore(db.DB, mocks.NewMockLogger())))
	require.NoError(t, err)
	require.Zero(t, svc.ListPurchases(ctx, []string{"sync-cert"}).Listed)
	require.Equal(t, 1, patches)
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.Zero(t, svc.ListPurchases(ctx, []string{"sync-cert"}).Listed)
	require.Equal(t, 1, patches)
}
func TestListingPreparationCommitBeforeExecution(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "commit-cert", 42, "")
	ctx := context.Background()
	fields := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	lister := &mocks.DHInventoryListerMock{UpdateInventoryStatusFn: func(context.Context, int, inventory.DHInventoryStatusUpdate) (int, error) {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM dh_mutation_attempts WHERE outcome='open'`).Scan(&n))
		require.Equal(t, 1, n)
		return 0, errors.New("unknown")
	}}
	svc, err := dhlisting.NewDHListingService(fields, mocks.NewMockLogger(), dhlisting.WithDHListingLister(lister), dhlisting.WithDHListingFieldsUpdater(fields), dhlisting.WithDHListingMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store), dhlisting.WithDHListingConfigLoader(NewDHStore(db.DB, mocks.NewMockLogger())))
	require.NoError(t, err)
	svc.ListPurchases(ctx, []string{"commit-cert"})
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.WithinDuration(t, time.Now(), state.PrecedingAttempt.StartedAt, time.Minute)
}
