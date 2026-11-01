package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func recoveryFixture(t *testing.T, action string) (*DB, *ConfirmedReturnStore, string) {
	t.Helper()
	db, store, _, _, id := setupReturnCore(t, "recovery-cert", 42, "")
	var query string
	if action == "relink-returned" {
		query = `UPDATE campaign_purchases SET dh_inventory_id=0,dh_status='',dh_card_id=0 WHERE id='return-p';
 INSERT INTO confirmed_dh_returns(id,idempotency_key,captured_purchase_id,dh_inventory_id,cert_number,grader,state,completed_at,listing_authorized_at,returned_order_id)
 VALUES('history','return-key','deleted-purchase',42,'recovery-cert','PSA','completed',now(),now(),'ext-old')`
	} else {
		query = `UPDATE campaign_purchases SET dh_sale_conflict='old void failed',dh_sale_conflict_at=now() WHERE id='return-p';
 INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity)
 VALUES('void-attempt','return-p','return-p',42,'recovery-cert','PSA','void','recover_void_reset_delete','frozen-request')`
	}
	_, err := db.Exec(query)
	require.NoError(t, err)
	return db, store, id
}

func recoverySnapshot(t *testing.T, db *DB) string {
	t.Helper()
	var snapshot string
	require.NoError(t, db.QueryRow(`SELECT jsonb_build_array(
 (SELECT jsonb_agg(p) FROM campaign_purchases p),
 (SELECT jsonb_agg(a) FROM dh_mutation_attempts a),
 (SELECT jsonb_agg(e) FROM confirmed_dh_returns e),
 (SELECT jsonb_agg(w) FROM dh_target_watermarks w),
 (SELECT count(*) FROM dh_state_events))::text`).Scan(&snapshot))
	return snapshot
}

func TestDHInventoryRecovery(t *testing.T) {
	for _, action := range []string{"relink-returned", "release-void"} {
		t.Run(action, func(t *testing.T) {
			db, store, id := recoveryFixture(t, action)
			before, err := store.GetReturnState(context.Background(), id)
			require.NoError(t, err)
			fetchedAt, err := store.ObservationTime(context.Background())
			require.NoError(t, err)
			require.NoError(t, store.RecoverDHInventory(context.Background(), id, action, "admin-1"))
			after, err := store.GetReturnState(context.Background(), id)
			require.NoError(t, err)
			require.Equal(t, 42, after.Purchase.DHInventoryID)
			require.Equal(t, "matched", after.Purchase.DHPushStatus)
			require.Zero(t, after.Purchase.DHPushAttempts)
			require.Empty(t, after.Purchase.DHSaleConflict)
			require.Nil(t, after.Purchase.DHSaleConflictAt)
			require.Equal(t, before.Purchase.ReviewedPriceCents, after.Purchase.ReviewedPriceCents)
			require.Equal(t, before.Purchase.DHCardID, after.Purchase.DHCardID)
			if action == "relink-returned" {
				require.Equal(t, inventory.DHStatusListed, after.Purchase.DHStatus)
				require.Equal(t, before.Operation, after.Operation, "history must not be rewritten")
				sale := &inventory.Sale{ID: "returned", PurchaseID: id, OrderID: "ext-old", SaleChannel: inventory.SaleChannelLocal, SaleDate: "2026-01-04"}
				require.ErrorIs(t, NewSaleStore(db.DB, mocks.NewMockLogger()).CreateSale(context.Background(), sale), inventory.ErrReturnConflict)
			} else {
				require.Equal(t, inventory.DHStatusInStock, after.Purchase.DHStatus)
				var outcome, payload, receipt string
				require.NoError(t, db.QueryRow(`SELECT outcome,payload_identity,receipt FROM dh_mutation_attempts WHERE id='void-attempt'`).Scan(&outcome, &payload, &receipt))
				require.Equal(t, "abandoned", outcome)
				require.Equal(t, "frozen-request", payload)
				require.Contains(t, receipt, `"original_provider_outcome":"unknown"`)
			}
			var actor string
			require.NoError(t, db.QueryRow(`SELECT notes::jsonb->>'actor' FROM dh_state_events WHERE purchase_id=$1 AND event_type='inventory_recovered'`, id).Scan(&actor))
			require.Equal(t, "admin-1", actor)
			applied, err := store.ApplyDHObservation(context.Background(), id, 42, fetchedAt, func(context.Context) error { t.Fatal("stale observation applied"); return nil })
			require.NoError(t, err)
			require.False(t, applied)
			require.NoError(t, store.WithPurchaseMutation(context.Background(), id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }))
			sale := &inventory.Sale{ID: "new-sale", PurchaseID: id, OrderID: "ext-new", SaleChannel: inventory.SaleChannelLocal, SaleDate: "2026-01-04"}
			require.NoError(t, NewSaleStore(db.DB, mocks.NewMockLogger()).CreateSale(context.Background(), sale))
			require.ErrorIs(t, store.RecoverDHInventory(context.Background(), id, action, "admin-1"), inventory.ErrReturnConflict)
		})
	}
}

func TestDHInventoryRecoveryBlockers(t *testing.T) {
	for _, tt := range []struct{ action, name, query string }{
		{"relink-returned", "linked", `UPDATE campaign_purchases SET dh_inventory_id=42`},
		{"relink-returned", "sold", `UPDATE campaign_purchases SET dh_status='sold'`},
		{"release-void", "sold", `UPDATE campaign_purchases SET dh_status='sold'`},
		{"release-void", "missing", `UPDATE campaign_purchases SET dh_inventory_id=0`},
		{"release-void", "other status", `UPDATE campaign_purchases SET dh_status='listed'`},
		{"relink-returned", "unreceived", `UPDATE campaign_purchases SET received_at=NULL`},
		{"release-void", "sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale','return-p','local','2026-01-04')`},
		{"relink-returned", "sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale','return-p','local','2026-01-04')`},
		{"relink-returned", "unauthorized", `UPDATE confirmed_dh_returns SET listing_authorized_at=NULL`},
		{"relink-returned", "pending", `UPDATE confirmed_dh_returns SET state='pending',completed_at=NULL,listing_authorized_at=NULL`},
		{"relink-returned", "conflicted", `UPDATE confirmed_dh_returns SET state='conflicted',completed_at=NULL,listing_authorized_at=NULL`},
		{"relink-returned", "attached", `UPDATE confirmed_dh_returns SET purchase_id='return-p'`},
		{"relink-returned", "foreign history", `UPDATE confirmed_dh_returns SET cert_number='foreign'`},
		{"relink-returned", "conflict flag", `UPDATE campaign_purchases SET dh_sale_conflict='conflict'`},
		{"release-void", "receipt", `UPDATE dh_mutation_attempts SET receipt='financial evidence'`},
		{"release-void", "keyed", `UPDATE dh_mutation_attempts SET idempotency_key='sale-key'`},
		{"release-void", "sale attempt", `UPDATE dh_mutation_attempts SET kind='sale'`},
		{"release-void", "foreign attempt", `UPDATE dh_mutation_attempts SET captured_purchase_id='foreign'`},
		{"release-void", "return", `INSERT INTO confirmed_dh_returns(id,idempotency_key,captured_purchase_id,dh_inventory_id,cert_number,grader) VALUES('pending','pending-key','return-p',42,'recovery-cert','PSA')`},
		{"relink-returned", "attempt", `INSERT INTO dh_mutation_attempts(id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('open','return-p',42,'recovery-cert','PSA','price','patch','frozen')`},
		{"release-void", "multiple matches", `UPDATE dh_mutation_attempts SET cert_number='old-cert'; INSERT INTO dh_mutation_attempts(id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('other','foreign',99,'recovery-cert','PSA','sale','record','frozen')`},
		{"relink-returned", "destination owner", `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,dh_inventory_id,purchase_date) VALUES('owner','return-c','Card','foreign','PSA',42,'2026-01-01')`},
	} {
		t.Run(tt.action+"/"+tt.name, func(t *testing.T) {
			db, store, id := recoveryFixture(t, tt.action)
			_, err := db.Exec(tt.query)
			require.NoError(t, err)
			before := recoverySnapshot(t, db)
			require.ErrorIs(t, store.RecoverDHInventory(context.Background(), id, tt.action, "admin-1"), inventory.ErrReturnConflict)
			require.Equal(t, before, recoverySnapshot(t, db))
		})
	}
}

func TestDHInventoryRecoveryRollback(t *testing.T) {
	for _, action := range []string{"relink-returned", "release-void"} {
		t.Run(action, func(t *testing.T) {
			db, store, id := recoveryFixture(t, action)
			_, err := db.Exec(`CREATE FUNCTION fail_recovery() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER fail_recovery BEFORE INSERT ON dh_state_events FOR EACH ROW EXECUTE FUNCTION fail_recovery()`)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, e := db.Exec(`DROP TRIGGER fail_recovery ON dh_state_events; DROP FUNCTION fail_recovery()`)
				require.NoError(t, e)
			})
			before := recoverySnapshot(t, db)
			require.Error(t, store.RecoverDHInventory(context.Background(), id, action, "admin-1"))
			require.Equal(t, before, recoverySnapshot(t, db))
		})
	}
}

func TestDHInventoryRecoveryDestinationRace(t *testing.T) {
	db, store, id := recoveryFixture(t, "relink-returned")
	_, err := db.Exec(`INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,purchase_date) VALUES('owner','return-c','Card','foreign','PSA','2026-01-01')`)
	require.NoError(t, err)
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`UPDATE campaign_purchases SET dh_inventory_id=42 WHERE id='owner'`)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- store.RecoverDHInventory(ctx, id, "relink-returned", "admin-1") }()
	select {
	case err := <-done:
		t.Fatalf("recovery bypassed uncommitted target owner: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	require.NoError(t, tx.Commit())
	require.ErrorIs(t, <-done, inventory.ErrReturnConflict)
}
