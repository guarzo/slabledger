package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

// Break caught: a legacy miss resets persisted backoff/exhaustion/reset epoch,
// or a complete zero/stale success is classified differently by the worker.
func TestShowLegacyWorkerInheritsScheduling(t *testing.T) {
	for _, tc := range []struct {
		name, state, payload, window string
		attempts                     int
		delay                        time.Duration
		retry                        bool
		epoch                        int64
		wantCalls                    int
		wantClass                    string
		wantCount                    int
	}{
		{"current", "complete", legacyPayload, "2026-09-15", 2, time.Hour, false, 19, 0, "current", 2},
		{"current-zero", "complete", `{"Source":"cardladder","Generation":5,"Complete":true,"Sales":[],"WindowStart":"2026-08-17","WindowEnd":"2026-09-15","RefreshedAt":"2026-09-15T11:00:00Z"}`, "2026-09-15", 2, time.Hour, false, 19, 0, "current", 2},
		{"backoff", "failed", legacyPayload, "2026-09-15", 2, time.Hour, false, 19, 0, "failed", 2},
		{"exhausted", "failed", legacyPayload, "2026-09-15", 3, -time.Hour, false, 19, 0, "failed", 3},
		{"due retains budget", "failed", legacyPayload, "2026-09-15", 2, -time.Hour, false, 19, 1, "failed", 3},
		{"stale next window", "complete", `{"Source":"cardladder","Generation":5,"Complete":true,"Sales":[],"WindowStart":"2026-08-16","WindowEnd":"2026-09-14","RefreshedAt":"2026-09-14T11:00:00Z"}`, "2026-09-14", 3, time.Hour, false, 19, 1, "stale", 1},
		{"retry resets once", "failed", legacyPayload, "2026-09-15", 3, time.Hour, true, 19, 1, "failed", 1},
		{"same epoch already consumed", "failed", legacyPayload, "2026-09-15", 3, time.Hour, true, 2, 0, "failed", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, workers := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "legacy", "unique", "PSA")
			alias := sp.Identity{ProfileID: "\u2003psa-1\n", Grader: " psa ", Grade: 10}
			seedLegacyRow(t, db, alias, tc.payload, tc.state)
			_, err := db.Exec(`UPDATE showprep_evidence SET retry_window=$1,retry_attempts=$2,retry_not_before=$3,retry_reset_epoch=$4`, tc.window, tc.attempts, legacyNow.Add(tc.delay), tc.epoch)
			require.NoError(t, err)
			before, business := legacyRowJSON(t, db, alias), legacyBusinessJSON(t, db)
			candidates, err := workers.Candidates(ctx)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			c := candidates[0]
			require.Equal(t, tc.wantClass, c.Classification(legacyNow))
			require.Equal(t, tc.attempts, c.Attempts)
			require.Equal(t, tc.window, c.RetryWindow)
			require.Equal(t, legacyNow.Add(tc.delay), c.NotBefore.UTC())
			require.Equal(t, tc.epoch, c.ResetEpoch)
			calls := 0
			worker := sp.NewEvidenceWorker(workers, workerSource(func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) { calls++; return sp.Snapshot{}, nil }), func() time.Time { return legacyNow }, nil)
			if tc.retry {
				require.NoError(t, worker.RequestRun(ctx, true))
			}
			err = worker.RunOnce(ctx)
			if tc.wantCalls > 0 {
				require.ErrorIs(t, err, sp.ErrWorkerFailed)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantCalls, calls)
			candidates, err = workers.Candidates(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.wantCount, candidates[0].Attempts)
			if tc.wantCalls == 0 {
				requireNoCanonical(t, db)
			} else {
				require.Equal(t, int64(8), candidates[0].Snapshot.Attempt)
				require.Equal(t, int64(5), candidates[0].Snapshot.Generation)
				require.Equal(t, c.Snapshot.Sales, candidates[0].Snapshot.Sales)
				wantEpoch := tc.epoch
				if tc.retry {
					wantEpoch = 2
				}
				require.Equal(t, wantEpoch, candidates[0].ResetEpoch)
				require.Equal(t, "2026-09-15", candidates[0].RetryWindow)
				delay := time.Hour
				if tc.wantCount == 1 {
					delay = 15 * time.Minute
				}
				require.Equal(t, legacyNow.Add(delay), candidates[0].NotBefore.UTC())
			}
			require.Equal(t, before, legacyRowJSON(t, db, alias))
			require.Equal(t, business, legacyBusinessJSON(t, db))
		})
	}
}

func TestShowLegacyRawFinishAndConcurrentCanonicalBegins(t *testing.T) {
	for _, when := range []string{"before adoption", "after adoption"} {
		t.Run(when, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			alias := sp.Identity{ProfileID: " psa-1 ", Grader: "PSA", Grade: 10}
			seedLegacyRow(t, db, alias, legacyPayload, "running")
			store := NewShowPrepStore(db.DB)
			old := sp.Snapshot{Source: "cardladder", Complete: true, Sales: []sp.Sale{{ID: "old-completion", Date: "2026-09-15", PriceCents: 31000}}}
			if when == "before adoption" {
				require.NoError(t, store.FinishAttempt(ctx, alias, 7, old))
			}
			original := legacyRowJSON(t, db, alias)
			type result struct {
				n   int64
				err error
			}
			done := make(chan result, 2)
			for range 2 {
				go func() { n, err := store.BeginAttempt(ctx, legacyCanonical, legacyNow); done <- result{n, err} }()
			}
			a, b := <-done, <-done
			require.NoError(t, a.err)
			require.NoError(t, b.err)
			require.ElementsMatch(t, []int64{8, 9}, []int64{a.n, b.n})
			canonical := legacyRowJSON(t, db, legacyCanonical)
			require.Equal(t, original, legacyRowJSON(t, db, alias))
			require.NoError(t, store.FinishAttempt(ctx, legacyCanonical, 8, sp.Snapshot{Complete: true, Sales: []sp.Sale{{ID: "obsolete"}}}))
			require.Equal(t, canonical, legacyRowJSON(t, db, legacyCanonical), "obsolete canonical Finish ignored")
			if when == "after adoption" {
				require.NoError(t, store.FinishAttempt(ctx, alias, 7, old))
				// Even a numeric attempt equal to the canonical attempt cannot redirect.
				n, err := store.BeginAttempt(ctx, alias, legacyNow)
				require.NoError(t, err)
				require.Equal(t, int64(8), n)
				n, err = store.BeginAttempt(ctx, alias, legacyNow)
				require.NoError(t, err)
				require.Equal(t, int64(9), n)
				require.NoError(t, store.FinishAttempt(ctx, alias, n, old))
				require.Equal(t, canonical, legacyRowJSON(t, db, legacyCanonical))
			}
			snaps, err := store.ReadSnapshots(ctx, []sp.Identity{legacyCanonical, alias})
			require.NoError(t, err)
			require.Equal(t, alias, snaps[alias].Identity)
			require.Equal(t, "old-completion", snaps[alias].Sales[0].ID)
			want := "one"
			if when == "before adoption" {
				want = "old-completion"
			}
			require.Equal(t, want, snaps[legacyCanonical].Sales[0].ID)
		})
	}
}

func TestShowLegacyAdoptionRollbackAndLeaseFences(t *testing.T) {
	for _, tc := range []string{"transaction rejection", "lease already lost", "lease expires during adoption"} {
		t.Run(tc, func(t *testing.T) {
			db, workers := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "legacy", "unique", "PSA")
			alias := sp.Identity{ProfileID: " psa-1 ", Grader: "PSA", Grade: 10}
			seedLegacyRow(t, db, alias, legacyPayload, "failed")
			_, err := db.Exec(`UPDATE showprep_evidence SET retry_not_before=$1`, legacyNow.Add(-time.Hour))
			require.NoError(t, err)
			before, business := legacyRowJSON(t, db, alias), legacyBusinessJSON(t, db)
			if tc == "transaction rejection" {
				err = NewShowPrepStore(db.DB).transaction(ctx, func(tx *showPrepSession) error {
					n, err := tx.beginEvidence(ctx, legacyCanonical, legacyNow)
					require.NoError(t, err)
					require.Equal(t, int64(8), n)
					return sp.ErrConflict
				})
				require.ErrorIs(t, err, sp.ErrConflict)
			} else {
				lease, ok, err := workers.Acquire(ctx, "adoption")
				require.NoError(t, err)
				require.True(t, ok)
				if tc == "lease already lost" {
					require.NoError(t, workers.CredentialsChanged(ctx))
				} else {
					_, err = db.Exec(`CREATE FUNCTION slow_legacy_adoption() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN NEW; END $$;
 CREATE TRIGGER slow_legacy_adoption BEFORE INSERT ON showprep_evidence FOR EACH ROW EXECUTE FUNCTION slow_legacy_adoption();
 UPDATE showprep_worker SET lease_until=clock_timestamp()+interval '100 milliseconds'`)
					require.NoError(t, err)
					t.Cleanup(func() {
						_, err := db.Exec(`DROP TRIGGER slow_legacy_adoption ON showprep_evidence; DROP FUNCTION slow_legacy_adoption()`)
						require.NoError(t, err)
					})
				}
				_, err = workers.Begin(ctx, lease, legacyCanonical, legacyNow)
				require.ErrorIs(t, err, sp.ErrWorkerLeaseLost)
			}
			requireNoCanonical(t, db)
			require.Equal(t, before, legacyRowJSON(t, db, alias))
			require.Equal(t, business, legacyBusinessJSON(t, db))
		})
	}
}
