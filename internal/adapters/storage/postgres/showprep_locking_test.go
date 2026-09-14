package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestShowPrepPackingLocksFinancialWriters(t *testing.T) {
	for _, tt := range []struct {
		name, write  string
		availability sp.Availability
	}{
		{"sale FK", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-09-14')`, sp.Sold},
		{"refund", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`, sp.Refunded},
		{"receipt", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`, sp.NotReceived},
		{"closure", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`, sp.CampaignClosed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
			store := NewShowPrepStore(db.DB)
			svc := sp.NewService(store, nil, time.Now)
			_, err := svc.CreateList(ctx, showList, "Locks")
			require.NoError(t, err)
			es, err := svc.Evaluate(ctx, []string{showPurchase})
			require.NoError(t, err)
			d, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
			require.NoError(t, err)
			// Exercise the real transaction/session used by UpdateItem, pausing after
			// eligibility read to expose the exact formerly vulnerable interleaving.
			locked := make(chan struct{})
			release := make(chan struct{})
			packed := make(chan error, 1)
			go func() {
				packed <- store.Within(ctx, func(tx sp.Session) error {
					if err := tx.LockPurchases(ctx, []string{showPurchase}); err != nil {
						return err
					}
					ps, err := tx.ReadPurchases(ctx, []string{showPurchase})
					if err != nil {
						return err
					}
					if sp.AvailabilityOf(ps[showPurchase]) != sp.Ready {
						return sp.ErrConflict
					}
					close(locked)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					item := d.Items[0]
					item.PackedAt = sp.Timestamp(time.Now())
					item.Version++
					return tx.SaveItem(ctx, showList, item)
				})
			}()
			select {
			case <-locked:
			case err := <-packed:
				t.Fatalf("packing failed before lock: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			conn, err := db.Conn(ctx)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			var pid int
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			written := make(chan error, 1)
			go func() { _, err := conn.ExecContext(ctx, tt.write, showPurchase); written <- err }()
			require.Eventually(t, func() bool {
				var waiting bool
				err := db.QueryRowContext(ctx, `SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting)
				return err == nil && waiting
			}, 2*time.Second, 10*time.Millisecond, "financial writer must actually block on row/FK locks")
			select {
			case err := <-written:
				t.Fatalf("writer interleaved before packing commit: %v", err)
			default:
			}
			close(release)
			require.NoError(t, <-packed)
			require.NoError(t, <-written)
			d, err = svc.ListDetail(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, tt.availability, d.Items[0].Evaluation.Availability)
			require.Equal(t, 1, d.Summary.PackedCount)
			yes := true
			_, err = svc.UpdateItem(ctx, showList, d.Items[0].ID, sp.UpdateItem{Version: d.Items[0].Version, EvaluationVersion: d.Items[0].Evaluation.Version, Packed: &yes})
			require.ErrorIs(t, err, sp.ErrConflict)
		})
	}
}

func TestShowPrepPackingRejectsContendedAvailability(t *testing.T) {
	for _, tt := range []struct {
		name, write  string
		availability sp.Availability
	}{
		{"sale FK", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-09-14')`, sp.Sold},
		{"refund", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`, sp.Refunded},
		{"receipt", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`, sp.NotReceived},
		{"closure", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`, sp.CampaignClosed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
			svc := sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
			_, err := svc.CreateList(ctx, showList, "Contended availability")
			require.NoError(t, err)
			es, err := svc.Evaluate(ctx, []string{showPurchase})
			require.NoError(t, err)
			d, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
			require.NoError(t, err)
			writer, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = writer.Rollback() }()
			_, err = writer.ExecContext(ctx, tt.write, showPurchase)
			require.NoError(t, err)
			yes := true
			_, err = svc.UpdateItem(ctx, showList, d.Items[0].ID, sp.UpdateItem{Version: 1, EvaluationVersion: d.Items[0].Evaluation.Version, Packed: &yes})
			require.ErrorIs(t, err, sp.ErrConflict, "contention requires review rather than waiting into a possible cycle")
			require.NoError(t, writer.Commit())
			d, err = svc.ListDetail(ctx, showList)
			require.NoError(t, err)
			require.Zero(t, d.Summary.PackedCount)
			require.Equal(t, tt.availability, d.Items[0].Evaluation.Availability)
		})
	}
}
