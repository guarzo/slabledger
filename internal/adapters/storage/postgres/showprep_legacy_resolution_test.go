package postgres

import (
	"context"
	"fmt"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Real SQL remains responsible for selection. The hook only commits a canonical
// row after the exact miss and before the fallback query.
func TestShowLegacyConcurrentCanonicalWinsFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload any
		state   string
	}{
		{"success", legacyPayload, "complete"}, {"NULL", nil, "running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			alias := sp.Identity{ProfileID: " psa-1 ", Grader: "PSA", Grade: 10}
			seedLegacyRow(t, db, alias, `{"Sales":"bad"}`, "complete")
			seedLegacyRow(t, db, sp.Identity{ProfileID: "\tpsa-1", Grader: "PSA", Grade: 10}, legacyPayload, "complete")
			before := legacyRowJSON(t, db, alias)
			queries := 0
			q := &mocks.ShowPrepQueryHook{DB: db.DB, BeforeQuery: func() {
				queries++
				if queries == 2 {
					seedLegacyRow(t, db, legacyCanonical, tc.payload, tc.state)
				}
			}}
			snapshots, err := (&showPrepSession{q: q}).ReadSnapshots(context.Background(), []sp.Identity{legacyCanonical})
			require.NoError(t, err)
			require.Equal(t, 2, queries)
			require.NotNil(t, snapshots[legacyCanonical])
			require.Equal(t, tc.state, snapshots[legacyCanonical].AttemptState)
			require.Equal(t, int64(7), snapshots[legacyCanonical].Attempt)
			require.Equal(t, tc.payload != nil, snapshots[legacyCanonical].Complete)
			require.Equal(t, before, legacyRowJSON(t, db, alias))
		})
	}
}

func TestShowLegacyBatchBoundsAndExactRawMiss(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ids := make([]sp.Identity, 0, 40)
	for i := range 40 {
		id := sp.Identity{ProfileID: fmt.Sprintf("profile-%d", i), Grader: "PSA", Grade: 10}
		ids = append(ids, id)
		alias := id
		alias.ProfileID = "\u2003" + id.ProfileID + "\t"
		seedLegacyRow(t, db, alias, legacyPayload, "complete")
	}
	// Substring, grader and grade are not identity matches.
	for _, id := range []sp.Identity{{ProfileID: "prefix-profile-0", Grader: "PSA", Grade: 10}, {ProfileID: "profile-0-suffix", Grader: "PSA", Grade: 10}, {ProfileID: "\tprofile-0", Grader: "BGS", Grade: 10}, {ProfileID: "\tprofile-0", Grader: "PSA", Grade: 9.5}} {
		seedLegacyRow(t, db, id, `{"Sales":false}`, "failed")
	}
	queries := 0
	q := &mocks.ShowPrepQueryHook{DB: db.DB, BeforeQuery: func() { queries++ }}
	session := &showPrepSession{q: q}
	snapshots, err := session.ReadSnapshots(context.Background(), ids)
	require.NoError(t, err)
	require.Len(t, snapshots, 40)
	require.Equal(t, 2, queries)
	for _, id := range ids {
		require.True(t, snapshots[id].Complete)
	}
	seedLegacyRow(t, db, legacyCanonical, legacyPayload, "complete")
	queries = 0
	snapshots, err = session.ReadSnapshots(context.Background(), []sp.Identity{legacyCanonical})
	require.NoError(t, err)
	require.True(t, snapshots[legacyCanonical].Complete)
	require.Equal(t, 1, queries)
	queries = 0
	snapshots, err = session.ReadSnapshots(context.Background(), []sp.Identity{{ProfileID: " psa-1", Grader: "PSA", Grade: 10}})
	require.NoError(t, err)
	require.Empty(t, snapshots)
	require.Equal(t, 1, queries, "raw misses do not cross namespaces")
}

func TestShowLegacyMigration46PayloadRemainsReadableAfter47(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "legacy", "unique", "PSA")
	business := legacyBusinessJSON(t, db)
	up, err := MigrationsFS.ReadFile("migrations/000047_showprep_worker.up.sql")
	require.NoError(t, err)
	down, err := MigrationsFS.ReadFile("migrations/000047_showprep_worker.down.sql")
	require.NoError(t, err)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, string(down))
	require.NoError(t, err)
	alias := sp.Identity{ProfileID: "\u00a0psa-1\n", Grader: "PSA", Grade: 10}
	_, err = tx.ExecContext(ctx, `INSERT INTO showprep_evidence(identity_key,profile_id,grader,grade,payload,attempt,attempt_state,attempt_started_at) VALUES($1,$2,$3,$4,$5::jsonb,7,'complete',$6)`, alias.Key(), alias.ProfileID, alias.Grader, alias.Grade, legacyPayload, legacyNow)
	require.NoError(t, err)
	var original string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT payload::text FROM showprep_evidence`).Scan(&original))
	_, err = tx.ExecContext(ctx, string(up))
	require.NoError(t, err)
	session := &showPrepSession{q: tx}
	snapshots, err := session.ReadSnapshots(ctx, []sp.Identity{legacyCanonical})
	require.NoError(t, err)
	require.NotNil(t, snapshots[legacyCanonical])
	require.Equal(t, int64(7), snapshots[legacyCanonical].Attempt)
	// The real worker reads the same upgraded lineage and migration-default budget.
	candidates, err := showWorkerCandidates(ctx, tx, nil)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, snapshots[legacyCanonical], candidates[0].Snapshot)
	require.Zero(t, candidates[0].Attempts)
	require.Empty(t, candidates[0].RetryWindow)
	var after string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT payload::text FROM showprep_evidence WHERE identity_key=$1`, alias.Key()).Scan(&after))
	require.Equal(t, original, after)
	require.NoError(t, tx.Rollback())
	require.Equal(t, business, legacyBusinessJSON(t, db))
}
