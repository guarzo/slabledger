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

// roleExists reports whether a Supabase role is present. anon, authenticated
// and service_role exist in Supabase but not in a local or CI Postgres, so the
// assertions that depend on them are conditional.
func roleExists(ctx context.Context, t *testing.T, db *DB, role string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists),
		"probe for role %s", role)
	return exists
}

// TestMigration000027_PoliciesScopedToRole asserts the property that actually
// gates access, which relrowsecurity does not: every policy on these tables is
// scoped to service_role and nothing else.
//
// This is the whole point of 000027. A policy with no TO clause defaults to TO
// PUBLIC and passes anon and authenticated, so RLS can be enabled and gate
// nothing — which is what 000003 did to 36 other tables. A test that checks
// only relrowsecurity would pass in exactly that broken state.
//
// Where service_role is absent (a local or CI Postgres) 000027 creates no
// policy at all, leaving RLS enabled with zero policies — the same effective
// deny — so that case asserts an empty policy set rather than a service_role one.
func TestMigration000027_PoliciesScopedToRole(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()
	hasServiceRole := roleExists(ctx, t, db, "service_role")

	for _, table := range rls000027Tables {
		t.Run(table, func(t *testing.T) {
			rows, err := db.QueryContext(ctx, `
				SELECT policyname, roles::text
				FROM pg_policies
				WHERE schemaname = 'public' AND tablename = $1
				ORDER BY policyname`, table)
			require.NoError(t, err, "query policies for %s", table)
			defer func() { _ = rows.Close() }()

			var policies int
			for rows.Next() {
				var policy, roles string
				require.NoError(t, rows.Scan(&policy, &roles))
				policies++
				// Exact match, not merely "not {public}": a second policy scoped
				// to some other role would re-open the table to whoever holds
				// that role's grants.
				assert.Equal(t, "{service_role}", roles,
					"policy %q on %s must be scoped to service_role alone; %s passes "+
						"every role listed", policy, table, roles)
			}
			require.NoError(t, rows.Err())

			if hasServiceRole {
				assert.Equal(t, 1, policies,
					"%s should carry exactly one TO service_role policy (migration 000027)", table)
			} else {
				assert.Zero(t, policies,
					"%s should have no policies at all where service_role is absent; "+
						"RLS enabled with zero policies is the same effective deny", table)
			}
		})
	}
}

// TestMigration000027_NoPublicRoleGrants asserts the other half of the control.
// The table GRANT is what decides whether PostgREST surfaces a table at all, so
// revoking it from anon and authenticated matters independently of any policy.
// Skips where the roles are absent, since there is nothing to revoke.
//
// Uses has_table_privilege rather than information_schema.role_table_grants:
// the latter lists only direct grants, so a privilege reaching anon through a
// grant to PUBLIC or through role inheritance would slip past it.
func TestMigration000027_NoPublicRoleGrants(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Every table-level privilege Postgres defines; REVOKE ALL must clear them all.
	tablePrivileges := []string{
		"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER",
	}

	for _, role := range []string{"anon", "authenticated"} {
		if !roleExists(ctx, t, db, role) {
			t.Logf("role %s absent (not a Supabase database) — nothing to revoke", role)
			continue
		}
		for _, table := range rls000027Tables {
			t.Run(role+"/"+table, func(t *testing.T) {
				for _, privilege := range tablePrivileges {
					var allowed bool
					require.NoError(t, db.QueryRowContext(ctx,
						`SELECT has_table_privilege($1::text, format('public.%I', $2::text), $3::text)`,
						role, table, privilege).Scan(&allowed),
						"probe %s on %s for %s", privilege, table, role)
					assert.False(t, allowed,
						"%s still has effective %s on %s; migration 000027 should have revoked it "+
							"(check for grants to PUBLIC or an inherited role, not just direct grants)",
						role, privilege, table)
				}
			})
		}
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
