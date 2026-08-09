package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three indexes 000039 removes, each paired with the index that already
// covers it. Both members are named so the test can assert the containment
// relationship against the live schema rather than trusting the reading of
// 000001 that motivated the migration.
var redundantIndexPairs = []struct {
	table     string
	dropped   string
	covers    string // the surviving index whose leading columns contain dropped's
	droppedOn []string
}{
	{
		table:     "card_access_log",
		dropped:   "idx_access_log_covering",
		covers:    "idx_access_log_card",
		droppedOn: []string{"card_name", "set_name", "card_number", "accessed_at"},
	},
	{
		table:     "dh_suggestions",
		dropped:   "idx_dh_suggestions_date",
		covers:    "dh_suggestions_pkey",
		droppedOn: []string{"suggestion_date"},
	},
	{
		table:     "campaign_purchases",
		dropped:   "idx_purchases_campaign",
		covers:    "idx_purchases_campaign_date",
		droppedOn: []string{"campaign_id"},
	},
}

// TestMigration000039_RedundantIndexesDropped is the acceptance criterion: after
// the full embedded migration set applies, the three redundant indexes are gone
// and the indexes that subsume them are still there.
//
// The survivors are asserted in the same test rather than a separate one for the
// same reason 000038 pinned api_usage_summary alongside its drops: "dropped the
// redundant index" and "kept the one that covers it" are one decision. Each pair
// sits on the same table over overlapping columns, so an assertion that only
// checked the absences would pass just as happily if the wrong member of the
// pair had been dropped -- the one mistake in this change that would stay
// invisible until a production query plan degraded.
//
// Uses indexColumns (migration_000032_test.go:70) as the existence probe: it
// returns nil for an index that is not there, and an index with zero key columns
// does not exist.
func TestMigration000039_RedundantIndexesDropped(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	for _, pair := range redundantIndexPairs {
		assert.Nil(t, indexColumns(ctx, t, db, pair.table, pair.dropped),
			"index %s should not exist after migration 000039", pair.dropped)

		require.NotNil(t, indexColumns(ctx, t, db, pair.table, pair.covers),
			"%s is what makes dropping %s safe and must survive 000039",
			pair.covers, pair.dropped)
	}

	// idx_dh_suggestions_card shares a table with a dropped index but has no
	// overlapping prefix with it, so it is not redundant and is not a target.
	// Named explicitly because "drop the redundant indexes on dh_suggestions"
	// is an easy over-reach.
	assert.NotNil(t, indexColumns(ctx, t, db, "dh_suggestions", "idx_dh_suggestions_card"),
		"idx_dh_suggestions_card is not redundant and must survive 000039")
}

// TestMigration000039_SurvivingIndexCoversDropped verifies the premise the
// migration rests on, against the live schema: for each pair, the columns of the
// dropped index are a leading prefix of the surviving index's columns.
//
// This is the assertion that would catch the migration being wrong. The
// containment argument was read out of 000001's DDL by hand; if a later
// migration quietly redefined one of the survivors -- none has, but nothing
// stops one -- the drop would remove coverage rather than remove duplication,
// and every absence assertion above would still pass.
//
// Sort direction is deliberately not compared. It is what makes
// idx_access_log_covering redundant rather than distinct: a btree is scannable
// in either direction at equal cost, so Postgres reads idx_access_log_card
// backwards to satisfy the ascending ordering the dropped index declared.
func TestMigration000039_SurvivingIndexCoversDropped(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	for _, pair := range redundantIndexPairs {
		cols := indexColumns(ctx, t, db, pair.table, pair.covers)
		require.GreaterOrEqual(t, len(cols), len(pair.droppedOn),
			"%s has fewer columns than the %s it is supposed to cover",
			pair.covers, pair.dropped)

		assert.Equal(t, pair.droppedOn, cols[:len(pair.droppedOn)],
			"%s must lead with exactly the columns %s indexed, or dropping %s "+
				"removes coverage instead of removing duplication",
			pair.covers, pair.dropped, pair.dropped)
	}
}

// TestMigration000039_DownRestoresIndexes pins what the rollback restores. The
// generic up/down/up roundtrip proves the SQL parses; it does not prove the
// indexes come back on the right columns.
//
// Rolls back to version 38 rather than one relative step. 000037 and 000038 are
// in flight on sibling branches, so whichever merges last needs renumbering by
// hand anyway -- and an absolute target that has to be edited in the same commit
// as the renumber is safer here than a relative step that silently retargets.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000039_DownRestoresIndexes(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Restore the package's shared database no matter how this test exits.
	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 38), "roll back migration 000039")

	for _, pair := range redundantIndexPairs {
		assert.Equal(t, pair.droppedOn,
			indexColumns(ctx, t, db, pair.table, pair.dropped),
			"000039's down migration should recreate %s on the columns 000001 declared",
			pair.dropped)
	}

	// Re-applying removes all three again, proving the rollback is not one-way.
	require.NoError(t, RunMigrations(db, ""), "re-apply migration 000039")
	for _, pair := range redundantIndexPairs {
		assert.Nil(t, indexColumns(ctx, t, db, pair.table, pair.dropped),
			"index %s should be gone again after re-applying 000039", pair.dropped)
	}
}
