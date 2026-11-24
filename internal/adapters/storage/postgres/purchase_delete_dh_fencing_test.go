package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Production cards that never reached DH: purchase has dh_inventory_id 0.
func setupUnpushedPurchase(t *testing.T) (*DB, *ConfirmedReturnStore, string) {
	t.Helper()
	db, store, _, _, id := setupReturnCore(t, "173686392", 0, "")
	return db, store, id
}

func insertOpenAttempt(t *testing.T, db *DB, kind, phase string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('open-attempt','return-p','return-p',0,'173686392','PSA',$1,$2,'payload')`, kind, phase)
	require.NoError(t, err)
}

func completedMismatchedReturn(t *testing.T, db *DB, state string) {
	t.Helper()
	completed := "NULL"
	if state == "completed" {
		completed = "now()"
	}
	_, err := db.Exec(`INSERT INTO confirmed_dh_returns(id,idempotency_key,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,state,completed_at) VALUES('old-return','old-key','return-p','return-p',555,'173686392','PSA','` + state + `',` + completed + `)`)
	require.NoError(t, err)
}

func recordSale(db *DB, id string) error {
	return NewSaleStore(db.DB, mocks.NewMockLogger()).CreateSale(context.Background(), &inventory.Sale{ID: "s-" + id, PurchaseID: id, SaleChannel: inventory.SaleChannelLocal, SalePriceCents: 30600, SaleDate: "2026-11-20", CreatedAt: time.Now(), UpdatedAt: time.Now()})
}

func deleteCode(t *testing.T, err error) string {
	t.Helper()
	var c *inventory.ReturnConflict
	require.ErrorAs(t, err, &c)
	return c.Code
}

func TestOpenPushAttemptDoesNotBlockLocalSale(t *testing.T) {
	db, _, id := setupUnpushedPurchase(t)
	insertOpenAttempt(t, db, "push", "psa_import")
	require.NoError(t, recordSale(db, id))
	var outcome string
	require.NoError(t, db.QueryRow(`SELECT outcome FROM dh_mutation_attempts WHERE id='open-attempt'`).Scan(&outcome))
	require.Equal(t, "open", outcome, "sale must leave DH reconciliation to the fenced paths")
}

func TestOpenPushAttemptWarnedDelete(t *testing.T) {
	db, store, id := setupUnpushedPurchase(t)
	insertOpenAttempt(t, db, "push", "psa_import")
	state, err := store.GetReturnState(context.Background(), id)
	require.NoError(t, err)
	require.True(t, state.FailedListingDeleteAllowed)
	ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	err = ps.DeletePurchase(context.Background(), id)
	require.Equal(t, "failed_listing_delete_confirmation_required", deleteCode(t, err))
	require.Contains(t, err.Error(), "uncertain")
	require.NoError(t, ps.DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id))
	var outcome, receipt string
	require.NoError(t, db.QueryRow(`SELECT outcome,receipt FROM dh_mutation_attempts WHERE id='open-attempt'`).Scan(&outcome, &receipt))
	require.Equal(t, "abandoned", outcome)
	require.Contains(t, receipt, "unknown")
}

func TestCompletedMismatchedReturnAllowsSaleAndWarnedDelete(t *testing.T) {
	t.Run("sale", func(t *testing.T) {
		db, _, id := setupUnpushedPurchase(t)
		completedMismatchedReturn(t, db, "completed")
		require.NoError(t, recordSale(db, id))
	})
	t.Run("delete", func(t *testing.T) {
		db, store, id := setupUnpushedPurchase(t)
		completedMismatchedReturn(t, db, "completed")
		state, err := store.GetReturnState(context.Background(), id)
		require.NoError(t, err)
		require.True(t, state.FailedListingDeleteAllowed)
		ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
		require.Equal(t, "failed_listing_delete_confirmation_required", deleteCode(t, ps.DeletePurchase(context.Background(), id)))
		require.NoError(t, ps.DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id))
		var linked *string
		var n int
		require.NoError(t, db.QueryRow(`SELECT purchase_id FROM confirmed_dh_returns WHERE id='old-return'`).Scan(&linked))
		require.Nil(t, linked, "retained episode is kept, unlinked")
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM campaign_purchases WHERE id=$1`, id).Scan(&n))
		require.Zero(t, n)
	})
}

func TestLocalSaleAndDeleteStillBlocked(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, *DB)
		code  string
	}{
		{"open sale attempt with live local sale", func(t *testing.T, db *DB) {
			insertOpenAttempt(t, db, "sale", "record")
			_, err := db.Exec(`INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,created_at,updated_at) VALUES('live-sale','return-p','local',100,'2026-01-03',now(),now())`)
			require.NoError(t, err)
		}, "preceding_dh_mutation_uncertain"},
		{"open sale attempt with non-record phase", func(t *testing.T, db *DB) { insertOpenAttempt(t, db, "sale", "other") }, "preceding_dh_mutation_uncertain"},
		{"open void attempt", func(t *testing.T, db *DB) { insertOpenAttempt(t, db, "void", "recover_void_reset_delete") }, "preceding_dh_mutation_uncertain"},
		{"open return attempt", func(t *testing.T, db *DB) { insertOpenAttempt(t, db, "return", "return_to_stock") }, "preceding_dh_mutation_uncertain"},
		{"unknown kind fails closed", func(t *testing.T, db *DB) { insertOpenAttempt(t, db, "mystery", "x") }, "preceding_dh_mutation_uncertain"},
		{"pending mismatched return", func(t *testing.T, db *DB) { completedMismatchedReturn(t, db, "pending") }, "identity_conflict"},
		{"conflicted mismatched return", func(t *testing.T, db *DB) { completedMismatchedReturn(t, db, "conflicted") }, "identity_conflict"},
		{"financial attempt plus completed mismatched return", func(t *testing.T, db *DB) {
			insertOpenAttempt(t, db, "void", "recover_void_reset_delete")
			completedMismatchedReturn(t, db, "completed")
		}, "preceding_dh_mutation_uncertain"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, id := setupUnpushedPurchase(t)
			tt.setup(t, db)
			require.Equal(t, tt.code, deleteCode(t, recordSale(db, id)))
			state, err := store.GetReturnState(context.Background(), id)
			require.NoError(t, err)
			require.False(t, state.FailedListingDeleteAllowed)
			err = NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id)
			require.Equal(t, tt.code, deleteCode(t, err))
			var n int
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM campaign_purchases WHERE id=$1`, id).Scan(&n))
			require.Equal(t, 1, n)
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM dh_mutation_attempts WHERE outcome='abandoned'`).Scan(&n))
			require.Zero(t, n)
		})
	}
}

func TestDHWritePathStillFencedByOpenPushAttempt(t *testing.T) {
	db, store, id := setupUnpushedPurchase(t)
	insertOpenAttempt(t, db, "push", "psa_import")
	executed := false
	err := inventory.NewDHMutationCoordinator(store, store).Run(context.Background(), id, func(context.Context) (inventory.DHMutationRequest, error) {
		return inventory.DHMutationRequest{Kind: "price", Phase: "patch", PayloadIdentity: "p"}, nil
	}, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		executed = true
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: "r"}, nil
	})
	require.Equal(t, "preceding_dh_mutation_uncertain", deleteCode(t, err))
	require.False(t, executed)
	require.Equal(t, "preceding_dh_mutation_uncertain", deleteCode(t, store.WithPurchaseMutation(context.Background(), id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) })))
	// execDHMutation (DH column writes) keeps the full fence too.
	err = NewPurchaseStore(db.DB, mocks.NewMockLogger()).execDHMutation(context.Background(), id, "test", `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
	require.Equal(t, "preceding_dh_mutation_uncertain", deleteCode(t, err))
}

// Production Mew-Holo shape: on DH, open list/intake_patch_sync attempt that
// never settled as a retryable failure (prepared, running or crashed).
func TestOpenNonRetryableListAttemptOnDHAllowsSaleAndWarnedDelete(t *testing.T) {
	setup := func(t *testing.T) (*DB, *ConfirmedReturnStore, string) {
		db, store, _, _, id := setupReturnCore(t, "165164315", 526940, "")
		_, err := db.Exec(`INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity,listing_retryable) VALUES('mew','return-p','return-p',526940,'165164315','PSA','list','intake_patch_sync','payload',false)`)
		require.NoError(t, err)
		return db, store, id
	}
	t.Run("sale", func(t *testing.T) {
		db, _, id := setup(t)
		require.NoError(t, recordSale(db, id))
	})
	t.Run("delete", func(t *testing.T) {
		db, store, id := setup(t)
		state, err := store.GetReturnState(context.Background(), id)
		require.NoError(t, err)
		require.True(t, state.FailedListingDeleteAllowed)
		ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
		require.Equal(t, "failed_listing_delete_confirmation_required", deleteCode(t, ps.DeletePurchase(context.Background(), id)))
		require.NoError(t, ps.DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id))
		var outcome string
		require.NoError(t, db.QueryRow(`SELECT outcome FROM dh_mutation_attempts WHERE id='mew'`).Scan(&outcome))
		require.Equal(t, "abandoned", outcome)
	})
}

// Regigigas shape: un-sold after an uncertain DH sale record. The sale row is
// gone, the sale/record attempt stays open for diagnosis.
func TestOpenSaleAttemptAfterUnsellAllowsSaleAndWarnedDelete(t *testing.T) {
	setup := func(t *testing.T) (*DB, *ConfirmedReturnStore, string) {
		db, store, _, _, id := setupReturnCore(t, "69007166", 647476, "")
		_, err := db.Exec(`INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,idempotency_key,payload_identity) VALUES('regigigas','return-p','return-p',647476,'69007166','PSA','sale','record','key-1','payload')`)
		require.NoError(t, err)
		return db, store, id
	}
	t.Run("sale", func(t *testing.T) {
		db, _, id := setup(t)
		require.NoError(t, recordSale(db, id))
	})
	t.Run("delete", func(t *testing.T) {
		db, store, id := setup(t)
		state, err := store.GetReturnState(context.Background(), id)
		require.NoError(t, err)
		require.True(t, state.FailedListingDeleteAllowed)
		ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
		require.Equal(t, "failed_listing_delete_confirmation_required", deleteCode(t, ps.DeletePurchase(context.Background(), id)))
		require.NoError(t, ps.DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id))
		var outcome string
		require.NoError(t, db.QueryRow(`SELECT outcome FROM dh_mutation_attempts WHERE id='regigigas'`).Scan(&outcome))
		require.Equal(t, "abandoned", outcome)
	})
	t.Run("DH sale path still fenced", func(t *testing.T) {
		_, store, id := setup(t)
		err := inventory.NewDHMutationCoordinator(store, store).Run(context.Background(), id, func(context.Context) (inventory.DHMutationRequest, error) {
			return inventory.DHMutationRequest{Kind: "sale", Phase: "record", Key: "key-2", PayloadIdentity: "other"}, nil
		}, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
			t.Fatal("must not execute")
			return nil, nil
		})
		require.Equal(t, "preceding_dh_mutation_uncertain", deleteCode(t, err))
	})
}

// An attempt captured against an older DH inventory id is not this card's
// current DH state; warned delete must not silently abandon it.
func TestWarnedDeleteBlockedByAttemptOnOldDHInventoryID(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "165164315", 526940, "")
	_, err := db.Exec(`INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('stale','return-p','return-p',111,'165164315','PSA','list','intake_patch_sync','payload')`)
	require.NoError(t, err)
	state, err := store.GetReturnState(context.Background(), id)
	require.NoError(t, err)
	require.False(t, state.FailedListingDeleteAllowed)
	ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	require.Equal(t, "preceding_dh_mutation_uncertain", deleteCode(t, ps.DeletePurchase(inventory.WithFailedListingDeleteConfirmation(context.Background()), id)))
}
