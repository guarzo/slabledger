package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The four objects 000035 removes: the two AI/scoring tables and the two views
// over ai_calls that 000003 created WITH (security_invoker = true).
//
// The views are the reason the up migration cannot be two DROP TABLE lines.
// Postgres refuses to drop a table a view selects from, and DROP TABLE CASCADE
// would take whatever else happens to be attached at deploy time — so the views
// are dropped explicitly, first, by name.
const (
	aiCallsTable         = "ai_calls"
	scoringDataGapsTable = "scoring_data_gaps"
	aiUsageSummaryView   = "ai_usage_summary"
	aiUsageByOpView      = "ai_usage_by_operation"
)

// TestMigration000035_ObjectsDropped is the acceptance criterion: after the full
// embedded migration set applies, none of the four objects exist.
//
// tableExists and viewExists are separate probes on purpose — see viewExists.
func TestMigration000035_ObjectsDropped(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	assert.False(t, tableExists(ctx, t, db, aiCallsTable),
		"%s should not exist after migration 000035", aiCallsTable)
	assert.False(t, tableExists(ctx, t, db, scoringDataGapsTable),
		"%s should not exist after migration 000035", scoringDataGapsTable)
	assert.False(t, viewExists(ctx, t, db, aiUsageSummaryView),
		"view %s should not exist after migration 000035", aiUsageSummaryView)
	assert.False(t, viewExists(ctx, t, db, aiUsageByOpView),
		"view %s should not exist after migration 000035", aiUsageByOpView)
}

// TestMigration000035_DownRestoresStructure pins the *shape* the rollback
// restores, which the generic up/down/up roundtrip in migrations_test.go cannot:
// that test proves the SQL is valid, not that it puts the schema back.
//
// Three things are worth pinning here:
//
//  1. The views come back at all. A down migration that restored only the two
//     tables would leave ai_usage_summary and ai_usage_by_operation gone
//     forever, and nothing else in the migration set recreates them.
//  2. The RLS shape is the post-000028 one (TO service_role, anon and
//     authenticated revoked), not 000003's TO PUBLIC original. 000028 runs
//     before this migration; restoring the looser statement would reopen the
//     hole 000028 closed.
//  3. The views are revoked too. 000028 listed them among its targets because
//     Supabase's default privileges grant on views as well as tables, so a
//     recreated view is readable by anon unless it is revoked again.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000035_DownRestoresStructure(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Restore the package's shared database no matter how this test exits.
	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 34), "roll back to version 34")

	require.True(t, tableExists(ctx, t, db, aiCallsTable),
		"000035's down migration should recreate %s", aiCallsTable)
	require.True(t, tableExists(ctx, t, db, scoringDataGapsTable),
		"000035's down migration should recreate %s", scoringDataGapsTable)
	require.True(t, viewExists(ctx, t, db, aiUsageSummaryView),
		"000035's down migration should recreate view %s", aiUsageSummaryView)
	require.True(t, viewExists(ctx, t, db, aiUsageByOpView),
		"000035's down migration should recreate view %s", aiUsageByOpView)

	// Columns come back with the original 000001 definitions.
	assert.Equal(t,
		map[string]string{
			"id":                  "bigint",
			"operation":           "text",
			"status":              "text",
			"error_message":       "text",
			"latency_ms":          "bigint",
			"tool_rounds":         "bigint",
			"input_tokens":        "bigint",
			"output_tokens":       "bigint",
			"total_tokens":        "bigint",
			"timestamp":           "timestamp without time zone",
			"cost_estimate_cents": "bigint",
		},
		tableColumns(ctx, t, db, aiCallsTable),
		"restored %s should match its 000001 column definition", aiCallsTable)

	assert.Equal(t,
		map[string]string{
			"id":          "bigint",
			"factor_name": "text",
			"reason":      "text",
			"entity_type": "text",
			"entity_id":   "text",
			"card_name":   "text",
			"set_name":    "text",
			"recorded_at": "timestamp without time zone",
		},
		tableColumns(ctx, t, db, scoringDataGapsTable),
		"restored %s should match its 000001 column definition", scoringDataGapsTable)

	// Indexes come back in the post-000003 set, not the 000001 set. 000003's
	// Fix 5 dropped idx_ai_calls_timestamp, idx_ai_calls_operation and
	// idx_scoring_gaps_factor as unused; recreating them here would leave the
	// rolled-back database with three indexes head does not have.
	assert.Empty(t, tableIndexes(ctx, t, db, aiCallsTable),
		"restored %s should carry no secondary indexes (000003 dropped both)", aiCallsTable)
	assert.Equal(t, []string{"idx_scoring_gaps_recorded"},
		tableIndexes(ctx, t, db, scoringDataGapsTable),
		"restored %s should carry only the index that survived 000003", scoringDataGapsTable)

	// The views must select through to the restored tables, not merely exist.
	for _, view := range []string{aiUsageSummaryView, aiUsageByOpView} {
		var n int64
		require.NoError(t,
			db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+view).Scan(&n),
			"restored view %s should be selectable", view)
	}

	// RLS comes back in the post-000028 shape on both tables.
	for _, table := range []string{aiCallsTable, scoringDataGapsTable} {
		var enabled bool
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT c.relrowsecurity
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1`, table).Scan(&enabled),
			"read relrowsecurity for %s", table)
		assert.True(t, enabled, "restored %s should have RLS enabled", table)

		rows, err := db.QueryContext(ctx, `
			SELECT policyname, roles::text
			FROM pg_policies
			WHERE schemaname = 'public' AND tablename = $1
			ORDER BY policyname`, table)
		require.NoError(t, err, "query policies for %s", table)

		var policies int
		for rows.Next() {
			var policy, roles string
			require.NoError(t, rows.Scan(&policy, &roles))
			policies++
			assert.Equal(t, "{service_role}", roles,
				"policy %q on restored %s must be scoped to service_role alone; %s "+
					"passes every role listed", policy, table, roles)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())

		// Where service_role is absent (a local or CI Postgres) the down
		// migration creates no policy at all: RLS enabled with zero policies is
		// the same effective deny, matching 000029 and 000030 off Supabase.
		if roleExists(ctx, t, db, "service_role") {
			assert.Equal(t, 1, policies,
				"restored %s should carry exactly one TO service_role policy", table)
		} else {
			assert.Zero(t, policies,
				"restored %s should have no policies where service_role is absent", table)
		}
	}

	// anon and authenticated hold no privilege on any of the four objects,
	// including the two views — 000028 revoked on views by name for exactly
	// this reason, and the down migration has to repeat it.
	//
	// Uses has_table_privilege rather than information_schema.role_table_grants,
	// matching migration_000027_test.go:108-141 and for the reason documented
	// there: role_table_grants lists only *direct* grants, so a privilege
	// reaching anon through a grant to PUBLIC or through role inheritance
	// slips past it and the assertion passes while the access is still live.
	// has_table_privilege answers the effective-access question this test
	// claims to be checking.
	tablePrivileges := []string{
		"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER",
	}
	for _, role := range []string{"anon", "authenticated"} {
		if !roleExists(ctx, t, db, role) {
			continue
		}
		for _, object := range []string{
			aiCallsTable, scoringDataGapsTable, aiUsageSummaryView, aiUsageByOpView,
		} {
			for _, privilege := range tablePrivileges {
				var allowed bool
				require.NoError(t, db.QueryRowContext(ctx,
					`SELECT has_table_privilege($1::text, format('public.%I', $2::text), $3::text)`,
					role, object, privilege).Scan(&allowed),
					"probe %s on %s for %s", privilege, object, role)
				assert.False(t, allowed,
					"restored %s must not be reachable by %s (effective %s remains; "+
						"check for grants to PUBLIC or an inherited role, not just direct grants)",
					object, role, privilege)
			}
		}
	}

	// Re-applying 000035 removes all four again, proving the rollback is not
	// one-way and that the view drops still precede the table drops.
	require.NoError(t, migrateToVersion(db, 35), "re-apply version 35")
	assert.False(t, tableExists(ctx, t, db, aiCallsTable),
		"%s should be gone again after re-applying 000035", aiCallsTable)
	assert.False(t, tableExists(ctx, t, db, scoringDataGapsTable),
		"%s should be gone again after re-applying 000035", scoringDataGapsTable)
	assert.False(t, viewExists(ctx, t, db, aiUsageSummaryView),
		"view %s should be gone again after re-applying 000035", aiUsageSummaryView)
	assert.False(t, viewExists(ctx, t, db, aiUsageByOpView),
		"view %s should be gone again after re-applying 000035", aiUsageByOpView)
}

// viewExists reports whether a VIEW of that name exists in public.
//
// This is NOT redundant with tableExists. tableExists filters relkind = 'r'
// (ordinary table), so it returns false for a view — every "the view is gone"
// assertion written against it would pass vacuously, including against a
// migration that never dropped the views at all. That is precisely the bug this
// task exists to avoid, so the view probe has to filter relkind = 'v'.
func viewExists(ctx context.Context, t *testing.T, db *DB, view string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1 AND c.relkind = 'v'
		)`, view).Scan(&exists), "probe existence of view %s", view)
	return exists
}

// tableIndexes returns the non-constraint index names on a public table, sorted.
// Primary-key and unique-constraint indexes are excluded so the assertion reads
// as "which secondary indexes survive", which is what 000003's Fix 5 changed.
func tableIndexes(ctx context.Context, t *testing.T, db *DB, table string) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT ci.relname
		FROM pg_index i
		JOIN pg_class ct ON ct.oid = i.indrelid
		JOIN pg_class ci ON ci.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = ct.relnamespace
		WHERE n.nspname = 'public'
		  AND ct.relname = $1
		  AND NOT i.indisprimary
		  AND NOT i.indisunique
		ORDER BY ci.relname`, table)
	require.NoError(t, err, "read indexes for %s", table)
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}
