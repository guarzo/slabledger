package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// marketMoversConfigTable is the last surviving piece of the Market Movers
// integration. 000021 retired MM and dropped mm_sales_comps and mm_card_mappings
// by name, but missed this one; 000030 finishes the job (SLA-20).
const marketMoversConfigTable = "marketmovers_config"

// TestMigration000030_TableDropped is the acceptance criterion: after the full
// embedded migration set applies, the table is gone.
//
// Asserted against pg_class rather than a SELECT, because a SELECT failing is
// indistinguishable from a connection problem, and because the table could
// linger with RLS denying reads — which would look identical from the client.
func TestMigration000030_TableDropped(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	assert.False(t, tableExists(ctx, t, db, marketMoversConfigTable),
		"%s should not exist after migration 000030", marketMoversConfigTable)
}

// TestMigration000030_DownRestoresStructure exercises the rollback path that the
// generic up/down/up roundtrip proves is *valid SQL* but says nothing about the
// *shape* it restores.
//
// The shape matters here for one specific reason: 000003 created this table's
// policy with no TO clause, which defaults to TO PUBLIC and so admits anon and
// authenticated. 000028 tightened it to TO service_role. Because 000030 runs
// after 000028, a down migration that naively restored 000003's original
// statement would silently reopen the hole 000028 closed — on a table holding a
// credential column. This test pins that it does not.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000030_DownRestoresStructure(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Restore the package's shared database no matter how this test exits.
	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 29), "roll back to version 29")
	require.True(t, tableExists(ctx, t, db, marketMoversConfigTable),
		"000030's down migration should recreate %s", marketMoversConfigTable)

	// Columns come back with the original 000001 definition.
	assert.Equal(t,
		map[string]string{
			"id":                      "bigint",
			"username":                "text",
			"encrypted_refresh_token": "text",
			"updated_at":              "text",
		},
		tableColumns(ctx, t, db, marketMoversConfigTable),
		"restored %s should match its 000001 column definition", marketMoversConfigTable)

	// RLS comes back in the post-000028 shape, not 000003's TO PUBLIC original.
	var enabled bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT c.relrowsecurity
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1`, marketMoversConfigTable).Scan(&enabled),
		"read relrowsecurity for %s", marketMoversConfigTable)
	assert.True(t, enabled, "restored %s should have RLS enabled", marketMoversConfigTable)

	rows, err := db.QueryContext(ctx, `
		SELECT policyname, roles::text
		FROM pg_policies
		WHERE schemaname = 'public' AND tablename = $1
		ORDER BY policyname`, marketMoversConfigTable)
	require.NoError(t, err, "query policies for %s", marketMoversConfigTable)
	defer func() { _ = rows.Close() }()

	var policies int
	for rows.Next() {
		var policy, roles string
		require.NoError(t, rows.Scan(&policy, &roles))
		policies++
		assert.Equal(t, "{service_role}", roles,
			"policy %q on restored %s must be scoped to service_role alone; %s "+
				"passes every role listed", policy, marketMoversConfigTable, roles)
	}
	require.NoError(t, rows.Err())

	// Where service_role is absent (a local or CI Postgres) the down migration
	// creates no policy at all: RLS enabled with zero policies is the same
	// effective deny, matching how 000029 behaves off Supabase.
	if roleExists(ctx, t, db, "service_role") {
		assert.Equal(t, 1, policies,
			"restored %s should carry exactly one TO service_role policy", marketMoversConfigTable)
	} else {
		assert.Zero(t, policies,
			"restored %s should have no policies where service_role is absent",
			marketMoversConfigTable)
	}

	// Re-applying 000030 drops it again, proving the rollback is not one-way.
	require.NoError(t, migrateToVersion(db, 30), "re-apply version 30")
	assert.False(t, tableExists(ctx, t, db, marketMoversConfigTable),
		"%s should be gone again after re-applying 000030", marketMoversConfigTable)
}

// migrateToVersion moves the database to an exact migration version, in either
// direction. Mirrors runMigrationsDown's setup but calls Migrate().
func migrateToVersion(db *DB, version uint) error {
	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}
	iofsDriver, err := iofs.New(MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", iofsDriver, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create migration instance: %w", err)
	}
	if err := m.Migrate(version); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate to %d: %w", version, err)
	}
	return nil
}

// tableExists reports whether a BASE TABLE of that name exists in public.
func tableExists(ctx context.Context, t *testing.T, db *DB, table string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1 AND c.relkind = 'r'
		)`, table).Scan(&exists), "probe existence of %s", table)
	return exists
}

// tableColumns returns a column name → data type map for a public table.
func tableColumns(ctx context.Context, t *testing.T, db *DB, table string) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1`, table)
	require.NoError(t, err, "read columns for %s", table)
	defer func() { _ = rows.Close() }()

	columns := map[string]string{}
	for rows.Next() {
		var name, dataType string
		require.NoError(t, rows.Scan(&name, &dataType))
		columns[name] = dataType
	}
	require.NoError(t, rows.Err())
	return columns
}
