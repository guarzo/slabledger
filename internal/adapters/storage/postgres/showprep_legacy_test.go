package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

var legacyNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
var legacyCanonical = sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}

// Insert old-writer rows directly, never through the new Begin/adoption path.
// The unknown nested field and old payload identity must survive SQL adoption.
const legacyPayload = `{"Identity":{"ProfileID":" psa-1 ","Grader":"PSA","Grade":10},"Source":"cardladder","Generation":5,"WindowStart":"2026-08-17","WindowEnd":"2026-09-15","RefreshedAt":"2026-09-15T11:00:00Z","Complete":true,"Sales":[{"id":"one","date":"2026-09-15","priceCents":27000},{"id":"two","date":"2026-09-15","priceCents":29000}],"future":{"nested":[1,{"opaque":"preserve"}]}}`

func seedLegacyRow(t *testing.T, db *DB, id sp.Identity, payload any, state string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `INSERT INTO showprep_evidence
 (identity_key,profile_id,grader,grade,payload,attempt,attempt_state,attempt_error,attempt_started_at,retry_window,retry_attempts,retry_not_before,retry_reset_epoch)
 VALUES($1,$2,$3,$4,$5::jsonb,7,$6,'', $7,'2026-09-15',2,$8,19)`, id.Key(), id.ProfileID, id.Grader, id.Grade, payload, state, legacyNow.Add(-time.Hour), legacyNow.Add(time.Hour))
	require.NoError(t, err)
}
func legacyRowJSON(t *testing.T, db *DB, id sp.Identity) string {
	t.Helper()
	var result string
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT row_to_json(e)::text FROM showprep_evidence e WHERE identity_key=$1`, id.Key()).Scan(&result))
	return result
}
func legacyBusinessJSON(t *testing.T, db *DB) string {
	t.Helper()
	var result string
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT jsonb_build_object(
 'purchases',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM campaign_purchases p),
 'campaigns',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM campaigns c),
 'sales',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM campaign_sales s),
 'holds',(SELECT jsonb_agg(to_jsonb(h) ORDER BY purchase_id) FROM showprep_price_holds h))::text`).Scan(&result))
	return result
}
func requireNoCanonical(t *testing.T, db *DB) {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM showprep_evidence WHERE identity_key=$1`, legacyCanonical.Key()).Scan(&n))
	require.Zero(t, n)
}

// Break caught: exact-key-only readers lose upgrade evidence; normalizing by SQL
// btrim or today's purchase spelling misses Unicode/old-only spellings.
func TestShowLegacyReadProjectionAndLockedVersions(t *testing.T) {
	for _, tc := range []struct{ name, profile, grader string }{
		{"ASCII", " psa-1 ", "PSA"},
		{"tabs-newlines", "\tpsa-1\n", "\tpsa\n"},
		{"Unicode", "\u00a0\u2003psa-1\u202f", "\u2003pSa\u00a0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, workerStore := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "legacy", "unique", "PSA")
			alias := sp.Identity{ProfileID: tc.profile, Grader: tc.grader, Grade: 10}
			seedLegacyRow(t, db, alias, legacyPayload, "complete")
			before, business := legacyRowJSON(t, db, alias), legacyBusinessJSON(t, db)
			store := NewShowPrepStore(db.DB)
			snapshots, err := store.ReadSnapshots(ctx, []sp.Identity{legacyCanonical, alias, {ProfileID: "psa-1", Grader: "BGS", Grade: 10}, {ProfileID: "psa-1", Grader: "PSA", Grade: 9.5}, {ProfileID: "PSA-1", Grader: "PSA", Grade: 10}})
			require.NoError(t, err)
			require.Len(t, snapshots, 2)
			require.NotNil(t, snapshots[legacyCanonical])
			require.Equal(t, alias, snapshots[alias].Identity, "raw exact read remains raw")
			require.Equal(t, legacyCanonical, snapshots[legacyCanonical].Identity)
			require.Equal(t, int64(5), snapshots[legacyCanonical].Generation)
			require.Equal(t, int64(7), snapshots[legacyCanonical].Attempt)
			candidates, err := workerStore.Candidates(ctx)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			require.Equal(t, snapshots[legacyCanonical], candidates[0].Snapshot)
			require.Equal(t, "current", candidates[0].Classification(legacyNow))
			calls := 0
			source := &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) { calls++; return sp.Snapshot{}, nil }}
			for _, src := range []sp.Source{nil, source} {
				svc := sp.NewService(store, src, func() time.Time { return legacyNow })
				es, err := svc.Evaluate(ctx, []string{showPurchase})
				require.NoError(t, err)
				require.Equal(t, sp.Supported, es[0].Status)
				require.Equal(t, 28000, es[0].MedianCents)
				detail, err := svc.Evidence(ctx, showPurchase)
				require.NoError(t, err)
				require.Equal(t, es[0], detail.Evaluation)
				require.Len(t, detail.Sales, 2)
				_, err = svc.CreateList(ctx, showList, "Legacy")
				require.NoError(t, err)
				list, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
				require.NoError(t, err, "locked evaluation must have the same fingerprint")
				require.Equal(t, es[0], list.Items[0].Evaluation)
				yes := true
				list, err = svc.UpdateItem(ctx, showList, list.Items[0].ID, sp.UpdateItem{Version: list.Items[0].Version, EvaluationVersion: es[0].Version, Packed: &yes})
				require.NoError(t, err)
				stored, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				_, err = svc.ListDetail(ctx, showList)
				require.NoError(t, err)
				after, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				require.Equal(t, stored, after, "reads preserve packing history")
			}
			require.Zero(t, calls)
			require.Equal(t, business, legacyBusinessJSON(t, db))
			require.Equal(t, before, legacyRowJSON(t, db, alias))
			requireNoCanonical(t, db)
		})
	}
}

// Break caught: selecting by payload quality/timestamp overrides row authority,
// or an undecodable losing alias poisons a valid canonical/batch read.
func TestShowLegacyCanonicalRowAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		payload     any
		malformed   bool
	}{
		{"complete", "complete", legacyPayload, false},
		{"failed", "failed", legacyPayload, false},
		{"running", "running", legacyPayload, false},
		{"NULL", "running", nil, false},
		{"malformed", "failed", `{"Sales":"not-an-array"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, workers := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "legacy", "unique", "PSA")
			alias := sp.Identity{ProfileID: " psa-1 ", Grader: "PSA", Grade: 10}
			seedLegacyRow(t, db, alias, `{"Sales":"malformed losing alias"}`, "complete")
			goodAlias := sp.Identity{ProfileID: "\tpsa-1\n", Grader: "PSA", Grade: 10}
			seedLegacyRow(t, db, goodAlias, legacyPayload, "complete")
			goodBefore := legacyRowJSON(t, db, goodAlias)
			seedLegacyRow(t, db, legacyCanonical, tc.payload, tc.state)
			_, err := db.Exec(`UPDATE showprep_evidence SET attempt=2,retry_attempts=1 WHERE identity_key=$1`, legacyCanonical.Key())
			require.NoError(t, err)
			before := legacyRowJSON(t, db, alias)
			store := NewShowPrepStore(db.DB)
			snapshots, err := store.ReadSnapshots(ctx, []sp.Identity{legacyCanonical, {ProfileID: "missing", Grader: "PSA", Grade: 10}})
			if tc.malformed {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(2), snapshots[legacyCanonical].Attempt)
				require.Equal(t, tc.state, snapshots[legacyCanonical].AttemptState)
				candidates, err := workers.Candidates(ctx)
				require.NoError(t, err)
				require.Equal(t, snapshots[legacyCanonical], candidates[0].Snapshot)
				require.Equal(t, 1, candidates[0].Attempts)
				if tc.payload == nil {
					require.False(t, snapshots[legacyCanonical].Complete)
					require.Empty(t, snapshots[legacyCanonical].Sales)
				}
			}
			n, err := store.BeginAttempt(ctx, legacyCanonical, legacyNow)
			require.NoError(t, err, "Begin selects rows without decoding payloads")
			require.Equal(t, int64(3), n)
			require.Equal(t, before, legacyRowJSON(t, db, alias))
			require.Equal(t, goodBefore, legacyRowJSON(t, db, goodAlias))
		})
	}
}

func TestShowLegacyAmbiguityFailsClosedPerIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		payload     any
	}{
		{"identical", "complete", legacyPayload}, {"empty", "running", nil},
		{"failed", "failed", `{}`}, {"malformed", "failed", `{"Sales":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, workers := setupShowWorkerDB(t)
			ctx := context.Background()
			seedShowPurchase(t, db, showPurchase, "legacy", "one", "PSA")
			seedShowPurchase(t, db, showOther, "legacy", "two", "PSA")
			_, err := db.Exec(`UPDATE campaign_purchases SET gem_rate_id='other' WHERE id=$1`, showOther)
			require.NoError(t, err)
			a := sp.Identity{ProfileID: " psa-1 ", Grader: "PSA", Grade: 10}
			b := sp.Identity{ProfileID: "\tpsa-1\n", Grader: "PSA", Grade: 10}
			other := sp.Identity{ProfileID: "other", Grader: "PSA", Grade: 10}
			seedLegacyRow(t, db, a, legacyPayload, "complete")
			seedLegacyRow(t, db, b, tc.payload, tc.state)
			seedLegacyRow(t, db, other, legacyPayload, "complete")
			beforeA, beforeB, business := legacyRowJSON(t, db, a), legacyRowJSON(t, db, b), legacyBusinessJSON(t, db)
			store := NewShowPrepStore(db.DB)
			snapshots, err := store.ReadSnapshots(ctx, []sp.Identity{legacyCanonical, other})
			require.NoError(t, err)
			require.NotNil(t, snapshots[legacyCanonical])
			require.Equal(t, &sp.Snapshot{Identity: legacyCanonical, Sales: []sp.Sale{}, AttemptState: "failed", AttemptError: "Ambiguous legacy evidence identity"}, snapshots[legacyCanonical])
			require.True(t, snapshots[other].Complete)
			calls := 0
			source := workerSource(func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) { calls++; return sp.Snapshot{}, nil })
			worker := sp.NewEvidenceWorker(workers, source, func() time.Time { return legacyNow }, nil)
			status, err := worker.Status(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, status.FailedIdentities)
			require.Equal(t, 1, status.CurrentIdentities)
			require.NoError(t, worker.RunOnce(ctx))
			require.NoError(t, worker.RequestRun(ctx, true))
			require.NoError(t, worker.RunOnce(ctx), "explicit retry cannot pick a lineage")
			lease, ok, err := workers.Acquire(ctx, "ambiguity")
			require.NoError(t, err)
			require.True(t, ok)
			_, err = workers.Begin(ctx, lease, legacyCanonical, legacyNow)
			require.ErrorIs(t, err, sp.ErrConflict)
			_, err = store.BeginAttempt(ctx, legacyCanonical, legacyNow)
			require.ErrorIs(t, err, sp.ErrConflict)
			require.Zero(t, calls)
			svc := sp.NewService(store, nil, func() time.Time { return legacyNow })
			es, err := svc.Evaluate(ctx, []string{showPurchase, showOther})
			require.NoError(t, err)
			require.Equal(t, "Ambiguous legacy evidence identity", es[0].EvidenceReason)
			require.True(t, es[0].CanPack, "weak evidence does not ban physical packing")
			require.Equal(t, sp.Supported, es[1].Status)
			_, err = svc.CreateList(ctx, showList, "Ambiguous")
			require.NoError(t, err)
			list, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
			require.NoError(t, err)
			yes := true
			_, err = svc.UpdateItem(ctx, showList, list.Items[0].ID, sp.UpdateItem{Version: 1, EvaluationVersion: es[0].Version, Packed: &yes})
			require.NoError(t, err)
			require.Equal(t, beforeA, legacyRowJSON(t, db, a))
			require.Equal(t, beforeB, legacyRowJSON(t, db, b))
			require.Equal(t, business, legacyBusinessJSON(t, db))
			requireNoCanonical(t, db)
		})
	}
}

func TestShowLegacyBeginAdoptsRawPayloadAndRetryMetadata(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	alias := sp.Identity{ProfileID: " psa-1 ", Grader: "PSA", Grade: 10}
	seedLegacyRow(t, db, alias, legacyPayload, "failed")
	before := legacyRowJSON(t, db, alias)
	store := NewShowPrepStore(db.DB)
	n, err := store.BeginAttempt(ctx, legacyCanonical, legacyNow)
	require.NoError(t, err)
	require.Equal(t, int64(8), n, "continue one lineage, never reset or combine generations")
	var same bool
	require.NoError(t, db.QueryRow(`SELECT c.payload=l.payload AND c.retry_window=l.retry_window AND c.retry_attempts=l.retry_attempts AND c.retry_not_before=l.retry_not_before AND c.retry_reset_epoch=l.retry_reset_epoch FROM showprep_evidence c,showprep_evidence l WHERE c.identity_key=$1 AND l.identity_key=$2`, legacyCanonical.Key(), alias.Key()).Scan(&same))
	require.True(t, same, "copy raw JSONB and all four retry fields")
	snaps, err := store.ReadSnapshots(ctx, []sp.Identity{legacyCanonical})
	require.NoError(t, err)
	require.Equal(t, "running", snaps[legacyCanonical].AttemptState)
	require.Equal(t, int64(5), snaps[legacyCanonical].Generation)
	require.Len(t, snaps[legacyCanonical].Sales, 2)
	require.NoError(t, store.FinishAttempt(ctx, legacyCanonical, n, sp.Snapshot{AttemptError: "refresh failed"}))
	snaps, err = store.ReadSnapshots(ctx, []sp.Identity{legacyCanonical})
	require.NoError(t, err)
	require.Equal(t, "failed", snaps[legacyCanonical].AttemptState)
	require.Equal(t, int64(5), snaps[legacyCanonical].Generation)
	require.Len(t, snaps[legacyCanonical].Sales, 2)
	var saved map[string]any
	require.NoError(t, json.Unmarshal([]byte(legacyRowJSON(t, db, legacyCanonical)), &saved))
	require.Contains(t, saved["payload"], "future")
	require.Equal(t, before, legacyRowJSON(t, db, alias))
}
