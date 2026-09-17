package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestShowPrepCanonicalPolicyPreservesLegacyList(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_listing_price_cents=90000,reviewed_price_cents=40000,reviewed_at='2026-09-16T12:00:00Z' WHERE id=$1`, showPurchase)
	require.NoError(t, err)
	store := NewShowPrepStore(db.DB)
	seedShowEvidence(t, store)
	svc := sp.NewService(store, nil, time.Now)
	_, err = svc.CreateList(ctx, showList, "Legacy packed list")
	require.NoError(t, err)
	yes := true
	legacyCommand := sp.UpdateItem{Version: 1, EvaluationVersion: "old-policy", Packed: &yes}
	const itemID = "44444444-4444-4444-8444-444444444444"
	_, err = db.ExecContext(ctx, `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,packed_at,version,acknowledged_price_cents,acknowledged_status,last_command)
	 VALUES($1,$2,$3,'Card','cert','PSA',10,'2026-09-14T10:00:00Z','2026-09-14T12:00:00Z',2,90000,'supported',$4)`, itemID, showList, showPurchase, sp.Fingerprint(legacyCommand))
	require.NoError(t, err)
	original, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	list, err := store.GetList(ctx, showList)
	require.NoError(t, err)
	for range 2 {
		d, err := svc.ListDetail(ctx, showList)
		require.NoError(t, err)
		require.Equal(t, 40000, d.Summary.KnownValueCents)
		require.True(t, d.Items[0].PriceChanged)
		require.True(t, d.Items[0].SupportChanged)
		require.Equal(t, sp.BelowTarget, d.Items[0].Evaluation.Status)
		require.Equal(t, 90000, d.Items[0].AcknowledgedPriceCents)
		require.Equal(t, sp.Supported, d.Items[0].AcknowledgedStatus)
		_, err = svc.UpdateItem(ctx, showList, itemID, legacyCommand)
		require.NoError(t, err, "exact old-policy replay is not a new acknowledgment")
		svc = sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
	}
	after, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Equal(t, original, after, "reads and replay preserve every saved field")
	afterList, err := store.GetList(ctx, showList)
	require.NoError(t, err)
	require.Equal(t, list, afterList)
	_, err = svc.UpdateItem(ctx, showList, itemID, sp.UpdateItem{Version: 2, EvaluationVersion: "old-policy", Acknowledge: true})
	require.ErrorIs(t, err, sp.ErrConflict)
	after, err = store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Equal(t, original, after, "stale acknowledgment cannot alter history")
	d, err := svc.ListDetail(ctx, showList)
	require.NoError(t, err)
	ack := sp.UpdateItem{Version: 2, EvaluationVersion: d.Items[0].Evaluation.Version, Acknowledge: true}
	d, err = svc.UpdateItem(ctx, showList, itemID, ack)
	require.NoError(t, err)
	require.EqualValues(t, 3, d.Items[0].Version)
	require.Equal(t, 40000, d.Items[0].AcknowledgedPriceCents)
	require.Equal(t, sp.BelowTarget, d.Items[0].AcknowledgedStatus)
	require.Equal(t, original[0].PackedAt, d.Items[0].PackedAt)
	require.False(t, d.Items[0].PriceChanged)
	require.False(t, d.Items[0].SupportChanged)
	acknowledged, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Equal(t, sp.Fingerprint(ack), acknowledged[0].LastCommand)

	// A later canonical price and DH collision change the projection, not history.
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET override_price_cents=30000,override_set_at='2026-09-16T13:00:00Z' WHERE id=$1`, showPurchase)
	require.NoError(t, err)
	seedShowPurchase(t, db, showOther, "other-camp", "cert", "BGS")
	d, err = svc.UpdateItem(ctx, showList, itemID, ack)
	require.NoError(t, err)
	require.Equal(t, sp.Supported, d.Items[0].Evaluation.Status)
	require.True(t, d.Items[0].Evaluation.PriceAssociationUnclear)
	require.Equal(t, 30000, d.Summary.KnownValueCents)
	require.Equal(t, 1, d.Summary.AmbiguousPriceCount)
	require.True(t, d.Items[0].PriceChanged)
	require.True(t, d.Items[0].SupportChanged)
	_, err = db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='other-camp'`)
	require.NoError(t, err)
	svc = sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
	d, err = svc.UpdateItem(ctx, showList, itemID, ack)
	require.NoError(t, err)
	require.True(t, d.Items[0].Evaluation.PriceAssociationUnclear, "hold survives competitor deletion and restart")
	require.Equal(t, 30000, d.Summary.KnownValueCents)
	require.Equal(t, 1, d.Summary.AmbiguousPriceCount)
	after, err = store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Equal(t, acknowledged, after, "retry cannot repack, increment version or acknowledge new inputs")
	var holds int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&holds))
	require.Equal(t, 2, holds)
}

func TestShowPrepCanonicalPriceProjectionAndAdd(t *testing.T) {
	for _, tt := range []struct {
		name, sql string
		price     int
		status    sp.Status
	}{
		{"newer reviewed price", `reviewed_price_cents=30000,reviewed_at='2026-09-16T14:00:00Z',override_price_cents=40000,override_set_at='2026-09-16T13:00:00Z'`, 30000, sp.Supported},
		{"newer override price", `reviewed_price_cents=40000,reviewed_at='2026-09-16T12:00:00Z',override_price_cents=30000,override_set_at='2026-09-16T13:00:00Z'`, 30000, sp.Supported},
		{"no borrowing DH or CL", `reviewed_price_cents=0,override_price_cents=0,cl_value_cents=30000`, 0, sp.NoListedPrice},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_listing_price_cents=90000,`+tt.sql+` WHERE id=$1`, showPurchase)
			require.NoError(t, err)
			store := NewShowPrepStore(db.DB)
			seedShowEvidence(t, store)
			svc := sp.NewService(store, nil, time.Now)
			es, err := svc.Evaluate(ctx, []string{showPurchase})
			require.NoError(t, err)
			require.Equal(t, tt.status, es[0].Status)
			require.Equal(t, tt.price, es[0].LocalPriceCents)
			evidence, err := svc.Evidence(ctx, showPurchase)
			require.NoError(t, err)
			require.Equal(t, es[0], evidence.Evaluation)
			_, err = svc.CreateList(ctx, showList, "Canonical add")
			require.NoError(t, err)
			adds := []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}}
			d, err := svc.AddItems(ctx, showList, adds)
			require.NoError(t, err)
			require.Equal(t, tt.price, d.Items[0].AcknowledgedPriceCents)
			require.Equal(t, tt.price, d.Summary.KnownValueCents)
			require.Equal(t, tt.status, d.Items[0].AcknowledgedStatus)
			require.Equal(t, tt.price == 0, d.Summary.MissingPriceCount == 1)
			retry, err := svc.AddItems(ctx, showList, adds)
			require.NoError(t, err)
			require.Equal(t, d, retry)
		})
	}
}
