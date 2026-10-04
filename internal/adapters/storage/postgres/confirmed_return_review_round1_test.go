package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewedReturnRequestKey struct{}

func TestConfirmedReturnConcurrentPreparedAttribution(t *testing.T) {
	db, store, _, fake, id := setupReturnCore(t, "round1-concurrent-cert", 364577, "")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `CREATE FUNCTION fail_round1_completion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='completed' THEN RAISE EXCEPTION 'completion unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_round1_completion BEFORE UPDATE ON confirmed_dh_returns FOR EACH ROW EXECUTE FUNCTION fail_round1_completion()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_round1_completion ON confirmed_dh_returns; DROP FUNCTION IF EXISTS fail_round1_completion()`)
	})

	preparedA, preparedB := make(chan struct{}), make(chan struct{})
	executeA, executeB := make(chan struct{}), make(chan struct{})
	var stagesA, stagesB, generated atomic.Int32
	var releaseA, releaseB sync.Once
	t.Cleanup(func() { releaseA.Do(func() { close(executeA) }); releaseB.Do(func() { close(executeB) }) })
	scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, p string, fn func(context.Context) error) error {
		label, _ := c.Value(reviewedReturnRequestKey{}).(string)
		var first bool
		switch label {
		case "A":
			first = stagesA.Add(1) == 1
		case "B":
			first = stagesB.Add(1) == 1
		}
		err := store.WithPurchaseMutation(c, p, fn)
		if err != nil || !first {
			return err
		}
		// Pause AFTER preparation COMMIT, BEFORE either execution can own the target.
		switch label {
		case "A":
			close(preparedA)
			<-executeA
		case "B":
			close(preparedB)
			<-executeB
		}
		return nil
	}}
	svc := inventory.NewConfirmedReturnService(scope, store, fake, nil, func() string { return fmt.Sprintf("round1-generated-%d", generated.Add(1)) })
	fake.ReturnInventoryToStockFn = func(c context.Context, target int, key string) (*inventory.DHReturnResult, error) {
		external := int64(999)
		if c.Value(reviewedReturnRequestKey{}) == "A" {
			external = 848
		}
		return &inventory.DHReturnResult{DHInventoryID: target, ItemStatus: "in_stock", ExternalSaleID: external}, nil
	}
	type result struct {
		state *inventory.ConfirmedReturnState
		err   error
	}
	doneA, doneB := make(chan result, 1), make(chan result, 1)
	req := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
	go func() {
		s, e := svc.ConfirmReturn(context.WithValue(ctx, reviewedReturnRequestKey{}, "A"), id, req)
		doneA <- result{s, e}
	}()
	<-preparedA
	go func() {
		s, e := svc.ConfirmReturn(context.WithValue(ctx, reviewedReturnRequestKey{}, "B"), id, req)
		doneB <- result{s, e}
	}()
	<-preparedB
	// Both callers captured a receipt-free episode; A fails completion and
	// durably records ext-848 before B is allowed to execute its stale preparation.
	releaseA.Do(func() { close(executeA) })
	a := <-doneA
	require.Error(t, a.err)
	require.NotNil(t, a.state.Operation.ObservedReceipt)
	require.Equal(t, int64(848), a.state.Operation.ObservedReceipt.ExternalSaleID)
	key := a.state.Operation.Key
	_, err = db.ExecContext(ctx, `DROP TRIGGER fail_round1_completion ON confirmed_dh_returns; DROP FUNCTION fail_round1_completion()`)
	require.NoError(t, err)
	releaseB.Do(func() { close(executeB) })
	b := <-doneB
	require.ErrorIs(t, b.err, inventory.ErrReturnConflict)
	assert.Equal(t, "response", b.state.Operation.LastError.Phase, "execution must validate the freshly resolved episode")
	assert.Equal(t, int64(848), b.state.Operation.ObservedReceipt.ExternalSaleID, "a stale failure must not replace first valid attribution")
	// The conflicting value must never become authority for a third retry.
	later, err := svc.ConfirmReturn(ctx, id, req)
	assert.ErrorIs(t, err, inventory.ErrReturnConflict)
	assert.NotEqual(t, "completed", later.Operation.State)
	assert.Equal(t, key, later.Operation.Key)
	fake.ReturnInventoryToStockFn = func(_ context.Context, target int, k string) (*inventory.DHReturnResult, error) {
		require.Equal(t, key, k)
		return &inventory.DHReturnResult{DHInventoryID: target, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	restored, err := svc.ConfirmReturn(ctx, id, req)
	require.NoError(t, err)
	require.Equal(t, "ext-848", restored.Operation.ReturnedOrderID)
}

func TestRecordReturnFailurePreservesFreshValidAttribution(t *testing.T) {
	for _, invalidFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid first", true: "invalid observation replaced by first valid receipt"}[invalidFirst], func(t *testing.T) {
			_, store, _, _, id := setupReturnCore(t, "round1-record-cert", 364577, "")
			ctx := context.Background()
			var episode *inventory.ConfirmedReturnEpisode
			require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
				state, err := store.PrepareReturn(c, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true}, "round1-op", "round1-key")
				if err == nil {
					episode = state.Operation
				}
				return err
			}))
			require.Nil(t, episode.ObservedReceipt)
			if invalidFirst {
				require.NoError(t, store.RecordReturnFailure(ctx, id, episode.ID, inventory.ReturnFailure{Code: "wrong_target", Phase: "response"}, &inventory.DHReturnResult{DHInventoryID: 99, ItemStatus: "in_stock", ExternalSaleID: 9}, true))
			}
			require.NoError(t, store.RecordReturnFailure(ctx, id, episode.ID, inventory.ReturnFailure{Code: "local_failure", Phase: "completion"}, &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, false))
			// A stale caller has no observed receipt in its preparation snapshot.
			require.NoError(t, store.RecordReturnFailure(ctx, id, episode.ID, inventory.ReturnFailure{Code: "later_conflict", Phase: "response"}, &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 999}, true))
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Equal(t, int64(848), state.Operation.ObservedReceipt.ExternalSaleID)
			require.Equal(t, "later_conflict", state.Operation.LastError.Code)
		})
	}
}

func TestConfirmedReturnHistoricalBodyAcrossNewEpisodes(t *testing.T) {
	for _, capturedOrder := range []string{"", "ext-848"} {
		t.Run(map[string]string{"": "initial null confirmation", "ext-848": "initial sale confirmation"}[capturedOrder], func(t *testing.T) {
			db, store, svc, fake, id := setupReturnCore(t, "round1-history-cert", 364577, capturedOrder)
			ctx := context.Background()
			original := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
			if capturedOrder != "" {
				sale := "old-sale"
				original.ExpectedSaleID = &sale
			}
			calls := 0
			external := int64(848)
			timeout := false
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				calls++
				if timeout {
					return nil, context.DeadlineExceeded
				}
				return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: external}, nil
			}
			a, err := svc.ConfirmReturn(ctx, id, original)
			require.NoError(t, err)
			require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AuthorizeReturnedListing(c, id, a.Operation.ID) }))
			_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,order_id,created_at,updated_at) VALUES('round1-new-sale',$1,'ebay','2026-01-04','ext-999',now(),now())`, id)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='sold' WHERE id=$1`, id)
			require.NoError(t, err)
			nextSale := "round1-new-sale"
			next := inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &nextSale}
			external = 999
			timeout = true
			b, err := svc.ConfirmReturn(ctx, id, next)
			require.Error(t, err)
			require.NotEqual(t, a.Operation.ID, b.Operation.ID)
			for _, complete := range []bool{false, true} {
				if complete {
					timeout = false
					b, err = svc.ConfirmReturn(ctx, id, next)
					require.NoError(t, err)
				}
				before := calls
				replay, e := svc.ConfirmReturn(ctx, id, original) // identical original body; operationId omitted
				assert.NoError(t, e)
				assert.Equal(t, a.Operation.ID, replay.Operation.ID)
				assert.Equal(t, "completed_replay", replay.Outcome)
				assert.Equal(t, map[bool]string{false: "pending", true: "completed"}[complete], b.Operation.State)
				assert.Equal(t, b.AwaitingListing, replay.AwaitingListing, "latest retained hold is independent of historical replay")
				assert.Equal(t, b.Purchase.DHStatus, replay.Purchase.DHStatus)
				assert.Equal(t, b.ExpectedSaleID, replay.ExpectedSaleID)
				assert.Equal(t, b.Sale, replay.Sale)
				assert.Equal(t, b.PrecedingAttempt, replay.PrecedingAttempt)
				assert.Equal(t, before, calls, "historical body must not dispatch another return")
				current, e := store.GetReturnState(ctx, id)
				require.NoError(t, e)
				require.Equal(t, b.Operation.ID, current.Operation.ID)
				require.Equal(t, b.Operation.State, current.Operation.State)
				require.Equal(t, b.Sale, current.Sale)
			}
			var episodes int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM confirmed_dh_returns`).Scan(&episodes))
			require.Equal(t, 2, episodes)
		})
	}
}

func TestConfirmedReturnPreparedHistoricalBodyAcrossNewEpisodes(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "new episode pending", true: "new episode completed"}[complete], func(t *testing.T) {
			db, store, svc, fake, id := setupReturnCore(t, "round1-prepared-history-cert", 364577, "")
			ctx := context.Background()
			timeout := true
			external := int64(848)
			calls := 0
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				calls++
				if timeout {
					return nil, context.DeadlineExceeded
				}
				return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: external}, nil
			}
			original := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
			_, err := svc.ConfirmReturn(ctx, id, original)
			require.Error(t, err)
			prepared, execute := make(chan struct{}), make(chan struct{})
			var stage atomic.Int32
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(execute) }) })
			scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, p string, fn func(context.Context) error) error {
				first := stage.Add(1) == 1
				err := store.WithPurchaseMutation(c, p, fn)
				if err == nil && first {
					close(prepared)
					<-execute
				}
				return err
			}}
			retry := inventory.NewConfirmedReturnService(scope, store, fake, nil, func() string { return "unused-retry-id" })
			type result struct {
				state *inventory.ConfirmedReturnState
				err   error
			}
			done := make(chan result, 1)
			go func() { s, e := retry.ConfirmReturn(ctx, id, original); done <- result{s, e} }()
			<-prepared
			// Another caller completes A, then the newly observed sale establishes B
			// before the already-prepared old body can acquire execution ownership.
			timeout = false
			a, err := svc.ConfirmReturn(ctx, id, original)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,order_id,created_at,updated_at) VALUES('round1-later-sale',$1,'ebay','2026-01-04','ext-999',now(),now())`, id)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='sold' WHERE id=$1`, id)
			require.NoError(t, err)
			nextSale := "round1-later-sale"
			external = 999
			timeout = !complete
			b, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &nextSale})
			if complete {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			before := calls
			release.Do(func() { close(execute) })
			replay := <-done
			require.NoError(t, replay.err)
			require.Equal(t, a.Operation.ID, replay.state.Operation.ID)
			require.Equal(t, "completed_replay", replay.state.Outcome)
			require.Equal(t, b.Sale, replay.state.Sale)
			require.Equal(t, b.PrecedingAttempt, replay.state.PrecedingAttempt)
			require.Equal(t, b.AwaitingListing, replay.state.AwaitingListing)
			require.Equal(t, before, calls)
		})
	}
}

func TestDHObservationReentrantFlagRestored(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "callback error"}[fail], func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "round1-observe-cert", 364577, "")
			ctx := context.Background()
			fetched, err := store.ObservationTime(ctx)
			require.NoError(t, err)
			ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			sentinel := errors.New("observation callback failure")
			var delayedFetch time.Time
			require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
				applied, e := store.ApplyDHObservation(c, id, 364577, fetched, func(observation context.Context) error {
					scope, err := ownedScope(observation, db.DB, id)
					if err != nil {
						return err
					}
					assert.True(t, scope.observation)
					nested, e := store.ApplyDHObservation(observation, id, 364577, fetched, func(context.Context) error { return nil })
					if e != nil {
						return e
					}
					assert.True(t, nested)
					assert.True(t, scope.observation, "nested observation must restore the outer observation flag")
					if err := ps.UpdatePurchaseDHStatus(observation, id, "in_stock"); err != nil {
						return err
					}
					if fail {
						return sentinel
					}
					return nil
				})
				if fail {
					require.ErrorIs(t, e, sentinel)
					require.False(t, applied)
				} else {
					require.NoError(t, e)
					require.True(t, applied)
				}
				scope, e := ownedScope(c, db.DB, id)
				if e != nil {
					return e
				}
				assert.False(t, scope.observation, "observation flag must not escape callback, including errors")
				delayedFetch, e = store.ObservationTime(c)
				if e != nil {
					return e
				}
				return ps.UpdatePurchaseDHStatus(c, id, "listed") // ordinary mutation must advance settlement watermark
			}))
			applied, err := store.ApplyDHObservation(ctx, id, 364577, delayedFetch, func(c context.Context) error { return ps.UpdatePurchaseDHStatus(c, id, "in_stock") })
			require.NoError(t, err)
			assert.False(t, applied, "pre-list snapshot must be discarded after the ordinary mutation")
			p, err := ps.GetPurchase(ctx, id)
			require.NoError(t, err)
			require.Equal(t, "listed", p.DHStatus)
		})
	}
}
