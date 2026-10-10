package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

const showScanCommand = "44444444-4444-4444-8444-444444444444"

var showScanNow = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func showScanIntent() sp.ScanCommand {
	return sp.ScanCommand{CommandID: showScanCommand, CertNumber: "12345678"}
}
func scanReceiptCount(t *testing.T, db *DB, want int) {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM showprep_command_receipts`).Scan(&n))
	require.Equal(t, want, n)
}
func setupScanService(t *testing.T) (*DB, *ShowPrepStore, *sp.Service) {
	t.Helper()
	db := setupShowPrepTestDB(t)
	seedShowPurchase(t, db, showPurchase, "camp-show", "12345678", "PSA")
	store := NewShowPrepStore(db.DB)
	svc := sp.NewService(store, nil, func() time.Time { return showScanNow })
	_, err := svc.CreateList(context.Background(), showList, "Scan")
	require.NoError(t, err)
	return db, store, svc
}

func TestShowPrepScanPersistenceReplay(t *testing.T) {
	for _, tt := range []struct{ name, state string }{
		{"price", "present"}, {"remove", "removed"}, {"remove readd restart", "removed"}, {"sale", "unavailable"}, {"refund", "unavailable"}, {"closed", "unavailable"}, {"receipt removed", "unavailable"}, {"purchase removed", "unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc := setupScanService(t)
			ctx := context.Background()
			cmd := showScanIntent()
			first, err := svc.ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.Equal(t, "confirmed", first.Original.Outcome)
			require.Equal(t, 30000, first.Original.AcknowledgedPriceCents)
			require.Equal(t, sp.NeedsReview, first.Original.AcknowledgedStatus)
			require.Equal(t, "2026-10-09T12:00:00Z", first.Original.ConfirmedAt)
			stored, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.EqualValues(t, 1, stored[0].Version)
			var write string
			switch tt.name {
			case "price":
				write = `UPDATE campaign_purchases SET reviewed_price_cents=40000 WHERE id=$1`
			case "sale":
				write = `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-10-09')`
			case "refund":
				write = `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`
			case "closed":
				write = `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`
			case "receipt removed":
				write = `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`
			case "purchase removed":
				write = `DELETE FROM campaign_purchases WHERE id=$1`
			case "remove", "remove readd restart":
				require.NoError(t, svc.RemoveItem(ctx, showList, stored[0].ID))
				if tt.name == "remove readd restart" {
					newCmd := cmd
					newCmd.CommandID = "55555555-5555-4555-8555-555555555555"
					newCmd.WithdrawalGeneration = 1
					readded, err := svc.ScanConfirm(ctx, showList, newCmd)
					require.NoError(t, err)
					require.NotEqual(t, first.Original.ItemID, readded.Original.ItemID)
				}
			}
			if write != "" {
				_, err = db.ExecContext(ctx, write, showPurchase)
				require.NoError(t, err)
			}
			beforeReplay, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			listBefore, listErr := store.GetList(ctx, showList)
			svc = sp.NewService(NewShowPrepStore(db.DB), nil, func() time.Time { return showScanNow.Add(24 * time.Hour) })
			replay, err := svc.ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.True(t, replay.Replay)
			require.Equal(t, first.Original, replay.Original)
			require.Equal(t, tt.state, replay.Current.State)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, beforeReplay, after)
			if listErr == nil {
				listAfter, err := store.GetList(ctx, showList)
				require.NoError(t, err)
				require.Equal(t, listBefore, listAfter)
			}
			want := 1
			if tt.name == "remove readd restart" {
				want = 2
			}
			scanReceiptCount(t, db, want)
		})
	}
}

func TestShowPrepScanGenerationLifetime(t *testing.T) {
	for _, tt := range []struct {
		name      string
		contended bool
	}{
		{"delayed first delivery", false}, {"unfinalized contention then remove", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc := setupScanService(t)
			ctx := context.Background()
			es, err := svc.Evaluate(ctx, []string{showPurchase})
			require.NoError(t, err)
			d, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
			require.NoError(t, err)
			cmd := showScanIntent()
			if tt.contended {
				writer, err := db.BeginTx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = writer.Rollback() }()
				_, err = writer.ExecContext(ctx, `SELECT id FROM campaign_purchases WHERE id=$1 FOR UPDATE`, showPurchase)
				require.NoError(t, err)
				_, err = svc.ScanConfirm(ctx, showList, cmd)
				require.ErrorIs(t, err, sp.ErrContended)
				scanReceiptCount(t, db, 0)
				require.NoError(t, writer.Rollback())
			}
			require.NoError(t, svc.RemoveItem(ctx, showList, d.Items[0].ID))
			result, err := svc.ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.Equal(t, "rejected", result.Original.Outcome)
			require.Equal(t, "contents_changed", result.Original.Code)
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Empty(t, items)
			scanReceiptCount(t, db, 1)
			replay, err := svc.ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.True(t, replay.Replay)
			require.Equal(t, result.Original, replay.Original)
		})
	}
}

func TestShowPrepScanAtomicReceiptFailure(t *testing.T) {
	db, store, svc := setupScanService(t)
	ctx := context.Background()
	// Force a real SQL failure after membership insert, not a mock rollback.
	_, err := db.ExecContext(ctx, `CREATE FUNCTION reject_scan_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'receipt unavailable'; END; $$; CREATE TRIGGER reject_scan_receipt BEFORE INSERT ON showprep_command_receipts FOR EACH ROW EXECUTE FUNCTION reject_scan_receipt()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS reject_scan_receipt ON showprep_command_receipts; DROP FUNCTION IF EXISTS reject_scan_receipt()`)
		require.NoError(t, err)
	})
	_, err = svc.ScanConfirm(ctx, showList, showScanIntent())
	require.ErrorContains(t, err, "receipt unavailable")
	items, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Empty(t, items)
	scanReceiptCount(t, db, 0)
	_, err = db.ExecContext(ctx, `DROP TRIGGER reject_scan_receipt ON showprep_command_receipts`)
	require.NoError(t, err)
	result, err := svc.ScanConfirm(ctx, showList, showScanIntent())
	require.NoError(t, err)
	require.Equal(t, "confirmed", result.Original.Outcome)
	scanReceiptCount(t, db, 1)
}

func TestShowPrepScanBaselineAndMembershipWrites(t *testing.T) {
	for _, tt := range []struct {
		name                string
		legacy, changePrice bool
		want                string
		version             int64
	}{
		{"legacy confirms", true, false, "confirmed", 2},
		{"unchanged confirmed", false, false, "already_confirmed", 1},
		{"changed confirmed", false, true, "rechecked", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc := setupScanService(t)
			ctx := context.Background()
			var item sp.Item
			if tt.legacy {
				es, err := svc.Evaluate(ctx, []string{showPurchase})
				require.NoError(t, err)
				d, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
				require.NoError(t, err)
				item = d.Items[0]
			} else {
				first, err := svc.ScanConfirm(ctx, showList, showScanIntent())
				require.NoError(t, err)
				items, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				item = items[0]
				require.Equal(t, first.Original.ItemID, item.ID)
			}
			if tt.changePrice {
				_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=40000 WHERE id=$1`, showPurchase)
				require.NoError(t, err)
			}
			listBefore, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			cmd := showScanIntent()
			cmd.CommandID = "55555555-5555-4555-8555-555555555555"
			result, err := svc.ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.Equal(t, tt.want, result.Original.Outcome)
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.Equal(t, item.ID, items[0].ID)
			require.Equal(t, item.AddedAt, items[0].AddedAt)
			require.Equal(t, tt.version, items[0].Version)
			if !tt.legacy {
				require.Equal(t, item.PackedAt, items[0].PackedAt)
			} else {
				require.Equal(t, "2026-10-09T12:00:00Z", items[0].PackedAt)
			}
			price := 30000
			if tt.changePrice {
				price = 40000
			}
			require.Equal(t, price, items[0].AcknowledgedPriceCents)
			require.Equal(t, sp.NeedsReview, items[0].AcknowledgedStatus)
			require.Equal(t, items[0].PackedAt, result.Original.ConfirmedAt)
			listAfter, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			require.Zero(t, listAfter.WithdrawalGeneration)
			if tt.want == "already_confirmed" {
				require.Equal(t, item.LastCommand, items[0].LastCommand)
				require.Equal(t, listBefore, listAfter)
			}
			count := 2
			if tt.legacy {
				count = 1
			}
			scanReceiptCount(t, db, count)
		})
	}
}

func TestShowPrepScanDuplicateWorkers(t *testing.T) {
	db, store, _ := setupScanService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results := make(chan sp.ScanResult, 2)
	errs := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			r, e := sp.NewService(NewShowPrepStore(db.DB), nil, func() time.Time { return showScanNow }).ScanConfirm(ctx, showList, showScanIntent())
			results <- r
			errs <- e
		}()
	}
	close(start)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	a, b := <-results, <-results
	require.NotEqual(t, a.Replay, b.Replay)
	require.Equal(t, a.Original, b.Original)
	items, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 1, items[0].Version)
	scanReceiptCount(t, db, 1)
}
