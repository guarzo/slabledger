package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func setupLegacyPriceAbandonment(t *testing.T) (*DB, *ConfirmedReturnStore, string, LegacyDHPriceAbandonmentRequest) {
	t.Helper()
	db, store, _, _, id := setupReturnCore(t, "legacy-price-cert", 364577, "")
	// The original attempted price is deliberately NOT today's approved price.
	a := preparePriceAttempt(t, store, id, inventory.DHMutationRequest{Kind: "price", Phase: "patch", PayloadIdentity: `{"ID":364577,"Price":20000}`})
	return db, store, id, LegacyDHPriceAbandonmentRequest{
		ExpectedAttemptID: a.ID, DHInventoryID: 364577, CertNumber: "legacy-price-cert", Grader: "PSA",
		FrozenPayloadIdentity: `{"ID":364577,"Price":20000}`, ApprovedListingPriceCents: 25000,
		OperatorAuthorization: "operator approved this exact attempt", QuiescenceEvidence: "old mutators drained; original request no longer executing",
	}
}

// Removing any CAS, eligibility, evidence or ownership check must fail a case.
func TestAbandonLegacyDHPricePreconditions(t *testing.T) {
	tests := []struct {
		name   string
		change func(*LegacyDHPriceAbandonmentRequest)
		sql    string
		mode   string
	}{
		{name: "success"},
		{name: "wrong attempt", change: func(r *LegacyDHPriceAbandonmentRequest) { r.ExpectedAttemptID = "other" }},
		{name: "empty attempt", change: func(r *LegacyDHPriceAbandonmentRequest) { r.ExpectedAttemptID = "" }},
		{name: "wrong target", change: func(r *LegacyDHPriceAbandonmentRequest) { r.DHInventoryID++ }},
		{name: "zero target", change: func(r *LegacyDHPriceAbandonmentRequest) { r.DHInventoryID = 0 }},
		{name: "wrong cert", change: func(r *LegacyDHPriceAbandonmentRequest) { r.CertNumber = "other" }},
		{name: "empty cert", change: func(r *LegacyDHPriceAbandonmentRequest) { r.CertNumber = "" }},
		{name: "wrong grader", change: func(r *LegacyDHPriceAbandonmentRequest) { r.Grader = "CGC" }},
		{name: "empty grader", change: func(r *LegacyDHPriceAbandonmentRequest) { r.Grader = "" }},
		{name: "different frozen payload", change: func(r *LegacyDHPriceAbandonmentRequest) { r.FrozenPayloadIdentity = `{"ID":364577,"Price":25000}` }},
		{name: "equivalent JSON not exact", change: func(r *LegacyDHPriceAbandonmentRequest) { r.FrozenPayloadIdentity = `{"Price":20000,"ID":364577}` }},
		{name: "empty frozen payload", change: func(r *LegacyDHPriceAbandonmentRequest) { r.FrozenPayloadIdentity = "" }},
		{name: "wrong approval", change: func(r *LegacyDHPriceAbandonmentRequest) { r.ApprovedListingPriceCents = 20000 }},
		{name: "zero approval", change: func(r *LegacyDHPriceAbandonmentRequest) { r.ApprovedListingPriceCents = 0 }},
		{name: "negative approval", change: func(r *LegacyDHPriceAbandonmentRequest) { r.ApprovedListingPriceCents = -1 }},
		{name: "empty authorization", change: func(r *LegacyDHPriceAbandonmentRequest) { r.OperatorAuthorization = "" }},
		{name: "blank authorization", change: func(r *LegacyDHPriceAbandonmentRequest) { r.OperatorAuthorization = " \n " }},
		{name: "empty quiescence", change: func(r *LegacyDHPriceAbandonmentRequest) { r.QuiescenceEvidence = "" }},
		{name: "blank quiescence", change: func(r *LegacyDHPriceAbandonmentRequest) { r.QuiescenceEvidence = "\t" }},
		{name: "no ownership", mode: "unowned"},
		{name: "other purchase ownership", mode: "other purchase"},
		{name: "other database ownership", mode: "other database"},
		{name: "changed purchase target", sql: `UPDATE campaign_purchases SET dh_inventory_id=42`},
		{name: "changed purchase cert", sql: `UPDATE campaign_purchases SET cert_number='changed'`},
		{name: "changed purchase grader", sql: `UPDATE campaign_purchases SET grader='CGC'`},
		{name: "changed captured purchase", sql: `UPDATE dh_mutation_attempts SET captured_purchase_id='other'`},
		{name: "changed attempt target", sql: `UPDATE dh_mutation_attempts SET dh_inventory_id=42`},
		{name: "changed attempt cert", sql: `UPDATE dh_mutation_attempts SET cert_number='changed'`},
		{name: "changed attempt grader", sql: `UPDATE dh_mutation_attempts SET grader='CGC'`},
		{name: "keyed", sql: `UPDATE dh_mutation_attempts SET idempotency_key='key'`},
		{name: "retained provider evidence", sql: `UPDATE dh_mutation_attempts SET receipt='original provider evidence; outcome still unknown'`},
		{name: "wrong kind", sql: `UPDATE dh_mutation_attempts SET kind='listing'`},
		{name: "wrong phase", sql: `UPDATE dh_mutation_attempts SET phase='live_price'`},
		{name: "succeeded", sql: `UPDATE dh_mutation_attempts SET outcome='succeeded',settled_at=now()`},
		{name: "rejected", sql: `UPDATE dh_mutation_attempts SET outcome='rejected',settled_at=now()`},
		{name: "already abandoned", sql: `UPDATE dh_mutation_attempts SET outcome='abandoned',settled_at=now()`},
		{name: "missing attempt", sql: `DELETE FROM dh_mutation_attempts`},
		{name: "not received", sql: `UPDATE campaign_purchases SET received_at=NULL`},
		{name: "listed", sql: `UPDATE campaign_purchases SET dh_status='listed'`},
		{name: "sold status", sql: `UPDATE campaign_purchases SET dh_status='sold'`},
		{name: "unknown status", sql: `UPDATE campaign_purchases SET dh_status='unknown'`},
		{name: "no committed price", sql: `UPDATE campaign_purchases SET reviewed_price_cents=0`},
		{name: "changed canonical override", sql: `UPDATE campaign_purchases SET override_price_cents=30000,override_set_at='2026-01-03T00:00:00Z'`},
		{name: "sale conflict", sql: `UPDATE campaign_purchases SET dh_sale_conflict='unresolved'`},
		{name: "sale conflict timestamp", sql: `UPDATE campaign_purchases SET dh_sale_conflict_at=now()`},
		{name: "current sale", sql: `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,created_at,updated_at) VALUES('current','return-p','local','2026-01-03',now(),now())`},
		{name: "pending return", mode: "pending"},
		{name: "conflicted return", mode: "conflicted"},
		{name: "awaiting listing", mode: "completed"},
		{name: "authorized completed return", mode: "authorized"},
		{name: "retained mismatched return", mode: "retained"},
		{name: "attempt return operation", mode: "return attempt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, store, id, req := setupLegacyPriceAbandonment(t)
			ctx := context.Background()
			if tt.change != nil {
				tt.change(&req)
			}
			if tt.sql != "" {
				_, err := db.ExecContext(ctx, tt.sql)
				require.NoError(t, err)
			}
			if tt.mode == "pending" || tt.mode == "conflicted" || tt.mode == "completed" || tt.mode == "authorized" || tt.mode == "retained" || tt.mode == "return attempt" {
				state := tt.mode
				if state == "authorized" || state == "retained" {
					state = "completed"
				}
				if state == "return attempt" {
					state = "pending"
				}
				_, err := db.ExecContext(ctx, `INSERT INTO confirmed_dh_returns(id,idempotency_key,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,state,completed_at,listing_authorized_at) VALUES('episode','return-key',$1,$1,364577,'legacy-price-cert','PSA',$2,CASE WHEN $2='completed' THEN now() END,CASE WHEN $3 THEN now() END)`, id, state, tt.mode == "authorized")
				require.NoError(t, err)
				if tt.mode == "retained" {
					_, err = db.ExecContext(ctx, `UPDATE confirmed_dh_returns SET purchase_id=NULL,dh_inventory_id=42`)
					require.NoError(t, err)
				}
				if tt.mode == "return attempt" {
					// This episode is not current for the target, so the attempt's
					// return_operation_id guard must reject it independently.
					_, err = db.ExecContext(ctx, `UPDATE confirmed_dh_returns SET dh_inventory_id=42,cert_number='other-cert',grader='CGC'; UPDATE dh_mutation_attempts SET return_operation_id='episode'`)
					require.NoError(t, err)
				}
			}
			var before, after string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT COALESCE(json_agg(a ORDER BY id)::text,'[]') FROM dh_mutation_attempts a`).Scan(&before))
			run := func(c context.Context) error { return store.AbandonLegacyDHPrice(c, id, req) }
			var err error
			switch tt.mode {
			case "unowned":
				err = run(ctx)
			case "other purchase":
				err = store.WithPurchaseMutation(ctx, "other-purchase", run)
			case "other database":
				other := NewConfirmedReturnStore(requireTestDB(t).DB)
				err = other.WithPurchaseMutation(ctx, id, run)
			default:
				err = store.WithPurchaseMutation(ctx, id, run)
			}
			if tt.name == "success" {
				require.NoError(t, err)
				state, err := store.GetReturnState(ctx, id)
				require.NoError(t, err)
				require.Nil(t, state.PrecedingAttempt)
			} else {
				require.ErrorIs(t, err, inventory.ErrReturnConflict)
				require.NoError(t, db.QueryRowContext(ctx, `SELECT COALESCE(json_agg(a ORDER BY id)::text,'[]') FROM dh_mutation_attempts a`).Scan(&after))
				require.Equal(t, before, after, "refusal must not alter any journal history")
			}
		})
	}
}

func TestAbandonLegacyDHPriceAuditAndWatermark(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "journal only", true: "separate reset in same transaction"}[reset], func(t *testing.T) {
			db, store, id, req := setupLegacyPriceAbandonment(t)
			ctx := context.Background()
			purchases := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			before, err := purchases.GetPurchase(ctx, id)
			require.NoError(t, err)
			var original string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT (to_jsonb(a)-'outcome'-'settled_at'-'receipt')::text FROM dh_mutation_attempts a WHERE id=$1`, req.ExpectedAttemptID).Scan(&original))
			fetchedAt, err := store.ObservationTime(ctx)
			require.NoError(t, err)
			require.ErrorIs(t, purchases.ResetDHFieldsForRepushDueToDelete(ctx, id), inventory.ErrReturnConflict, "existing local writer fence remains strict")
			require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
				if err := store.AbandonLegacyDHPrice(c, id, req); err != nil {
					return err
				}
				// Abandonment acquires full execution ownership: new preparation
				// must wait for this transaction to finish, not nest inside it.
				_, err := store.PrepareDHMutation(c, id, inventory.DHMutationRequest{Kind: "listing", Phase: "patch_and_sync", PayloadIdentity: "new listing"})
				var conflict *inventory.ReturnConflict
				require.ErrorAs(t, err, &conflict)
				require.Equal(t, "nested_dh_preparation", conflict.Code)
				if reset {
					return purchases.ResetDHFieldsForRepushDueToDelete(c, id)
				}
				return nil
			}))
			var unchanged, outcome, receipt string
			var settledAt sql.NullTime
			require.NoError(t, db.QueryRowContext(ctx, `SELECT (to_jsonb(a)-'outcome'-'settled_at'-'receipt')::text,outcome,settled_at,receipt FROM dh_mutation_attempts a WHERE id=$1`, req.ExpectedAttemptID).Scan(&unchanged, &outcome, &settledAt, &receipt))
			require.Equal(t, original, unchanged, "all frozen identity, payload, key and original timing survive")
			require.Equal(t, "abandoned", outcome)
			require.True(t, settledAt.Valid)
			var audit map[string]any
			require.NoError(t, json.Unmarshal([]byte(receipt), &audit))
			require.Equal(t, "administrative_abandonment", audit["type"])
			require.Equal(t, "unknown", audit["original_provider_outcome"])
			require.Equal(t, req.OperatorAuthorization, audit["operator_authorization"])
			require.Equal(t, req.QuiescenceEvidence, audit["quiescence_evidence"])
			require.Equal(t, float64(25000), audit["approved_listing_price_cents"])
			var watermark time.Time
			require.NoError(t, db.QueryRowContext(ctx, `SELECT mutation_settled_at FROM dh_target_watermarks WHERE grader='PSA' AND cert_number='legacy-price-cert'`).Scan(&watermark))
			require.False(t, watermark.Before(settledAt.Time))
			after, err := purchases.GetPurchase(ctx, id)
			require.NoError(t, err)
			if reset {
				require.Zero(t, after.DHInventoryID)
				require.NotNil(t, after.DHUnlistedDetectedAt)
				require.Equal(t, 25000, after.ReviewedPriceCents)
			} else {
				require.Equal(t, before, after, "abandonment alone must not alter local purchase or prices")
			}
			// Use the unchanged target case to prove the settlement watermark, not identity drift, drops this fetch.
			if !reset {
				applied, err := store.ApplyDHObservation(ctx, id, 364577, fetchedAt, func(context.Context) error { t.Fatal("stale observation applied"); return nil })
				require.NoError(t, err)
				require.False(t, applied)
				fresh, err := store.ObservationTime(ctx)
				require.NoError(t, err)
				applied, err = store.ApplyDHObservation(ctx, id, 364577, fresh, func(context.Context) error { return nil })
				require.NoError(t, err)
				require.True(t, applied)
			}
		})
	}
}

func TestAbandonLegacyDHPriceRollbackAndNormalSettlement(t *testing.T) {
	for _, mode := range []string{"caller rollback", "watermark failure", "normal settlement remains strict"} {
		t.Run(mode, func(t *testing.T) {
			db, store, id, req := setupLegacyPriceAbandonment(t)
			ctx := context.Background()
			before, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			var journalBefore, journalAfter string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT to_jsonb(a)::text FROM dh_mutation_attempts a WHERE id=$1`, req.ExpectedAttemptID).Scan(&journalBefore))
			if mode == "watermark failure" {
				_, err := db.ExecContext(ctx, `CREATE FUNCTION fail_legacy_watermark() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'watermark failed'; END $$; CREATE TRIGGER fail_legacy_watermark BEFORE INSERT OR UPDATE ON dh_target_watermarks FOR EACH ROW EXECUTE FUNCTION fail_legacy_watermark()`)
				require.NoError(t, err)
				t.Cleanup(func() {
					_, _ = db.ExecContext(ctx, `DROP TRIGGER fail_legacy_watermark ON dh_target_watermarks; DROP FUNCTION fail_legacy_watermark()`)
				})
			}
			failure := errors.New("caller rollback")
			err = store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
				if mode == "normal settlement remains strict" {
					require.NoError(t, store.OwnDHMutation(c, id, before.PrecedingAttempt))
					return store.SettleDHMutation(c, id, before.PrecedingAttempt, inventory.DHMutationSettlement{Outcome: "abandoned", Receipt: "not a verified provider response"})
				}
				if err := store.AbandonLegacyDHPrice(c, id, req); err != nil {
					return err
				}
				if err := NewPurchaseStore(db.DB, mocks.NewMockLogger()).ResetDHFieldsForRepushDueToDelete(c, id); err != nil {
					return err
				}
				return failure
			})
			switch mode {
			case "caller rollback":
				require.ErrorIs(t, err, failure)
			case "normal settlement remains strict":
				var conflict *inventory.ReturnConflict
				require.ErrorAs(t, err, &conflict)
				require.Equal(t, "terminal_receipt_required", conflict.Code)
			default:
				require.ErrorContains(t, err, "watermark failed")
			}
			require.NoError(t, db.QueryRowContext(ctx, `SELECT to_jsonb(a)::text FROM dh_mutation_attempts a WHERE id=$1`, req.ExpectedAttemptID).Scan(&journalAfter))
			require.Equal(t, journalBefore, journalAfter)
			after, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Equal(t, before, after)
			var settled *time.Time
			require.NoError(t, db.QueryRowContext(ctx, `SELECT mutation_settled_at FROM dh_target_watermarks WHERE grader='PSA' AND cert_number='legacy-price-cert'`).Scan(&settled))
			require.Nil(t, settled)
		})
	}
}

func TestAbandonLegacyDHPriceOnlySelectedTargetUnfenced(t *testing.T) {
	db, store, id, req := setupLegacyPriceAbandonment(t)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,dh_inventory_id,dh_status,reviewed_price_cents,received_at,purchase_date,created_at,updated_at) VALUES('unselected','return-c','Other','unselected-cert','PSA',42,'in_stock',25000,now(),'2026-01-01',now(),now())`)
	require.NoError(t, err)
	other := preparePriceAttempt(t, store, "unselected", inventory.DHMutationRequest{Kind: "price", Phase: "patch", PayloadIdentity: "other frozen payload"})
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AbandonLegacyDHPrice(c, id, req) }))
	for _, purchase := range []string{id, "unselected"} {
		t.Run(purchase, func(t *testing.T) {
			var prepared *inventory.DHMutationAttempt
			err := store.WithPurchaseMutation(ctx, purchase, func(c context.Context) error {
				var err error
				prepared, err = store.PrepareDHMutation(c, purchase, inventory.DHMutationRequest{Kind: "listing", Phase: "patch_and_sync", PayloadIdentity: "new approved listing"})
				return err
			})
			if purchase == id {
				require.NoError(t, err)
				require.NotEqual(t, req.ExpectedAttemptID, prepared.ID)
				require.False(t, prepared.Replaying)
			} else {
				require.ErrorIs(t, err, inventory.ErrReturnConflict)
				state, err := store.GetReturnState(ctx, purchase)
				require.NoError(t, err)
				require.Equal(t, other, state.PrecedingAttempt)
			}
		})
	}
}
