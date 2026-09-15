package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestShowWorkerOverageAcquisitionRetainsPayloadAndBackoff(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	id := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	legacy := NewShowPrepStore(db.DB)
	yesterday := now.AddDate(0, 0, -1)
	generation, err := legacy.BeginAttempt(ctx, id, yesterday)
	require.NoError(t, err)
	retained, err := workerEmpty(ctx, id, yesterday)
	require.NoError(t, err)
	retained.Sales = []sp.Sale{{ID: "retained", Date: "2026-09-14", PriceCents: 27000}}
	require.NoError(t, legacy.FinishAttempt(ctx, id, generation, retained))
	calls := 0
	source := workerSource(func(c context.Context, i sp.Identity, n time.Time) (sp.Snapshot, error) {
		calls++
		result, err := workerEmpty(c, i, n)
		result.RefreshedAt = n.Add(-24*time.Hour - time.Nanosecond)
		result.Sales = []sp.Sale{{ID: "overage", Date: "2026-09-15", PriceCents: 1}}
		return result, err
	})
	worker := sp.NewEvidenceWorker(store, source, func() time.Time { return now }, nil)
	for index, delay := range []time.Duration{15 * time.Minute, 60 * time.Minute} {
		require.ErrorIs(t, worker.RunOnce(ctx), sp.ErrWorkerFailed)
		require.Equal(t, index+1, calls)
		candidates, err := store.Candidates(ctx)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		got := candidates[0]
		require.Equal(t, "partial", got.Snapshot.AttemptState)
		require.Equal(t, generation, got.Snapshot.Generation)
		require.Equal(t, retained.Sales, got.Snapshot.Sales)
		require.Equal(t, index+1, got.Attempts)
		require.True(t, now.Add(delay).Equal(got.NotBefore))
		status, err := worker.Status(ctx)
		require.NoError(t, err)
		require.Equal(t, "failed", status.State)
		require.Equal(t, 1, status.FailedIdentities)
		require.Zero(t, status.CurrentIdentities)
		require.Equal(t, sp.Timestamp(now.Add(delay)), status.RetryAt)
		now = now.Add(delay - time.Nanosecond)
		// Restart must not turn an old current-window response into a fresh attempt.
		worker = sp.NewEvidenceWorker(NewShowPrepWorkerStore(db.DB), source, func() time.Time { return now }, nil)
		require.NoError(t, worker.RunOnce(ctx))
		require.Equal(t, index+1, calls)
		now = now.Add(time.Nanosecond)
	}
}

func TestShowWorkerAcquisitionAgeBoundaryAndMidnight(t *testing.T) {
	for _, tt := range []struct {
		name          string
		start         time.Time
		age           time.Duration
		crossMidnight bool
		wantState     string
	}{
		{"exactly 24 hours", time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), 24 * time.Hour, false, "current"},
		{"finishes after UTC midnight", time.Date(2026, 9, 15, 23, 59, 59, 0, time.UTC), 0, true, "stale"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
			now := tt.start
			source := workerSource(func(c context.Context, id sp.Identity, started time.Time) (sp.Snapshot, error) {
				snapshot, err := workerEmpty(c, id, started)
				if tt.crossMidnight {
					now = started.Add(2 * time.Second)
				}
				snapshot.RefreshedAt = now.Add(-tt.age)
				return snapshot, err
			})
			w := sp.NewEvidenceWorker(store, source, func() time.Time { return now }, nil)
			require.NoError(t, w.RunOnce(ctx))
			candidates, err := store.Candidates(ctx)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			require.Equal(t, "complete", candidates[0].Snapshot.AttemptState)
			require.True(t, candidates[0].Snapshot.Complete)
			require.True(t, candidates[0].NotBefore.IsZero(), "only valid completion clears backoff")
			require.Equal(t, tt.wantState, candidates[0].Classification(now))
			require.Equal(t, "2026-09-15", candidates[0].Snapshot.WindowEnd)
		})
	}
}

func TestShowWorkerCanonicalInventoryGroupingAndCachedReaders(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	// All four forms must share one persisted identity and one dispatch. Two
	// whitespace-only fields are unresolved cards, not additional identities.
	profiles := []string{"\tpsa-1\n", "\npsa-1\t", "\u00a0psa-1\u2003", " \u3000psa-1\u202f ", "\t\n", "\u00a0\u2003\u3000"}
	for i, profile := range profiles {
		purchase := fmt.Sprintf("canonical-%d", i)
		seedShowPurchase(t, db, purchase, "worker", purchase, "PSA")
		_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id=$1 WHERE id=$2`, profile, purchase)
		require.NoError(t, err)
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	canonical := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	calls := 0
	source := workerSource(func(c context.Context, id sp.Identity, n time.Time) (sp.Snapshot, error) {
		calls++
		require.Equal(t, canonical, id)
		return workerEmpty(c, id, n)
	})
	worker := sp.NewEvidenceWorker(store, source, func() time.Time { return now }, nil)
	before, err := worker.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 6, before.EligibleCards)
	require.Equal(t, 1, before.EligibleIdentities)
	require.Equal(t, 1, before.MissingIdentities)
	require.Equal(t, 2, before.UnresolvedCards)
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 1, calls)
	// Reload real stores: evidence lookup must use the canonical identity key,
	// rather than joining normalized inventory back to a differently padded row.
	worker = sp.NewEvidenceWorker(NewShowPrepWorkerStore(db.DB), source, func() time.Time { return now }, nil)
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 1, calls)
	after, err := worker.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, after.CurrentIdentities)
	require.Equal(t, 4, after.CurrentCards)
	require.Equal(t, 2, after.UnresolvedCards)
	require.Zero(t, after.FailedIdentities)
	cached := sp.NewService(NewShowPrepStore(db.DB), nil, func() time.Time { return now })
	for i := range profiles {
		evaluations, err := cached.Evaluate(ctx, []string{fmt.Sprintf("canonical-%d", i)})
		require.NoError(t, err)
		if i < 4 {
			require.Equal(t, sp.ReadinessCurrent, evaluations[0].Readiness.State)
		} else {
			require.Equal(t, sp.ReadinessUnavailable, evaluations[0].Readiness.State)
		}
	}
	var evidenceRows int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_evidence`).Scan(&evidenceRows))
	require.Equal(t, 1, evidenceRows)
	// A changed raw spelling must remain discoverable by Begin's fresh scope
	// recheck even though no row contains the plain canonical profile string.
	now = now.AddDate(0, 0, 1)
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id=$1 WHERE id='canonical-0'`, "\u2003\tpsa-1\n\u3000")
	require.NoError(t, err)
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 2, calls)
}
