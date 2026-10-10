package postgres

import (
	"context"
	"slices"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

const showTransitionCommand = "66666666-6666-4666-8666-666666666666"

func setupTransitionService(t *testing.T) (*DB, *ShowPrepStore, *sp.Service, sp.ConfirmExistingCommand) {
	t.Helper()
	db, store, svc := setupScanService(t)
	seedShowPurchase(t, db, showOther, "camp-other", "87654321", "PSA")
	ids := []string{showPurchase, showOther}
	es, err := svc.Evaluate(context.Background(), ids)
	require.NoError(t, err)
	adds := []sp.AddItem{}
	for _, e := range es {
		adds = append(adds, sp.AddItem{PurchaseID: e.PurchaseID, EvaluationVersion: e.Version})
	}
	d, err := svc.AddItems(context.Background(), showList, adds)
	require.NoError(t, err)
	itemIDs := []string{}
	for _, item := range d.Items {
		itemIDs = append(itemIDs, item.ID)
	}
	return db, store, svc, sp.ConfirmExistingCommand{CommandID: showTransitionCommand, ExpectedState: d.StateFingerprint, ItemIDs: itemIDs}
}

func TestShowPrepTransitionPersistenceReplay(t *testing.T) {
	for _, tt := range []struct {
		name   string
		remove bool
	}{{"later price", false}, {"later removal restart", true}} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc, cmd := setupTransitionService(t)
			ctx := context.Background()
			// One member already has packing history, one has no asking. Both stay eligible.
			_, err := db.ExecContext(ctx, `UPDATE showprep_items SET packed_at='2026-10-02T12:00:00Z',acknowledged_status='supported' WHERE purchase_id=$1`, showOther)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=0 WHERE id=$1`, showPurchase)
			require.NoError(t, err)
			d, err := svc.ListDetail(ctx, showList)
			require.NoError(t, err)
			cmd.ExpectedState = d.StateFingerprint
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			result, err := svc.ConfirmExisting(ctx, showList, cmd)
			require.NoError(t, err)
			require.False(t, result.Replay)
			sorted := slices.Clone(cmd.ItemIDs)
			slices.Sort(sorted)
			require.Equal(t, sorted, result.ConfirmedItemIDs)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Len(t, after, 2)
			for i, item := range after {
				require.Equal(t, before[i].ID, item.ID)
				require.Equal(t, before[i].AddedAt, item.AddedAt)
				require.Equal(t, before[i].Version+1, item.Version)
				if item.PurchaseID == showOther {
					require.Equal(t, "2026-10-02T12:00:00Z", item.PackedAt)
					require.Equal(t, 30000, item.AcknowledgedPriceCents)
					require.Equal(t, sp.NeedsReview, item.AcknowledgedStatus)
				} else {
					require.Equal(t, "2026-10-09T12:00:00Z", item.PackedAt)
					require.Zero(t, item.AcknowledgedPriceCents)
					require.Equal(t, sp.NoListedPrice, item.AcknowledgedStatus)
				}
			}
			scanReceiptCount(t, db, 1)
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=45000 WHERE id=$1`, showPurchase)
			require.NoError(t, err)
			if tt.remove {
				require.NoError(t, svc.RemoveItem(ctx, showList, after[0].ID))
			}
			beforeReplay, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			listBefore, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			svc = sp.NewService(NewShowPrepStore(db.DB), nil, func() time.Time { return showScanNow.Add(time.Hour) })
			slices.Reverse(cmd.ItemIDs)
			replay, err := svc.ConfirmExisting(ctx, showList, cmd)
			require.NoError(t, err)
			result.Replay = true
			require.Equal(t, result, replay)
			afterReplay, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, beforeReplay, afterReplay)
			listAfter, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, listBefore, listAfter)
			scanReceiptCount(t, db, 1)
			cmd.ExpectedState = "altered"
			_, err = svc.ConfirmExisting(ctx, showList, cmd)
			require.ErrorIs(t, err, sp.ErrConflict)
		})
	}
}

func TestShowPrepTransitionRejectsAtomicCohort(t *testing.T) {
	for _, tt := range []struct{ name, write string }{
		{"sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-10-09')`},
		{"refund", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`},
		{"not received", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`},
		{"closure", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`},
		{"purchase deletion", `DELETE FROM campaign_purchases WHERE id=$1`},
		{"cert", `UPDATE campaign_purchases SET cert_number='changed' WHERE id=$1`},
		{"grader", `UPDATE campaign_purchases SET grader='BGS' WHERE id=$1`},
		{"grade", `UPDATE campaign_purchases SET grade_value=9 WHERE id=$1`},
		{"rename", `UPDATE showprep_lists SET name='Changed'`},
		{"new member", ""}, {"remove member", ""}, {"omit member", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc, cmd := setupTransitionService(t)
			ctx := context.Background()
			if tt.write != "" {
				var err error
				if tt.name == "rename" {
					_, err = db.ExecContext(ctx, tt.write)
				} else {
					_, err = db.ExecContext(ctx, tt.write, showOther)
				}
				require.NoError(t, err)
			}
			switch tt.name {
			case "new member":
				seedShowPurchase(t, db, "77777777-7777-4777-8777-777777777777", "camp-new", "new", "PSA")
				es, err := svc.Evaluate(ctx, []string{"77777777-7777-4777-8777-777777777777"})
				require.NoError(t, err)
				_, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: es[0].PurchaseID, EvaluationVersion: es[0].Version}})
				require.NoError(t, err)
			case "remove member":
				require.NoError(t, svc.RemoveItem(ctx, showList, cmd.ItemIDs[0]))
			case "omit member":
				cmd.ItemIDs = cmd.ItemIDs[:1]
			}
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			_, err = svc.ConfirmExisting(ctx, showList, cmd)
			require.ErrorIs(t, err, sp.ErrConflict)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, before, after)
			scanReceiptCount(t, db, 0)
		})
	}
}

func TestShowPrepTransitionReceiptFailureRollback(t *testing.T) {
	for _, failure := range []string{"receipt", "second member"} {
		t.Run(failure, func(t *testing.T) {
			db, store, svc, cmd := setupTransitionService(t)
			ctx := context.Background()
			if failure == "second member" {
				// Force a successful first SaveItem before the later SQL failure;
				// random UUID ordering must not turn this into a first-write test.
				_, err := db.ExecContext(ctx, `UPDATE showprep_items SET added_at=CASE WHEN purchase_id=$1 THEN '2026-10-01T12:00:00Z'::timestamptz ELSE '2026-10-02T12:00:00Z'::timestamptz END`, showPurchase)
				require.NoError(t, err)
				d, err := svc.ListDetail(ctx, showList)
				require.NoError(t, err)
				cmd.ExpectedState = d.StateFingerprint
			}
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			listBefore, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			trigger := `CREATE FUNCTION reject_transition() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'transition unavailable'; END; $$; CREATE TRIGGER reject_transition BEFORE INSERT ON showprep_command_receipts FOR EACH ROW EXECUTE FUNCTION reject_transition()`
			if failure == "second member" {
				trigger = `CREATE FUNCTION reject_transition() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.purchase_id='22222222-2222-4222-8222-222222222222' THEN RAISE EXCEPTION 'transition unavailable'; END IF; RETURN NEW; END; $$; CREATE TRIGGER reject_transition BEFORE UPDATE ON showprep_items FOR EACH ROW EXECUTE FUNCTION reject_transition()`
			}
			_, err = db.ExecContext(ctx, trigger)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := db.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS reject_transition() CASCADE`)
				require.NoError(t, err)
			})
			_, err = svc.ConfirmExisting(ctx, showList, cmd)
			require.ErrorContains(t, err, "transition unavailable")
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, before, after)
			listAfter, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, listBefore, listAfter)
			scanReceiptCount(t, db, 0)
		})
	}
}
