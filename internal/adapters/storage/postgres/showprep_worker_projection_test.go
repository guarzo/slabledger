package postgres

import (
	"context"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowWorkerInventorySQLPrefiltersGraderAndGrade(t *testing.T) {
	db, _ := setupShowWorkerDB(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name, profile, grader string
		grade                 float64
	}{
		{"canonical", "psa-1", "PSA", 10},
		{"unicode", "\u2003\tpsa-1\n\u3000", "PSA", 10},
		{"other-profile", "other", "PSA", 10},
		{"other-grade", "psa-1", "PSA", 9.5},
		{"other-grader", "psa-1", "BGS", 10},
		{"other-both", "psa-1", "CGC", 9},
	} {
		seedShowPurchase(t, db, tc.name, "worker", tc.name, tc.grader)
		_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id=$1,grade_value=$2 WHERE id=$3`, tc.profile, tc.grade, tc.name)
		require.NoError(t, err)
	}
	for _, tc := range []struct {
		name           string
		only           *sp.Identity
		wantGroups     int
		wantCandidates int
		wantCards      int
	}{
		{"fleet", nil, 6, 5, 6},
		{"PSA ten", &sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}, 3, 1, 2},
		{"PSA fractional", &sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 9.5}, 1, 1, 1},
		{"BGS ten", &sp.Identity{ProfileID: "psa-1", Grader: "BGS", Grade: 10}, 1, 1, 1},
		{"absent grade", &sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 8}, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var query string
			var args []any
			q := &mocks.ShowPrepQueryHook{DB: db.DB, ObserveQuery: func(sql string, values []any) {
				query, args = sql, values
			}}
			candidates, err := showWorkerInventory(ctx, q, tc.only)
			require.NoError(t, err)
			require.Len(t, candidates, tc.wantCandidates)
			cards := 0
			for _, candidate := range candidates {
				cards += candidate.Cards
				if tc.only != nil {
					require.Equal(t, *tc.only, candidate.Identity)
				}
			}
			require.Equal(t, tc.wantCards, cards)
			// Replay the actual emitted SQL against real rows: output-only checks
			// cannot detect a full-fleet projection discarded later by Go.
			var groups int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM (`+query+`) projection`, args...).Scan(&groups))
			require.Equal(t, tc.wantGroups, groups, "SQL must exclude unrelated grader/grade groups, but retain raw profile spellings")
		})
	}
}
