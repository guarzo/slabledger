package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnThenListingObservationSettlementFence(t *testing.T) {
	db, store, svc, fake, id := setupReturnCore(t, "watermark-cert", 364577, "")
	ctx := context.Background()
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true})
	require.NoError(t, err)
	fetchedAfterReturn, err := store.ObservationTime(ctx)
	require.NoError(t, err)
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AuthorizeReturnedListing(c, id, state.Operation.ID) }))
	coord := inventory.NewDHMutationCoordinator(store, store)
	var fetchedDuringMutation time.Time
	require.NoError(t, coord.Run(ctx, id, func(context.Context) (inventory.DHMutationRequest, error) {
		return inventory.DHMutationRequest{Kind: "listing", Phase: "patch_sync", PayloadIdentity: "price-25000-ebay"}, nil
	}, func(c context.Context, _ *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		// Fetch AFTER dispatch marker: only the settlement watermark invalidates it.
		var err error
		fetchedDuringMutation, err = store.ObservationTime(c)
		if err != nil {
			return nil, err
		}
		err = NewPurchaseStore(db.DB, mocks.NewMockLogger()).UpdatePurchaseDHFields(c, id, inventory.DHFieldsUpdate{InventoryID: 364577, DHStatus: "listed", ListingPriceCents: 25000, ChannelsJSON: `["ebay"]`})
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: "verified-patch-and-sync"}, err
	}))
	for _, fetched := range []time.Time{fetchedAfterReturn, fetchedDuringMutation} {
		applied, err := store.ApplyDHObservation(ctx, id, 364577, fetched, func(context.Context) error { t.Fatal("delayed in_stock applied after listing"); return nil })
		require.NoError(t, err)
		require.False(t, applied)
	}
	fresh, err := store.ObservationTime(ctx)
	require.NoError(t, err)
	applied, err := store.ApplyDHSoldObservation(ctx, id, "removed-old-sale", 364577, fresh, func(context.Context) error { t.Fatal("sold write without current sale"); return nil })
	require.NoError(t, err)
	require.False(t, applied)
	returned, err := store.IsReturnedOrder(ctx, id, "ext-848")
	require.NoError(t, err)
	require.True(t, returned)
	returned, err = store.IsReturnedOrder(ctx, id, "ext-999")
	require.NoError(t, err)
	require.False(t, returned)
	p, err := NewPurchaseStore(db.DB, mocks.NewMockLogger()).GetPurchase(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "listed", p.DHStatus)
	require.Equal(t, 25000, p.DHListingPriceCents)
}

func TestConfirmedReturnSettlementCommitFailure(t *testing.T) {
	db, store, svc, fake, id := setupReturnCore(t, "settle-failure-cert", 364577, "")
	ctx := context.Background()
	// The transaction reaches settlement successfully; COMMIT alone fails.
	_, err := db.ExecContext(ctx, `CREATE FUNCTION fail_attempt_settlement_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.outcome='succeeded' THEN RAISE EXCEPTION 'settlement commit rejected'; END IF; RETURN NEW; END $$; CREATE CONSTRAINT TRIGGER fail_settlement_commit AFTER UPDATE ON dh_mutation_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_attempt_settlement_commit()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_settlement_commit ON dh_mutation_attempts; DROP FUNCTION IF EXISTS fail_attempt_settlement_commit()`)
	})
	key := ""
	fake.ReturnInventoryToStockFn = func(_ context.Context, _ int, k string) (*inventory.DHReturnResult, error) {
		if key == "" {
			key = k
		} else {
			require.Equal(t, key, k)
		}
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true})
	require.Error(t, err)
	require.Equal(t, "pending", state.Operation.State)
	require.Equal(t, "settlement", state.Operation.LastError.Phase)
	require.NotNil(t, state.PrecedingAttempt)
	require.NotNil(t, state.Operation.ObservedReceipt)
	require.ErrorIs(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }), inventory.ErrReturnConflict)
	_, err = db.ExecContext(ctx, `DROP TRIGGER fail_settlement_commit ON dh_mutation_attempts; DROP FUNCTION fail_attempt_settlement_commit()`)
	require.NoError(t, err)
	state, err = svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, OperationID: state.Operation.ID})
	require.NoError(t, err)
	require.Equal(t, "completed", state.Operation.State)
}
