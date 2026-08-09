package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dhPushStatusIndex = "idx_campaign_purchases_dh_push_status"

// The two fragments that distinguish 000001's partial index from the non-partial
// one 000003 tried to create under the same name. Checking the definition rather
// than just "an index by this name exists" is the point of these tests: the bug
// being fixed was a CREATE INDEX IF NOT EXISTS naming this index with a
// different definition and being silently discarded, because that construct
// matches on relation name alone. A name-only assertion would have passed
// throughout the entire lifetime of the bug.
//
// Matched as substrings, not as one pinned statement: pg_get_indexdef's exact
// spacing and schema qualification vary across Postgres versions, and this
// repository runs against both Supabase and a local server. These two fragments
// carry the whole distinction being asserted -- same column, still partial.
const (
	dhPushStatusIndexKey       = "btree (dh_push_status)"
	dhPushStatusIndexPredicate = "WHERE (dh_push_status <> ''::text)"
)

// requirePartialDHPushStatusIndex asserts the named index exists with 000001's
// partial definition, prefixing failures with where in the migration sequence
// the check ran.
func requirePartialDHPushStatusIndex(ctx context.Context, t *testing.T, db *DB, when string) {
	t.Helper()
	def := indexDef(ctx, t, db, dhPushStatusIndex)
	require.NotEmpty(t, def, "%s: index %s should exist", when, dhPushStatusIndex)
	assert.Contains(t, def, dhPushStatusIndexKey, "%s: index should key on dh_push_status", when)
	assert.Contains(t, def, dhPushStatusIndexPredicate,
		"%s: index should keep 000001's partial predicate, not the non-partial "+
			"definition 000003 tried to create under the same name", when)
}

// indexDef returns the reconstructed CREATE INDEX statement for a named index in
// the public schema, or "" when no such index exists.
func indexDef(ctx context.Context, t *testing.T, db *DB, index string) string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT pg_get_indexdef(c.oid)
		FROM pg_class     c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1 AND c.relkind = 'i'`, index)
	require.NoError(t, err, "query definition of index %s", index)
	defer func() { _ = rows.Close() }()

	var defs []string
	for rows.Next() {
		var def string
		require.NoError(t, rows.Scan(&def))
		defs = append(defs, def)
	}
	require.NoError(t, rows.Err())
	require.LessOrEqual(t, len(defs), 1,
		"index name %s is not unique in the public schema", index)
	if len(defs) == 0 {
		return ""
	}
	return defs[0]
}

// TestMigration000003_DHPushStatusIndexIsPartial is acceptance criterion 1: a
// database migrated to head carries exactly one index by this name, and it is
// the partial one 000001 declares.
//
// Uniqueness is enforced inside indexDef, not asserted separately, because "two
// indexes share this name" is not a state Postgres can reach -- the value of
// checking is that a duplicate would mean the query is matching something other
// than what it claims.
func TestMigration000003_DHPushStatusIndexIsPartial(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	requirePartialDHPushStatusIndex(ctx, t, db, "at head")
}

// TestMigration000003_RollbackPreservesDHPushStatusIndex is acceptance criterion
// 2, and the regression this ticket exists for: head -> rollback through 000003
// -> head must yield the same definition at every point where the index is
// supposed to exist.
//
// Before the fix, the middle step destroyed the index -- 000003's down dropped a
// name that 000001 owns -- and re-applying 000003 did not restore it, because
// its up migration was a no-op against an index that no longer existed. Any
// deployment that rolled back past 000003 silently lost the index for good.
//
// Rolls back to version 2 rather than one relative step from head: the target is
// "the state just before 000003 ran", which is a fixed point in history, whereas
// head moves with every migration added.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000003_RollbackPreservesDHPushStatusIndex(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Restore the package's shared database no matter how this test exits.
	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NotEmpty(t, indexDef(ctx, t, db, dhPushStatusIndex),
		"precondition: the index should exist at head")

	require.NoError(t, migrateToVersion(db, 2), "roll back to before 000003")

	requirePartialDHPushStatusIndex(ctx, t, db, "after rolling back 000003")

	require.NoError(t, RunMigrations(db, ""), "re-apply to head")

	requirePartialDHPushStatusIndex(ctx, t, db, "after re-applying to head")
}
