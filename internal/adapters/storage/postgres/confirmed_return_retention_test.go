package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnRetainedAwaitingHold(t *testing.T) {
	db, store, svc, fake, id := setupReturnCore(t, "retain-cert", 364577, "")
	ctx := context.Background()
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedReturnTarget("retain-cert", 364577)})
	require.NoError(t, err)
	ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	require.NoError(t, ps.DeletePurchase(ctx, id))
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,dh_inventory_id,received_at,purchase_date,created_at,updated_at) VALUES('retain-new','return-c','Card','retain-cert','PSA',364577,now(),'2026-01-01',now(),now())`)
	require.NoError(t, err)
	id = "retain-new"
	retained, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.True(t, retained.AwaitingListing)
	require.Equal(t, state.Operation.ID, retained.Operation.ID)
	require.Equal(t, "return-p", retained.Operation.CapturedPurchaseID)
	mutations := []struct {
		name string
		run  func() error
	}{
		{"unmatch", func() error { return ps.UnmatchPurchaseDH(ctx, id, "pending") }},
		{"repush", func() error { return ps.ResetDHFieldsForRepush(ctx, id) }},
		{"reset for relist", func() error { return ps.ResetDHFieldsForRelistAfterVoid(ctx, id) }},
		{"price/status overwrite", func() error {
			return ps.UpdatePurchaseDHFields(ctx, id, inventory.DHFieldsUpdate{InventoryID: 364577, DHStatus: "listed", ListingPriceCents: 90000})
		}},
		{"push status", func() error { return ps.UpdatePurchaseDHPushStatus(ctx, id, "pending") }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) { require.ErrorIs(t, tt.run(), inventory.ErrReturnConflict) })
	}
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=0 WHERE id=$1`, id)
	require.NoError(t, err)
	require.ErrorIs(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }), inventory.ErrReturnConflict)
	require.ErrorIs(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AuthorizeReturnedListing(c, id, state.Operation.ID) }), inventory.ErrReturnConflict)
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=364577 WHERE id=$1`, id)
	require.NoError(t, err)
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AuthorizeReturnedListing(c, id, state.Operation.ID) }))
	require.NoError(t, ps.UpdatePurchaseDHStatus(ctx, id, "listed"))
}

func TestDHMutationCrashAndCancellationFences(t *testing.T) {
	db, store, svc, fake, id := setupReturnCore(t, "crash-cert", 364577, "")
	ctx := context.Background()
	// Preparation committed but no dispatch: conservatively possibly dispatched.
	var a *inventory.DHMutationAttempt
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
		var e error
		a, e = store.PrepareDHMutation(c, id, inventory.DHMutationRequest{Kind: "listing", Phase: "patch_sync", PayloadIdentity: "immutable-orchestration"})
		return e
	}))
	require.NotNil(t, a)
	ctxCancelled, cancel := context.WithCancel(ctx)
	// Fake older remote work continues after local ownership/cancellation ends.
	remoteContinues := make(chan struct{})
	remoteDone := make(chan struct{})
	providerStatus := "in_stock"
	fake.GetReturnInventoryStatusFn = func(context.Context, int, string) (string, error) { return "in_stock", nil }
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		t.Fatal("return dispatched while preceding remote request still executing")
		return nil, nil
	}
	err := store.WithPurchaseMutation(ctxCancelled, id, func(c context.Context) error {
		require.NoError(t, store.OwnDHMutation(c, id, a))
		// Simulated provider work is intentionally independent of c cancellation.
		go func() { <-remoteContinues; providerStatus = "listed"; close(remoteDone) }()
		cancel()
		return c.Err()
	})
	require.Error(t, err)
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(context.Context) error { return nil }))
	_, err = svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedReturnTarget("crash-cert", 364577)})
	require.ErrorIs(t, err, inventory.ErrReturnConflict)
	fetched, err := store.ObservationTime(ctx)
	require.NoError(t, err)
	applied, err := store.ApplyDHObservation(ctx, id, 364577, fetched, func(context.Context) error { t.Fatal("observation cleared journal"); return nil })
	require.NoError(t, err)
	require.False(t, applied)
	require.ErrorIs(t, NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(ctx, id), inventory.ErrReturnConflict)
	require.Equal(t, "in_stock", providerStatus)
	close(remoteContinues)
	<-remoteDone
	require.Equal(t, "listed", providerStatus)
	require.ErrorIs(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }), inventory.ErrReturnConflict)
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.Equal(t, a.ID, state.PrecedingAttempt.ID)
}

func TestDHMutationNestedPreparationAndCommitFailure(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "nested-cert", 364577, "")
	ctx := context.Background()
	coordinator := inventory.NewDHMutationCoordinator(store, store)
	prepare := func(context.Context) (inventory.DHMutationRequest, error) {
		return inventory.DHMutationRequest{Kind: "price", Phase: "patch", PayloadIdentity: "price-1"}, nil
	}
	execute := func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		t.Fatal("nested/preparation commit failure dispatched")
		return nil, nil
	}
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
		require.ErrorIs(t, coordinator.Run(c, id, prepare, execute), inventory.ErrReturnConflict)
		return nil
	}))
	// A deferred trigger makes SQL preparation succeed but COMMIT fail.
	_, err := db.ExecContext(ctx, `CREATE FUNCTION fail_attempt_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'prepare commit rejected'; END $$; CREATE CONSTRAINT TRIGGER fail_attempt_commit AFTER INSERT ON dh_mutation_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_attempt_commit()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_attempt_commit ON dh_mutation_attempts; DROP FUNCTION IF EXISTS fail_attempt_commit()`)
	})
	require.Error(t, coordinator.Run(ctx, id, prepare, execute))
	var n int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM dh_mutation_attempts`).Scan(&n))
	require.Zero(t, n)
}

func TestConfirmedReturnDuplicateCrossingCompletion(t *testing.T) {
	db, _, svc, fake, id := setupReturnCore(t, "duplicate-cert", 364577, "")
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		calls++
		close(entered)
		<-release
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	req := inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedReturnTarget("duplicate-cert", 364577)}
	done := make(chan error, 2)
	go func() { _, err := svc.ConfirmReturn(ctx, id, req); done <- err }()
	<-entered
	go func() { _, err := svc.ConfirmReturn(ctx, id, req); done <- err }()
	// The second request is submitted while the first execution owns the scope.
	time.Sleep(20 * time.Millisecond)
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	require.Equal(t, 1, calls)
	var n int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM confirmed_dh_returns`).Scan(&n))
	require.Equal(t, 1, n)
}
