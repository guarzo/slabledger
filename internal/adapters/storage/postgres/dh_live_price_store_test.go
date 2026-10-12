package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func livePriceRequest() inventory.DHMutationRequest {
	return inventory.DHMutationRequest{Kind: "price", Phase: "live_price", Key: "immutable-price-key", PayloadIdentity: `{"ID":364577,"Price":25000}`}
}

func preparePriceAttempt(t *testing.T, store *ConfirmedReturnStore, id string, request inventory.DHMutationRequest) *inventory.DHMutationAttempt {
	t.Helper()
	var a *inventory.DHMutationAttempt
	require.NoError(t, store.WithPurchaseMutation(context.Background(), id, func(ctx context.Context) error {
		var err error
		a, err = store.PrepareDHMutation(ctx, id, request)
		return err
	}))
	return a
}

func TestDHLivePriceKeyedReplay(t *testing.T) {
	for _, status := range []string{"listed", "in_stock", "sold"} {
		t.Run(status, func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "live-cert", 364577, "")
			ctx := context.Background()
			request := livePriceRequest()
			first := preparePriceAttempt(t, store, id, request)
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status=$1,reviewed_price_cents=30000 WHERE id=$2`, status, id)
			require.NoError(t, err)
			fresh := NewConfirmedReturnStore(db.DB)
			second := preparePriceAttempt(t, fresh, id, request)
			require.Equal(t, first.ID, second.ID)
			require.Equal(t, request.PayloadIdentity, second.PayloadIdentity)
			require.True(t, second.Replaying)
			require.NoError(t, fresh.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
				if err := fresh.OwnDHMutation(owned, id, second); err != nil {
					return err
				}
				return fresh.SettleDHMutation(owned, id, second, inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: "exact-price-receipt"})
			}))
			state, err := fresh.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Nil(t, state.PrecedingAttempt)
			require.Equal(t, status, state.Purchase.DHStatus)
			require.Equal(t, 999, state.Purchase.DHListingPriceCents)
		})
	}
}

func TestDHLivePriceErrorContext(t *testing.T) {
	cause := errors.New("rate limited")
	deadline := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for _, err := range []error{
		&inventory.DHLivePriceRejection{Code: "stale_listing", Cause: cause},
		&inventory.DHMutationRetryError{NotBefore: deadline, Cause: cause},
	} {
		require.ErrorIs(t, err, cause)
		require.Contains(t, err.Error(), cause.Error())
	}
}

func TestDHLivePriceCooldownDurableAndMonotonic(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "live-cooldown-cert", 364577, "")
	ctx := context.Background()
	a := preparePriceAttempt(t, store, id, livePriceRequest())
	retryStore, ok := any(store).(inventory.DHMutationRetryRepository)
	require.True(t, ok, "store must support durable cooldown")
	deadline := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	for _, next := range []time.Time{deadline, deadline.Add(-time.Minute), deadline.Add(time.Minute)} {
		require.NoError(t, store.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
			if err := store.OwnDHMutation(owned, id, a); err != nil {
				return err
			}
			return retryStore.DeferDHMutation(owned, id, a, next)
		}))
	}
	fresh := NewConfirmedReturnStore(db.DB)
	state, err := fresh.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.NotNil(t, state.PrecedingAttempt.RetryNotBefore)
	require.True(t, deadline.Add(time.Minute).Equal(*state.PrecedingAttempt.RetryNotBefore))
	var persisted *time.Time
	var settled sql.NullTime
	var receipt, outcome string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT retry_not_before,settled_at,receipt,outcome FROM dh_mutation_attempts WHERE id=$1`, a.ID).Scan(&persisted, &settled, &receipt, &outcome))
	require.NotNil(t, persisted)
	require.True(t, deadline.Add(time.Minute).Equal(*persisted))
	require.False(t, settled.Valid)
	require.Empty(t, receipt)
	require.Equal(t, "open", outcome)
	// A rolled-back extension must not leak past transaction ownership.
	failure := errors.New("rollback cooldown")
	require.ErrorIs(t, fresh.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		if err := fresh.OwnDHMutation(owned, id, a); err != nil {
			return err
		}
		if err := any(fresh).(inventory.DHMutationRetryRepository).DeferDHMutation(owned, id, a, deadline.Add(2*time.Hour)); err != nil {
			return err
		}
		return failure
	}), failure)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT retry_not_before FROM dh_mutation_attempts WHERE id=$1`, a.ID).Scan(&persisted))
	require.True(t, deadline.Add(time.Minute).Equal(*persisted))
}

func TestDHLivePricePendingDiscovery(t *testing.T) {
	tests := []struct {
		name, kind, phase, key, outcome, status string
		attached, pending                       bool
	}{
		{"listed keyed", "price", "live_price", "key", "open", "listed", true, true},
		{"no drift in stock", "price", "live_price", "key", "open", "in_stock", true, true},
		{"locally sold", "price", "live_price", "key", "open", "sold", true, true},
		{"legacy patch", "price", "patch", "key", "open", "listed", true, false},
		{"unkeyed live", "price", "live_price", "", "open", "listed", true, false},
		{"different kind", "sale", "live_price", "key", "open", "listed", true, false},
		{"terminal", "price", "live_price", "key", "succeeded", "listed", true, false},
		{"detached retained", "price", "live_price", "key", "open", "listed", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, _, _, id := setupReturnCore(t, "live-pending-cert", 364577, "")
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status=$1,dh_listing_price_cents=reviewed_price_cents WHERE id=$2`, tt.status, id)
			require.NoError(t, err)
			var purchase any
			if tt.attached {
				purchase = id
			}
			_, err = db.ExecContext(ctx, `INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,idempotency_key,payload_identity,outcome,settled_at,retry_not_before) VALUES('pending', $1,$2,364577,'live-pending-cert','PSA',$3,$4,$5,'exact',$6,CASE WHEN $6='open' THEN NULL ELSE now() END,now()+interval '1 hour')`, purchase, id, tt.kind, tt.phase, tt.key, tt.outcome)
			require.NoError(t, err)
			reader, ok := any(NewPurchaseStore(db.DB, nil)).(interface {
				ListPendingDHLivePrices(context.Context) ([]string, error)
			})
			require.True(t, ok, "purchase store must discover preceding live prices independently of drift")
			ids, err := reader.ListPendingDHLivePrices(ctx)
			require.NoError(t, err)
			if tt.pending {
				require.Equal(t, []string{id}, ids)
			} else {
				require.Empty(t, ids)
			}
		})
	}
}

func TestDHLivePriceCoordinatorPersistsCooldownAfterRollback(t *testing.T) {
	for _, replay := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "replay"}[replay], func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "live-coordinator-cert", 364577, "")
			if replay {
				preparePriceAttempt(t, store, id, livePriceRequest())
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deadline := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
			cause := errors.New("provider 429")
			failure := &inventory.DHMutationRetryError{NotBefore: deadline, Cause: errors.Join(cause, context.Canceled)}
			coord := inventory.NewDHMutationCoordinator(store, store)
			err := coord.Run(ctx, id, func(context.Context) (inventory.DHMutationRequest, error) { return livePriceRequest(), nil }, func(owned context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				require.Equal(t, replay, a.Replaying)
				_, err := executor(owned, db.DB).ExecContext(owned, `UPDATE campaign_purchases SET dh_listing_price_cents=777 WHERE id=$1`, id)
				require.NoError(t, err)
				cancel()
				return nil, failure
			})
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, err, failure)
			state, err := NewConfirmedReturnStore(db.DB).GetReturnState(context.Background(), id)
			require.NoError(t, err)
			require.Equal(t, 999, state.Purchase.DHListingPriceCents)
			require.NotNil(t, state.PrecedingAttempt)
			require.NotNil(t, state.PrecedingAttempt.RetryNotBefore, "execution rollback must be followed by detached cooldown persistence")
			require.True(t, deadline.Equal(*state.PrecedingAttempt.RetryNotBefore))
		})
	}
}

func TestDHLivePriceCoordinatorCooldownFailureIsJoined(t *testing.T) {
	for _, phase := range []string{"update", "commit", "ownership"} {
		t.Run(phase, func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "live-cooldown-failure", 364577, "")
			ctx := context.Background()
			if phase != "ownership" {
				trigger := `CREATE TRIGGER live_cooldown_failure BEFORE UPDATE ON dh_mutation_attempts FOR EACH ROW EXECUTE FUNCTION live_cooldown_failure()`
				if phase == "commit" {
					trigger = `CREATE CONSTRAINT TRIGGER live_cooldown_failure AFTER UPDATE ON dh_mutation_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION live_cooldown_failure()`
				}
				_, err := db.ExecContext(ctx, `CREATE FUNCTION live_cooldown_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.retry_not_before IS NOT NULL THEN RAISE EXCEPTION 'cooldown persistence failed'; END IF; RETURN NEW; END $$; `+trigger)
				require.NoError(t, err)
				t.Cleanup(func() {
					_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS live_cooldown_failure ON dh_mutation_attempts; DROP FUNCTION IF EXISTS live_cooldown_failure()`)
				})
			}
			cause := errors.New("provider rate limit")
			failure := &inventory.DHMutationRetryError{NotBefore: time.Now().Add(time.Hour), Cause: cause}
			err := inventory.NewDHMutationCoordinator(store, store).Run(ctx, id, func(context.Context) (inventory.DHMutationRequest, error) { return livePriceRequest(), nil }, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				if phase == "ownership" {
					_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=42 WHERE id=$1`, id)
					require.NoError(t, err)
				}
				return nil, failure
			})
			require.ErrorIs(t, err, cause)
			if phase == "ownership" {
				require.ErrorIs(t, err, inventory.ErrReturnConflict)
			} else {
				require.Contains(t, err.Error(), "cooldown persistence failed")
			}
			var deadline *time.Time
			var outcome, receipt string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT retry_not_before,outcome,receipt FROM dh_mutation_attempts WHERE captured_purchase_id=$1`, id).Scan(&deadline, &outcome, &receipt))
			require.Nil(t, deadline)
			require.Equal(t, "open", outcome)
			require.Empty(t, receipt)
		})
	}
}

func TestDHLivePriceOwnershipRequiresExactPhase(t *testing.T) {
	_, store, _, _, id := setupReturnCore(t, "live-phase-cert", 364577, "")
	a := preparePriceAttempt(t, store, id, livePriceRequest())
	a.Phase = "patch"
	err := store.WithPurchaseMutation(context.Background(), id, func(ctx context.Context) error { return store.OwnDHMutation(ctx, id, a) })
	require.ErrorIs(t, err, inventory.ErrReturnConflict)
}
