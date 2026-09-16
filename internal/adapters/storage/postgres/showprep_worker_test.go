package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func workerSource(fn func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error)) sp.SourceProvider {
	return func(context.Context) (sp.Source, error) { return &mocks.ShowPrepSourceMock{FetchFn: fn}, nil }
}
func workerEmpty(_ context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
	start, end := sp.Window(now)
	return sp.Snapshot{Identity: id, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now}, nil
}
func TestShowWorkerFleetCoverageAndCurrentZero(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		seedShowPurchase(t, db, fmt.Sprint("card", i), "worker", fmt.Sprint("cert", i), "PSA")
	}
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET received_at=NULL,dh_listing_price_cents=0;
 UPDATE campaign_purchases SET gem_rate_id=' psa-1 ' WHERE id='card1';
 UPDATE campaign_purchases SET gem_rate_id='' WHERE id='card2';
 UPDATE campaign_purchases SET was_refunded=true WHERE id='card3';
 INSERT INTO campaigns(id,name,phase) VALUES('closed','Closed','closed');
 UPDATE campaign_purchases SET campaign_id='closed' WHERE id='card4';
 INSERT INTO campaign_sales(id,purchase_id,sale_date,sale_price_cents,sale_channel) VALUES('sale','card5','2026-09-15',10000,'local');
 UPDATE campaign_purchases SET gem_rate_id='second' WHERE id='card6';
 UPDATE campaign_purchases SET grade_value=0 WHERE id='card7'`)
	require.NoError(t, err)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	var calls int
	worker := sp.NewEvidenceWorker(store, workerSource(func(c context.Context, id sp.Identity, n time.Time) (sp.Snapshot, error) {
		calls++
		return workerEmpty(c, id, n)
	}), func() time.Time { return now }, nil)
	before, err := worker.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 5, before.EligibleCards)
	require.Equal(t, 2, before.EligibleIdentities)
	require.Equal(t, 2, before.UnresolvedCards)
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 2, calls)
	status, err := worker.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, status.CurrentIdentities)
	require.Equal(t, 3, status.CurrentCards)
	require.Zero(t, status.MissingIdentities)
	// Coverage and the existing cached reader must agree for normalized profiles.
	cached, err := sp.NewService(NewShowPrepStore(db.DB), nil, func() time.Time { return now }).Evaluate(ctx, []string{"card1"})
	require.NoError(t, err)
	require.Equal(t, sp.ReadinessCurrent, cached[0].Readiness.State)
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 2, calls, "complete zero is current")
	now = time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 4, calls, "midnight window expires before 24h")
}

func TestShowWorkerFailuresAreFairDurableAndBounded(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "one", "PSA")
	seedShowPurchase(t, db, showOther, "worker", "two", "PSA")
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id='z-good' WHERE id=$1`, showOther)
	require.NoError(t, err)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	calls := map[string]int{}
	source := workerSource(func(c context.Context, id sp.Identity, n time.Time) (sp.Snapshot, error) {
		calls[id.ProfileID]++
		if id.ProfileID == "psa-1" {
			return sp.Snapshot{AttemptError: "secret https://user:password/provider?token=secret"}, nil
		}
		return workerEmpty(c, id, n)
	})
	run := func() {
		w := sp.NewEvidenceWorker(NewShowPrepWorkerStore(db.DB), source, func() time.Time { return now }, nil)
		require.ErrorIs(t, w.RunOnce(ctx), sp.ErrWorkerFailed)
	}
	run()
	require.Equal(t, 1, calls["psa-1"])
	require.Equal(t, 1, calls["z-good"])
	// A new process/store never resets backoff or charges another current success.
	w := sp.NewEvidenceWorker(store, source, func() time.Time { return now }, nil)
	require.NoError(t, w.RunOnce(ctx))
	require.Equal(t, 1, calls["psa-1"])
	now = now.Add(15*time.Minute - time.Nanosecond)
	require.NoError(t, w.RunOnce(ctx))
	require.Equal(t, 1, calls["psa-1"])
	now = now.Add(time.Nanosecond)
	run()
	require.Equal(t, 2, calls["psa-1"])
	now = now.Add(60*time.Minute - time.Nanosecond)
	require.NoError(t, w.RunOnce(ctx))
	require.Equal(t, 2, calls["psa-1"])
	now = now.Add(time.Nanosecond)
	run()
	require.Equal(t, 3, calls["psa-1"])
	now = now.Add(2 * time.Hour)
	require.NoError(t, w.RunOnce(ctx))
	require.Equal(t, 3, calls["psa-1"])
	status, err := w.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, status.FailedIdentities)
	require.Equal(t, "2026-09-16T00:00:00Z", status.RetryAt)
	require.NoError(t, w.RequestRun(ctx, true))
	require.NoError(t, w.RequestRun(ctx, true))
	require.Equal(t, 3, calls["psa-1"], "request only records intent")
	run()
	require.Equal(t, 4, calls["psa-1"])
	require.Equal(t, 1, calls["z-good"])
	require.NoError(t, w.RunOnce(ctx))
	require.Equal(t, 4, calls["psa-1"], "coalesced retry consumed once")
	snapshots, err := NewShowPrepStore(db.DB).ReadSnapshots(ctx, []sp.Identity{{ProfileID: "psa-1", Grader: "PSA", Grade: 10}})
	require.NoError(t, err)
	for _, s := range snapshots {
		require.NotContains(t, s.AttemptError, "secret")
		require.False(t, s.Complete)
	}
}

func TestShowWorkerAuthHoldAndRepairFenceOldCredentials(t *testing.T) {
	for _, repair := range []string{"credentials", "retry"} {
		t.Run(repair, func(t *testing.T) {
			db, store := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
			other := requireTestDB(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			source := workerSource(func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
				calls.Add(1)
				close(entered)
				<-release
				return sp.Snapshot{}, fmt.Errorf("wrapped: %w", apperrors.ProviderAuthFailed("cardladder", errors.New("token=secret")))
			})
			old := sp.NewEvidenceWorker(store, source, time.Now, nil)
			done := make(chan error, 1)
			go func() { done <- old.RunOnce(ctx) }()
			<-entered
			nextStore := NewShowPrepWorkerStore(other.DB)
			next := sp.NewEvidenceWorker(nextStore, workerSource(workerEmpty), time.Now, nil)
			require.NoError(t, next.RunOnce(ctx))
			require.Equal(t, int32(1), calls.Load(), "second connection cannot acquire")
			if repair == "credentials" {
				require.NoError(t, next.CredentialsChanged(ctx))
			} else {
				require.NoError(t, next.RequestRun(ctx, true))
			}
			close(release)
			require.ErrorIs(t, <-done, sp.ErrWorkerLeaseLost)
			state, err := nextStore.ReadState(ctx)
			require.NoError(t, err)
			require.False(t, state.AuthHold)
			// Repair preserves a wake intent, and the old completion cannot consume it.
			var requested bool
			require.NoError(t, db.QueryRowContext(ctx, `SELECT requested FROM showprep_worker`).Scan(&requested))
			require.True(t, requested)
			// Credentials alone do not waive a charged identity's retry budget.
			if repair == "credentials" {
				_, err = db.ExecContext(ctx, `UPDATE showprep_evidence SET retry_not_before=clock_timestamp()-interval '1 second'`)
				require.NoError(t, err)
			}
			require.NoError(t, next.RunOnce(ctx))
			status, err := next.Status(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, status.CurrentIdentities)
		})
	}
}

func TestShowWorkerAuthStopsWholeFleetUntilExplicitRepair(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "one", "PSA")
	seedShowPurchase(t, db, showOther, "worker", "two", "PSA")
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id='another' WHERE id=$1`, showOther)
	require.NoError(t, err)
	var calls int
	source := workerSource(func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
		calls++
		return sp.Snapshot{}, apperrors.ProviderAuthFailed("cardladder", errors.New("secret"))
	})
	w := sp.NewEvidenceWorker(store, source, time.Now, nil)
	require.ErrorIs(t, w.RunOnce(ctx), sp.ErrWorkerAuthHold)
	require.Equal(t, 1, calls)
	w = sp.NewEvidenceWorker(NewShowPrepWorkerStore(db.DB), source, time.Now, nil)
	require.NoError(t, w.RequestRun(ctx, false))
	require.NoError(t, w.RunOnce(ctx))
	require.Equal(t, 1, calls)
	status, err := w.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, "auth_hold", status.State)
	require.NoError(t, w.CredentialsChanged(ctx))
	require.ErrorIs(t, w.RunOnce(ctx), sp.ErrWorkerAuthHold)
	require.Equal(t, 2, calls)
}
