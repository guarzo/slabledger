package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func setupFailedListingDelete(t *testing.T) (*DB, *ConfirmedReturnStore, string) {
	t.Helper()
	db, store, _, _, id := setupReturnCore(t, "delete-listing", 42, "")
	coordinator := inventory.NewDHMutationCoordinator(store, store)
	err := coordinator.Run(context.Background(), id, func(context.Context) (inventory.DHMutationRequest, error) {
		return inventory.DHMutationRequest{Kind: "list", Phase: "intake_patch_sync", PayloadIdentity: `{"price":123}`}, nil
	}, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		return nil, &inventory.DHListingPatchRetryError{Cause: errors.New("DH PATCH failed")}
	})
	require.Error(t, err)
	var retry bool
	require.NoError(t, db.QueryRow(`SELECT listing_retryable FROM dh_mutation_attempts WHERE captured_purchase_id=$1`, id).Scan(&retry))
	require.True(t, retry, "drive the real failure bookkeeping, not a synthetic failed marker")
	return db, store, id
}

func TestFailedListingDeleteConfirmed(t *testing.T) {
	db, store, id := setupFailedListingDelete(t)
	ctx := inventory.WithFailedListingDeleteConfirmation(context.Background())
	// Current inputs may differ: abandonment never rebuilds the captured request.
	_, err := db.Exec(`UPDATE campaign_purchases SET reviewed_price_cents=777,front_image_url='changed' WHERE id=$1`, id)
	require.NoError(t, err)
	require.NoError(t, NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(ctx, id))
	var outcome, receipt, payload string
	var retry bool
	var linked *string
	require.NoError(t, db.QueryRow(`SELECT outcome,receipt,payload_identity,listing_retryable,purchase_id FROM dh_mutation_attempts WHERE captured_purchase_id=$1`, id).Scan(&outcome, &receipt, &payload, &retry, &linked))
	require.Equal(t, "abandoned", outcome)
	require.Contains(t, receipt, "unknown")
	require.False(t, retry)
	require.Nil(t, linked)
	require.Equal(t, `{"price":123}`, payload)
	_, err = store.GetReturnState(context.Background(), id)
	require.ErrorIs(t, err, inventory.ErrPurchaseNotFound)
}

func TestFailedListingDeleteBlockers(t *testing.T) {
	for _, tt := range []struct{ name, sql string }{
		{"prepared or running", `UPDATE dh_mutation_attempts SET listing_retryable=false`},
		{"other kind", `UPDATE dh_mutation_attempts SET listing_retryable=false,kind='void',phase='recover_void_reset_delete'`},
		{"sync failed not PATCH", `UPDATE dh_mutation_attempts SET listing_retryable=false,phase='sync'`},
		{"receipt", `UPDATE dh_mutation_attempts SET receipt='retained financial evidence'`},
		{"old captured identity", `UPDATE dh_mutation_attempts SET cert_number='old-cert'`},
		{"another purchase", `UPDATE dh_mutation_attempts SET captured_purchase_id='old-purchase'`},
		{"sale conflict", `UPDATE campaign_purchases SET dh_sale_conflict='unresolved'`},
		{"remote sold", `UPDATE campaign_purchases SET dh_status='sold'`},
		{"sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,created_at,updated_at) VALUES('blocked-sale','return-p','local',100,'2026-01-03',now(),now())`},
		{"return", `INSERT INTO confirmed_dh_returns(id,idempotency_key,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,state) VALUES('blocked-return','return-key','return-p','return-p',42,'delete-listing','PSA','pending')`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, id := setupFailedListingDelete(t)
			preview, err := store.GetReturnState(context.Background(), id)
			require.NoError(t, err)
			require.True(t, preview.FailedListingDeleteAllowed)
			_, err = db.Exec(tt.sql)
			require.NoError(t, err)
			state, err := store.GetReturnState(context.Background(), id)
			require.NoError(t, err)
			require.False(t, state.FailedListingDeleteAllowed)
			err = NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id)
			require.ErrorIs(t, err, inventory.ErrReturnConflict)
			var outcome string
			require.NoError(t, db.QueryRow(`SELECT outcome FROM dh_mutation_attempts WHERE captured_purchase_id=$1 OR purchase_id=$1`, id).Scan(&outcome))
			require.Equal(t, "open", outcome)
		})
	}
}

func TestFailedListingDeleteChecksEveryMatchingAttempt(t *testing.T) {
	db, store, id := setupFailedListingDelete(t)
	// Different historical captures can each match one part of today's identity.
	_, err := db.Exec(`UPDATE dh_mutation_attempts SET cert_number='old-cert';
 INSERT INTO dh_mutation_attempts(id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('other-attempt','another-purchase',99,'delete-listing','PSA','sale','record','financial')`)
	require.NoError(t, err)
	state, err := store.GetReturnState(context.Background(), id)
	require.NoError(t, err)
	require.False(t, state.FailedListingDeleteAllowed)
	require.ErrorIs(t, NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id), inventory.ErrReturnConflict)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM dh_mutation_attempts WHERE outcome='open'`).Scan(&count))
	require.Equal(t, 2, count)
}

func TestFailedListingDeleteRollback(t *testing.T) {
	db, _, id := setupFailedListingDelete(t)
	_, err := db.Exec(`CREATE FUNCTION fail_listing_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected delete failure'; END $$;
 CREATE TRIGGER fail_listing_delete BEFORE DELETE ON campaign_purchases FOR EACH ROW EXECUTE FUNCTION fail_listing_delete()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, e := db.Exec(`DROP TRIGGER fail_listing_delete ON campaign_purchases; DROP FUNCTION fail_listing_delete()`)
		require.NoError(t, e)
	})
	require.Error(t, NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id))
	var outcome, receipt string
	var retry bool
	require.NoError(t, db.QueryRow(`SELECT outcome,receipt,listing_retryable FROM dh_mutation_attempts WHERE captured_purchase_id=$1`, id).Scan(&outcome, &receipt, &retry))
	require.Equal(t, "open", outcome)
	require.Empty(t, receipt)
	require.True(t, retry)
}

func TestFailedListingDeleteNeedsWarning(t *testing.T) {
	db, store, id := setupFailedListingDelete(t)
	state, err := store.GetReturnState(context.Background(), id)
	require.NoError(t, err)
	data, err := json.Marshal(state)
	require.NoError(t, err)
	var public map[string]any
	require.NoError(t, json.Unmarshal(data, &public))
	require.Equal(t, true, public["failedListingDeleteAllowed"])
	err = NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(context.Background(), id)
	var conflict *inventory.ReturnConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "failed_listing_delete_confirmation_required", conflict.Code)
	var outcome string
	require.NoError(t, db.QueryRow(`SELECT outcome FROM dh_mutation_attempts WHERE captured_purchase_id=$1`, id).Scan(&outcome))
	require.Equal(t, "open", outcome)
}
