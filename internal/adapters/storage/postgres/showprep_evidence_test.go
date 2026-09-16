package postgres

import (
	"context"
	"encoding/json"
	"errors"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestShowPrepEvidenceGenerationAndRetention(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	store := NewShowPrepStore(db.DB)
	identity := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	seedShowEvidence(t, store)
	snapshots, err := store.ReadSnapshots(ctx, []sp.Identity{identity})
	require.NoError(t, err)
	initial := snapshots[identity]
	require.Len(t, initial.Sales, 2)
	older, err := store.BeginAttempt(ctx, identity, time.Now())
	require.NoError(t, err)
	newer, err := store.BeginAttempt(ctx, identity, time.Now())
	require.NoError(t, err)
	require.Greater(t, newer, older)
	require.NoError(t, store.FinishAttempt(ctx, identity, newer, sp.Snapshot{AttemptError: "source timeout"}))
	require.NoError(t, store.FinishAttempt(ctx, identity, older, sp.Snapshot{Complete: true, Sales: []sp.Sale{}}))
	snapshots, err = store.ReadSnapshots(ctx, []sp.Identity{identity})
	require.NoError(t, err)
	got := snapshots[identity]
	require.Equal(t, initial.Generation, got.Generation)
	require.Len(t, got.Sales, 2)
	require.Equal(t, "failed", got.AttemptState)
	require.Equal(t, newer, got.Attempt)
	// A cancelled client leaves a durably visible running attempt without deleting old evidence.
	_, err = store.BeginAttempt(ctx, identity, time.Now())
	require.NoError(t, err)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, store.FinishAttempt(cancelled, identity, newer+1, sp.Snapshot{Complete: true}))
	snapshots, err = store.ReadSnapshots(ctx, []sp.Identity{identity})
	require.NoError(t, err)
	require.Equal(t, "running", snapshots[identity].AttemptState)
	require.Len(t, snapshots[identity].Sales, 2)
	// Complete empty windows are publishable generations, not missing data.
	n, err := store.BeginAttempt(ctx, identity, time.Now())
	require.NoError(t, err)
	start, end := sp.Window(time.Now())
	require.NoError(t, store.FinishAttempt(ctx, identity, n, sp.Snapshot{Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: time.Now()}))
	snapshots, err = store.ReadSnapshots(ctx, []sp.Identity{identity})
	require.NoError(t, err)
	require.Equal(t, n, snapshots[identity].Generation)
	require.Empty(t, snapshots[identity].Sales)
	require.Equal(t, "complete", snapshots[identity].AttemptState)
}
func TestShowPrepNoPriceRefreshHealthAndRetainedEvidence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		failed bool
		reason string
	}{{"failed recheck", true, "CardLadder refresh failed"}, {"complete recheck", false, ""}} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=0 WHERE id=$1`, showPurchase)
			require.NoError(t, err)
			store := NewShowPrepStore(db.DB)
			seedShowEvidence(t, store)
			source := &mocks.ShowPrepSourceMock{FetchFn: func(_ context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
				if tt.failed {
					return sp.Snapshot{}, errors.New("source failed")
				}
				start, end := sp.Window(now)
				return sp.Snapshot{Identity: id, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now, Sales: []sp.Sale{{ID: "a", Date: end, PriceCents: 27000}, {ID: "b", Date: end, PriceCents: 27000}}}, nil
			}}
			svc := sp.NewService(store, source, time.Now)
			before, err := svc.Evidence(ctx, showPurchase)
			require.NoError(t, err)
			refreshed, err := svc.Refresh(ctx, []string{showPurchase}, time.Now().Add(time.Second), time.Now().Add(2*time.Second))
			require.NoError(t, err)
			require.Len(t, refreshed, 1)
			// Reconstruct the service to prove the health derives from saved attempts.
			after, err := sp.NewService(store, nil, time.Now).Evidence(ctx, showPurchase)
			require.NoError(t, err)
			require.Len(t, after.Sales, 2)
			if tt.failed {
				require.Equal(t, before.Sales, after.Sales)
			}
			for _, e := range []sp.Evaluation{refreshed[0], after.Evaluation} {
				require.Equal(t, sp.NoListedPrice, e.Status)
				require.Equal(t, "No positive SlabLedger asking price", e.Reason)
				require.Nil(t, e.Recent.GapPct)
				encoded, err := json.Marshal(e)
				require.NoError(t, err)
				var wire map[string]any
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.Equal(t, tt.failed, wire["evidenceNeedsReview"])
				require.Equal(t, tt.reason, wire["evidenceReason"])
			}
		})
	}
}

func TestShowPrepConcurrentRefreshPublishesNewestAttempt(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
	entered := make(chan struct{})
	release := make(chan struct{})
	oldSource := &mocks.ShowPrepSourceMock{FetchFn: func(_ context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
		close(entered)
		<-release
		start, end := sp.Window(now)
		return sp.Snapshot{Identity: id, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now, Sales: []sp.Sale{{ID: "old", Date: end, PriceCents: 10000}}}, nil
	}}
	newSource := &mocks.ShowPrepSourceMock{FetchFn: func(_ context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
		start, end := sp.Window(now)
		return sp.Snapshot{Identity: id, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now, Sales: []sp.Sale{{ID: "new", Date: end, PriceCents: 40000}}}, nil
	}}
	store := NewShowPrepStore(db.DB)
	older := sp.NewService(store, oldSource, time.Now)
	newer := sp.NewService(store, newSource, time.Now)
	done := make(chan error, 1)
	go func() {
		_, err := older.Refresh(ctx, []string{showPurchase}, time.Now().Add(5*time.Second), time.Now().Add(6*time.Second))
		done <- err
	}()
	<-entered
	_, err := newer.Refresh(ctx, []string{showPurchase}, time.Now().Add(5*time.Second), time.Now().Add(6*time.Second))
	require.NoError(t, err)
	close(release)
	require.NoError(t, <-done)
	identity := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	snapshots, err := store.ReadSnapshots(ctx, []sp.Identity{identity})
	require.NoError(t, err)
	require.Equal(t, int64(2), snapshots[identity].Generation)
	require.Equal(t, "new", snapshots[identity].Sales[0].ID)
}

func TestShowPrepIndependentTableResetAndRetention(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "camp-show", "collision", "PSA")
	seedShowPurchase(t, db, showOther, "camp-show", "collision", "BGS")
	store := NewShowPrepStore(db.DB)
	seedShowEvidence(t, store)
	svc := sp.NewService(store, nil, time.Now)
	_, err := svc.CreateList(ctx, showList, "Show")
	require.NoError(t, err)
	es, err := svc.Evaluate(ctx, []string{showPurchase})
	require.NoError(t, err)
	_, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
	require.NoError(t, err)
	require.NoError(t, NewCampaignStore(db.DB, mocks.NewMockLogger()).DeleteCampaign(ctx, "camp-show"))
	for _, table := range []string{"showprep_lists", "showprep_items", "showprep_evidence", "showprep_price_holds"} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count))
		require.Positive(t, count, table+" survives campaign deletion")
	}
	resetShowPrep(t, db)
	for _, table := range []string{"showprep_lists", "showprep_items", "showprep_evidence", "showprep_price_holds"} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count))
		require.Zero(t, count, table+" explicitly isolated")
	}
}
func TestShowPrepNeverMutatesFinancialRecords(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
	var before, after string
	query := `SELECT json_build_object('purchase',(SELECT row_to_json(p) FROM campaign_purchases p WHERE id=$1),'campaign',(SELECT row_to_json(c) FROM campaigns c WHERE id='camp-show'),'sales',(SELECT json_agg(s) FROM campaign_sales s))::text`
	require.NoError(t, db.QueryRowContext(ctx, query, showPurchase).Scan(&before))
	source := &mocks.ShowPrepSourceMock{FetchFn: func(_ context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
		start, end := sp.Window(now)
		return sp.Snapshot{Identity: id, Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now}, nil
	}}
	svc := sp.NewService(NewShowPrepStore(db.DB), source, time.Now)
	es, err := svc.Refresh(ctx, []string{showPurchase}, time.Now().Add(time.Second), time.Now().Add(2*time.Second))
	require.NoError(t, err)
	_, err = svc.CreateList(ctx, showList, "No mutation")
	require.NoError(t, err)
	d, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
	require.NoError(t, err)
	yes := true
	_, err = svc.UpdateItem(ctx, showList, d.Items[0].ID, sp.UpdateItem{Version: 1, EvaluationVersion: d.Items[0].Evaluation.Version, Packed: &yes})
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, query, showPurchase).Scan(&after))
	require.Equal(t, before, after)
}
