package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepCompoundLifecycle(t *testing.T) {
	for _, tt := range []struct {
		name, sql    string
		availability sp.Availability
	}{
		{"sold", `INSERT INTO campaign_sales(id,purchase_id,sale_price_cents,sale_date,sale_channel) VALUES('sale',$1,35000,'2026-09-14','local')`, sp.Sold},
		{"refunded", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`, sp.Refunded},
		{"closed", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`, sp.CampaignClosed},
		{"receipt removed", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`, sp.NotReceived},
		{"purchase removed", `DELETE FROM campaign_purchases WHERE id=$1`, sp.Removed},
		{"campaign removed", `DELETE FROM campaigns WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`, sp.Removed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
			store := NewShowPrepStore(db.DB)
			seedShowEvidence(t, store)
			svc := sp.NewService(store, nil, time.Now)
			list, err := svc.CreateList(ctx, showList, "  September Show  ")
			require.NoError(t, err)
			require.Equal(t, "September Show", list.Name)
			retry, err := svc.CreateList(ctx, showList, "September Show")
			require.NoError(t, err)
			require.Equal(t, list, retry)
			_, err = svc.CreateList(ctx, showList, "Different")
			require.ErrorIs(t, err, sp.ErrConflict)
			es, err := svc.Evaluate(ctx, []string{showPurchase})
			require.NoError(t, err)
			require.Equal(t, sp.Supported, es[0].Status)
			adds := []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}}
			d, err := svc.AddItems(ctx, showList, adds)
			require.NoError(t, err)
			require.Len(t, d.Items, 1)
			require.Equal(t, 30000, d.Summary.KnownValueCents)
			d2, err := svc.AddItems(ctx, showList, adds)
			require.NoError(t, err)
			require.Equal(t, d.Items, d2.Items)
			yes := true
			cmd := sp.UpdateItem{Version: 1, EvaluationVersion: d.Items[0].Evaluation.Version, Packed: &yes}
			d, err = svc.UpdateItem(ctx, showList, d.Items[0].ID, cmd)
			require.NoError(t, err)
			require.Equal(t, 1, d.Summary.PackedCount)
			item := d.Items[0]
			d2, err = svc.UpdateItem(ctx, showList, item.ID, cmd)
			require.NoError(t, err)
			require.Equal(t, item.Version, d2.Items[0].Version)
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_listing_price_cents=40000 WHERE id=$1`, showPurchase)
			require.NoError(t, err)
			svc = sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
			d, err = svc.ListDetail(ctx, showList)
			require.NoError(t, err)
			require.True(t, d.Items[0].PriceChanged)
			require.True(t, d.Items[0].SupportChanged)
			require.Equal(t, 1, d.Summary.PackedCount)
			_, err = svc.UpdateItem(ctx, showList, item.ID, sp.UpdateItem{Version: item.Version, EvaluationVersion: item.Evaluation.Version, Acknowledge: true})
			require.ErrorIs(t, err, sp.ErrConflict)
			d, err = svc.UpdateItem(ctx, showList, item.ID, sp.UpdateItem{Version: item.Version, EvaluationVersion: d.Items[0].Evaluation.Version, Acknowledge: true})
			require.NoError(t, err)
			require.False(t, d.Items[0].PriceChanged)
			_, err = db.ExecContext(ctx, tt.sql, showPurchase)
			require.NoError(t, err)
			d, err = svc.ListDetail(ctx, showList)
			require.NoError(t, err)
			require.Len(t, d.Items, 1)
			require.Equal(t, tt.availability, d.Items[0].Evaluation.Availability)
			require.Equal(t, 1, d.Summary.PackedCount)
			require.Zero(t, d.Summary.KnownValueCents)
			_, err = svc.UpdateItem(ctx, showList, item.ID, sp.UpdateItem{Version: d.Items[0].Version, EvaluationVersion: d.Items[0].Evaluation.Version, Packed: &yes})
			require.ErrorIs(t, err, sp.ErrConflict)
			no := false
			d, err = svc.UpdateItem(ctx, showList, item.ID, sp.UpdateItem{Version: d.Items[0].Version, Packed: &no})
			require.NoError(t, err)
			require.Zero(t, d.Summary.PackedCount)
			require.NoError(t, svc.RemoveItem(ctx, showList, item.ID))
			require.NoError(t, svc.RemoveItem(ctx, showList, item.ID))
		})
	}
}
func TestShowPrepPackingConflictStillRecordsNewCollision(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "camp-show", "collision", "PSA")
	svc := sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
	_, err := svc.CreateList(ctx, showList, "Collision during review")
	require.NoError(t, err)
	es, err := svc.Evaluate(ctx, []string{showPurchase})
	require.NoError(t, err)
	d, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
	require.NoError(t, err)
	seedShowPurchase(t, db, showOther, "other-camp", "collision", "BGS")
	yes := true
	_, err = svc.UpdateItem(ctx, showList, d.Items[0].ID, sp.UpdateItem{Version: 1, EvaluationVersion: d.Items[0].Evaluation.Version, Packed: &yes})
	require.ErrorIs(t, err, sp.ErrConflict)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&count))
	require.Equal(t, 2, count, "an optimistic conflict must not undo observation of ambiguity")
}

func TestShowPrepBatchAddRollback(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "camp-show", "cert1", "PSA")
	seedShowPurchase(t, db, showOther, "camp-show", "cert2", "PSA")
	svc := sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
	_, err := svc.CreateList(ctx, showList, "List")
	require.NoError(t, err)
	es, err := svc.Evaluate(ctx, []string{showPurchase, showOther})
	require.NoError(t, err)
	_, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}, {PurchaseID: showOther, EvaluationVersion: "stale"}})
	require.ErrorIs(t, err, sp.ErrConflict)
	d, err := svc.ListDetail(ctx, showList)
	require.NoError(t, err)
	require.Empty(t, d.Items)
}
func TestShowPrepDurablePriceHolds(t *testing.T) {
	for _, tt := range []struct {
		name   string
		remove func(context.Context, *DB) error
	}{
		{"purchase delete", func(ctx context.Context, db *DB) error {
			return NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(ctx, showOther)
		}},
		{"campaign delete", func(ctx context.Context, db *DB) error {
			return NewCampaignStore(db.DB, mocks.NewMockLogger()).DeleteCampaign(ctx, "other-camp")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "camp-show", "collision", "PSA")
			seedShowPurchase(t, db, showOther, "other-camp", "collision", "BGS")
			seedShowPurchase(t, db, "44444444-4444-4444-8444-444444444444", "camp-show", "unique", "PSA")
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_listing_price_cents=99999,was_refunded=true WHERE id=$1`, showOther)
			require.NoError(t, err)
			store := NewShowPrepStore(db.DB)
			seedShowEvidence(t, store)
			svc := sp.NewService(store, nil, time.Now)
			es, err := svc.Evaluate(ctx, []string{showPurchase})
			require.NoError(t, err)
			require.Equal(t, sp.NeedsReview, es[0].Status)
			require.True(t, es[0].PriceAssociationUnclear)
			var count int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&count))
			require.Equal(t, 2, count, "both rows held, unrelated row untouched")
			_, err = svc.CreateList(ctx, showList, "Held card")
			require.NoError(t, err)
			_, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
			require.NoError(t, err)
			require.NoError(t, tt.remove(ctx, db))
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_last_synced_at=$1 WHERE id=$2`, time.Now().Format(time.RFC3339), showPurchase)
			require.NoError(t, err)
			seedShowEvidence(t, store)
			svc = sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
			d, err := svc.ListDetail(ctx, showList)
			require.NoError(t, err)
			require.True(t, d.Items[0].Evaluation.PriceAssociationUnclear)
			require.Zero(t, d.Summary.KnownValueCents)
			require.Equal(t, 1, d.Summary.AmbiguousPriceCount)
			d, err = svc.UpdateItem(ctx, showList, d.Items[0].ID, sp.UpdateItem{Version: d.Items[0].Version, EvaluationVersion: d.Items[0].Evaluation.Version, Acknowledge: true})
			require.NoError(t, err)
			require.Equal(t, sp.NeedsReview, d.Items[0].Evaluation.Status)
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&count))
			require.Equal(t, 2, count)
		})
	}
}
