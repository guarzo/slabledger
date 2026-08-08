package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rls000027Tables are the six live tables retrofitted with row-level security by
// migration 000027 — each was created after the 000003 blanket RLS pass and
// never picked it up.
var rls000027Tables = []string{
	"dh_comp_cache",
	"dh_card_tombstones",
	"psa_portal_token",
	"psa_portal_snapshot",
	"psa_campaign_snapshot",
	"psa_portal_catalog",
}

// TestMigration000027_RLSEnabled verifies that the six tables migration 000027
// targets have row-level security enabled after the full migration set applies.
func TestMigration000027_RLSEnabled(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	for _, table := range rls000027Tables {
		t.Run(table, func(t *testing.T) {
			var enabled bool
			require.NoError(t, db.QueryRowContext(ctx, `
				SELECT c.relrowsecurity
				FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public' AND c.relname = $1`, table).Scan(&enabled),
				"read relrowsecurity for %s", table)
			assert.True(t, enabled, "%s should have RLS enabled (migration 000027)", table)
		})
	}
}

// TestRLSCoverage_NoUntrackedGaps guards against the drift that produced
// 000027: a new table lands in a migration and silently misses RLS. Any public
// table without RLS must be listed here with a reason.
func TestRLSCoverage_NoUntrackedGaps(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	knownWithoutRLS := map[string]string{
		// golang-migrate's own version-tracking table, not application data.
		"schema_migrations": "migration tooling table",
		// Tracked separately — its fix is not mechanical.
		"psa_campaign_push_queue": "ticketed separately (SLA-9)",
	}

	rows, err := db.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND NOT c.relrowsecurity
		ORDER BY c.relname`)
	require.NoError(t, err, "query tables without RLS")
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var table string
		require.NoError(t, rows.Scan(&table))
		_, known := knownWithoutRLS[table]
		assert.True(t, known,
			"table %q has no RLS and is not a documented exception — enable RLS in a migration "+
				"or add it to knownWithoutRLS with a reason", table)
	}
	require.NoError(t, rows.Err())
}
