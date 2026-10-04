package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnTimeoutSameAttemptReplay(t *testing.T) {
	_, store, svc, fake, id := setupReturnCore(t, "timeout-replay-cert", 364577, "")
	ctx := context.Background()
	calls, preflights := 0, 0
	key := ""
	fake.GetReturnInventoryStatusFn = func(context.Context, int, string) (string, error) { preflights++; return "sold", nil }
	fake.ReturnInventoryToStockFn = func(_ context.Context, _ int, k string) (*inventory.DHReturnResult, error) {
		calls++
		if key == "" {
			key = k
		} else {
			require.Equal(t, key, k)
		}
		if calls == 1 {
			return nil, context.DeadlineExceeded
		}
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	req := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
	pending, err := svc.ConfirmReturn(ctx, id, req)
	require.Error(t, err)
	require.Equal(t, "pending", pending.Operation.State)
	require.NotNil(t, pending.PrecedingAttempt)
	req.OperationID = pending.Operation.ID
	completed, err := svc.ConfirmReturn(ctx, id, req)
	require.NoError(t, err)
	require.Equal(t, pending.Operation.ID, completed.Operation.ID)
	require.Equal(t, 1, preflights)
	require.Equal(t, 2, calls)
	var outcome string
	require.NoError(t, store.db.QueryRowContext(ctx, `SELECT outcome FROM dh_mutation_attempts WHERE id=$1`, pending.PrecedingAttempt.ID).Scan(&outcome))
	require.Equal(t, "succeeded", outcome)
}

func TestConfirmedReturnObservedAttributionCannotChange(t *testing.T) {
	db, _, svc, fake, id := setupReturnCore(t, "observed-attribution", 364577, "")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `CREATE FUNCTION fail_observed_completion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='completed' THEN RAISE EXCEPTION 'completion unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_observed_completion BEFORE UPDATE ON confirmed_dh_returns FOR EACH ROW EXECUTE FUNCTION fail_observed_completion()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_observed_completion ON confirmed_dh_returns; DROP FUNCTION IF EXISTS fail_observed_completion()`)
	})
	external := int64(848)
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: external}, nil
	}
	req := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
	pending, err := svc.ConfirmReturn(ctx, id, req)
	require.Error(t, err)
	require.Equal(t, int64(848), pending.Operation.ObservedReceipt.ExternalSaleID)
	_, err = db.ExecContext(ctx, `DROP TRIGGER fail_observed_completion ON confirmed_dh_returns; DROP FUNCTION fail_observed_completion()`)
	require.NoError(t, err)
	external = 999
	state, err := svc.ConfirmReturn(ctx, id, req)
	require.ErrorIs(t, err, inventory.ErrReturnConflict)
	require.Equal(t, "conflicted", state.Operation.State)
	require.Equal(t, int64(848), state.Operation.ObservedReceipt.ExternalSaleID)
}
