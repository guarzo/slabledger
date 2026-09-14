package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepPackingConcurrentDeleteCampaign(t *testing.T) {
	for _, tt := range []struct {
		name        string
		pack, multi bool
	}{
		{"pack", true, false},
		{"batch add", false, false},
		{"multi-purchase batch add", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ids := []string{showPurchase}
			if tt.multi {
				// Heap order opposes prep's ID order: the legacy bulk DELETE
				// takes the high ID first while prep first locks the low ID.
				seedShowPurchase(t, db, showOther, "camp-show", "other", "PSA")
				ids = append(ids, showOther)
			}
			seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
			store := NewShowPrepStore(db.DB)
			svc := sp.NewService(store, nil, time.Now)
			_, err := svc.CreateList(ctx, showList, "Delete race")
			require.NoError(t, err)
			es, err := svc.Evaluate(ctx, ids)
			require.NoError(t, err)
			adds := make([]sp.AddItem, 0, len(es))
			for _, e := range es {
				adds = append(adds, sp.AddItem{PurchaseID: e.PurchaseID, EvaluationVersion: e.Version})
			}
			var detail sp.ListDetail
			if tt.pack {
				detail, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
				require.NoError(t, err)
			}

			// Pause the actual DeleteCampaign after it owns the purchase row,
			// before it can request the campaign lock. No legacy writer is mocked.
			_, err = db.ExecContext(ctx, `CREATE FUNCTION showprep_pause_delete() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN PERFORM pg_advisory_xact_lock(731947); RETURN OLD; END $$`)
			require.NoError(t, err)
			defer func() {
				_, err := db.ExecContext(context.Background(), `DROP FUNCTION showprep_pause_delete() CASCADE`)
				require.NoError(t, err)
			}()
			_, err = db.ExecContext(ctx, `CREATE TRIGGER showprep_pause_delete BEFORE DELETE ON campaign_purchases FOR EACH ROW EXECUTE FUNCTION showprep_pause_delete()`)
			require.NoError(t, err)
			gate, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = gate.Rollback() }()
			_, err = gate.ExecContext(ctx, `SELECT pg_advisory_xact_lock(731947)`)
			require.NoError(t, err)
			deleted := make(chan error, 1)
			go func() {
				deleted <- NewCampaignStore(db.DB, mocks.NewMockLogger()).DeleteCampaign(ctx, "camp-show")
			}()
			waitShowPrepQueryLock(t, ctx, db, `DELETE FROM campaign_purchases WHERE campaign_id%`)
			if len(ids) > 1 {
				// Verify the controlled schedule, rather than assuming the planner
				// visited the fixture's heap rows in the intended order.
				requireShowPrepPurchaseUnlocked(t, ctx, db, showPurchase)
			}
			prepared := make(chan error, 1)
			go func() {
				if tt.pack {
					yes := true
					_, err := svc.UpdateItem(ctx, showList, detail.Items[0].ID, sp.UpdateItem{Version: 1, EvaluationVersion: es[0].Version, Packed: &yes})
					prepared <- err
				} else {
					_, err := svc.AddItems(ctx, showList, adds)
					prepared <- err
				}
			}()
			require.ErrorIs(t, <-prepared, sp.ErrConflict, "contended validation must fail closed, not wait into a lock cycle")
			if len(ids) > 1 {
				requireShowPrepPurchaseUnlocked(t, ctx, db, showPurchase)
			}
			require.NoError(t, gate.Commit())
			require.NoError(t, <-deleted, "prep must not deadlock the existing campaign deletion path")
			detail, err = svc.ListDetail(ctx, showList)
			require.NoError(t, err)
			require.Zero(t, detail.Summary.PackedCount)
			require.Zero(t, detail.Summary.KnownValueCents)
			if tt.pack {
				require.Len(t, detail.Items, 1)
				require.Equal(t, sp.Removed, detail.Items[0].Evaluation.Availability)
				require.Equal(t, int64(1), detail.Items[0].Version)
			} else {
				require.Empty(t, detail.Items)
			}
		})
	}
}

func requireShowPrepPurchaseUnlocked(t *testing.T, ctx context.Context, db *DB, purchaseID string) {
	t.Helper()
	probe, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = probe.Rollback() }()
	var id string
	require.NoError(t, probe.QueryRowContext(ctx, `SELECT id FROM campaign_purchases WHERE id=$1 FOR UPDATE NOWAIT`, purchaseID).Scan(&id), "low-ID row must be unlocked")
}

func waitShowPrepQueryLock(t *testing.T, ctx context.Context, db *DB, query string) {
	t.Helper()
	require.Eventually(t, func() bool {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1`, query).Scan(&count)
		return err == nil && count > 0
	}, 3*time.Second, 10*time.Millisecond, "expected a real PostgreSQL lock wait for %s", query)
}
