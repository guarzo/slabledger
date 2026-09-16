package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestShowWorkerAbandonedStartsSurviveRestart(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	id := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	for i := 1; i <= 3; i++ {
		lease, ok, err := store.Acquire(ctx, "abandoned")
		require.NoError(t, err)
		require.True(t, ok)
		_, err = store.Begin(ctx, lease, id, now)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE showprep_worker SET lease_until=clock_timestamp()-interval '1 second'`)
		require.NoError(t, err)
		nextStore := NewShowPrepWorkerStore(requireTestDB(t).DB)
		next, ok, err := nextStore.Acquire(ctx, "restart")
		require.NoError(t, err)
		require.True(t, ok)
		_, err = nextStore.Begin(ctx, next, id, now)
		require.ErrorIs(t, err, sp.ErrConflict)
		require.NoError(t, nextStore.End(ctx, next, sp.WorkerRunResult{State: "idle"}, now))
		if i == 1 {
			now = now.Add(15 * time.Minute)
		} else {
			now = now.Add(time.Hour)
		}
	}
	lease, ok, err := store.Acquire(ctx, "exhausted")
	require.NoError(t, err)
	require.True(t, ok)
	_, err = store.Begin(ctx, lease, id, now)
	require.ErrorIs(t, err, sp.ErrConflict)
	candidates, err := store.Candidates(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, candidates[0].Attempts)
	// The next UTC window, not a restart, renews the budget.
	_, err = store.Begin(ctx, lease, id, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
}

func TestShowWorkerSourceDoesNotHoldBusinessLockOrWriteFinance(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	seedShowEvidence(t, NewShowPrepStore(db.DB))
	now := time.Now().UTC().AddDate(0, 0, 1)
	before, err := NewShowPrepStore(db.DB).ReadSnapshots(ctx, []sp.Identity{{ProfileID: "psa-1", Grader: "PSA", Grade: 10}})
	require.NoError(t, err)
	var financeBefore, financeAfter string
	query := `SELECT json_build_object('p',(SELECT json_agg(p) FROM campaign_purchases p),'s',(SELECT json_agg(s) FROM campaign_sales s),'c',(SELECT json_agg(c) FROM campaigns c))::text`
	require.NoError(t, db.QueryRowContext(ctx, query).Scan(&financeBefore))
	source := workerSource(func(c context.Context, id sp.Identity, n time.Time) (sp.Snapshot, error) {
		conn, err := db.Conn(c)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		tx, err := conn.BeginTx(c, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		var acquired bool
		require.NoError(t, tx.QueryRowContext(c, `SELECT pg_try_advisory_xact_lock(731946)`).Scan(&acquired))
		require.True(t, acquired)
		return sp.Snapshot{Sales: []sp.Sale{{ID: "partial", Date: "2026-09-15", PriceCents: 1}}, AttemptError: "https://secret"}, nil
	})
	w := sp.NewEvidenceWorker(store, source, func() time.Time { return now }, nil)
	require.ErrorIs(t, w.RunOnce(ctx), sp.ErrWorkerFailed)
	after, err := NewShowPrepStore(db.DB).ReadSnapshots(ctx, []sp.Identity{{ProfileID: "psa-1", Grader: "PSA", Grade: 10}})
	require.NoError(t, err)
	for id, b := range before {
		require.Equal(t, b.Generation, after[id].Generation)
		require.Equal(t, b.Sales, after[id].Sales)
		require.Equal(t, "partial", after[id].AttemptState)
		require.NotContains(t, after[id].AttemptError, "secret")
	}
	require.NoError(t, db.QueryRowContext(ctx, query).Scan(&financeAfter))
	require.Equal(t, financeBefore, financeAfter)
}

func TestShowWorkerCancellationAndHeartbeatLoss(t *testing.T) {
	for _, mode := range []string{"shutdown", "lease loss"} {
		t.Run(mode, func(t *testing.T) {
			db, store := setupShowWorkerDB(t)
			seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
			entered := make(chan struct{})
			sourceStopped := make(chan struct{})
			source := workerSource(func(ctx context.Context, _ sp.Identity, _ time.Time) (sp.Snapshot, error) {
				close(entered)
				<-ctx.Done()
				close(sourceStopped)
				return sp.Snapshot{}, ctx.Err()
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			w := sp.NewEvidenceWorker(store, source, time.Now, nil)
			go func() { done <- w.RunOnce(ctx) }()
			<-entered
			if mode == "shutdown" {
				cancel()
			} else {
				_, err := db.ExecContext(ctx, `UPDATE showprep_worker SET lease_until=clock_timestamp()-interval '1 second'`)
				require.NoError(t, err)
			}
			select {
			case err := <-done:
				if mode == "shutdown" {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.ErrorIs(t, err, sp.ErrWorkerLeaseLost)
				}
			case <-time.After(13 * time.Second):
				t.Fatal("worker did not cancel/join after lease loss")
			}
			<-sourceStopped
			// No detached failure or release can hide the charged interrupted start.
			candidates, err := store.Candidates(context.Background())
			require.NoError(t, err)
			require.Equal(t, "running", candidates[0].Snapshot.AttemptState)
			require.Equal(t, 1, candidates[0].Attempts)
		})
	}
}

func TestShowWorkerBoundedSweepContinuesWithUnattemptedPeers(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	for _, id := range []string{"one", "two", "three"} {
		seedShowPurchase(t, db, id, "worker", id, "PSA")
		_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id=$1 WHERE id=$1`, id)
		require.NoError(t, err)
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	calls := 0
	source := workerSource(func(c context.Context, id sp.Identity, n time.Time) (sp.Snapshot, error) {
		calls++
		if calls == 1 {
			<-c.Done()
			return sp.Snapshot{}, c.Err()
		}
		return workerEmpty(c, id, n)
	})
	w := sp.NewEvidenceWorker(store, source, func() time.Time { return now }, nil)
	bounded, cancel := context.WithTimeout(ctx, 5200*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, w.RunOnce(bounded), sp.ErrWorkerFailed)
	require.Equal(t, 1, calls)
	require.NoError(t, w.RunOnce(ctx))
	require.Equal(t, 3, calls)
	status, err := w.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, status.CurrentIdentities)
	require.Equal(t, 1, status.FailedIdentities)
	require.Equal(t, "failed", status.State)
}

func TestShowWorkerUnconfiguredDoesNotFailFleet(t *testing.T) {
	for _, mode := range []string{"nil provider", "nil source", "missing config", "fetch missing config", "resolve auth"} {
		t.Run(mode, func(t *testing.T) {
			db, store := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
			seedShowPurchase(t, db, showOther, "worker", "other", "PSA")
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id='other' WHERE id=$1`, showOther)
			require.NoError(t, err)
			var provider sp.SourceProvider
			var calls int
			switch mode {
			case "nil source":
				provider = func(context.Context) (sp.Source, error) { return nil, nil }
			case "missing config":
				provider = func(context.Context) (sp.Source, error) { return nil, apperrors.ConfigMissing("CardLadder", "") }
			case "resolve auth":
				provider = func(context.Context) (sp.Source, error) {
					return nil, apperrors.ProviderAuthFailed("CardLadder", errors.New("secret"))
				}
			case "fetch missing config":
				provider = workerSource(func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
					calls++
					return sp.Snapshot{}, apperrors.ConfigMissing("CardLadder", "")
				})
			}
			w := sp.NewEvidenceWorker(store, provider, time.Now, nil)
			err = w.RunOnce(ctx)
			if mode == "resolve auth" {
				require.ErrorIs(t, err, sp.ErrWorkerAuthHold)
			} else {
				require.NoError(t, err)
			}
			status, err := w.Status(ctx)
			require.NoError(t, err)
			if mode == "resolve auth" {
				require.Equal(t, "auth_hold", status.State)
			} else {
				require.Equal(t, "unconfigured", status.State)
			}
			if mode == "fetch missing config" {
				require.Equal(t, 1, calls)
			} else {
				require.Equal(t, 2, status.MissingIdentities)
				require.Zero(t, status.FailedIdentities)
			}
		})
	}
}
