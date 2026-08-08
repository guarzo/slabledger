package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pushQueueTable is the one table migration 000027 deliberately skipped: it was
// created in 000017, after the 000003 blanket RLS pass, and tracked as SLA-9
// because its exposure is not only a grants problem. Migration 000029 brings it
// under the same TO service_role + REVOKE pattern as 000027.
const pushQueueTable = "psa_campaign_push_queue"

// TestMigration000029_RLSEnabled verifies the push queue has row-level security
// enabled after the full migration set applies.
func TestMigration000029_RLSEnabled(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	var enabled bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT c.relrowsecurity
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1`, pushQueueTable).Scan(&enabled),
		"read relrowsecurity for %s", pushQueueTable)
	assert.True(t, enabled, "%s should have RLS enabled (migration 000029)", pushQueueTable)
}

// TestMigration000029_PoliciesScopedToRole asserts the property that actually
// gates access: every policy on the queue is scoped to service_role and nothing
// else. A policy with no TO clause defaults to TO PUBLIC and passes anon and
// authenticated, so RLS can be enabled and gate nothing.
//
// Where service_role is absent (a local or CI Postgres) 000029 creates no policy
// at all, leaving RLS enabled with zero policies — the same effective deny.
func TestMigration000029_PoliciesScopedToRole(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()
	hasServiceRole := roleExists(ctx, t, db, "service_role")

	rows, err := db.QueryContext(ctx, `
		SELECT policyname, roles::text
		FROM pg_policies
		WHERE schemaname = 'public' AND tablename = $1
		ORDER BY policyname`, pushQueueTable)
	require.NoError(t, err, "query policies for %s", pushQueueTable)
	defer func() { _ = rows.Close() }()

	var policies int
	for rows.Next() {
		var policy, roles string
		require.NoError(t, rows.Scan(&policy, &roles))
		policies++
		// Exact match, not merely "not {public}": a second policy scoped to some
		// other role would re-open the queue to whoever holds that role's grants.
		assert.Equal(t, "{service_role}", roles,
			"policy %q on %s must be scoped to service_role alone; %s passes every "+
				"role listed", policy, pushQueueTable, roles)
	}
	require.NoError(t, rows.Err())

	if hasServiceRole {
		assert.Equal(t, 1, policies,
			"%s should carry exactly one TO service_role policy (migration 000029)", pushQueueTable)
	} else {
		assert.Zero(t, policies,
			"%s should have no policies at all where service_role is absent; RLS "+
				"enabled with zero policies is the same effective deny", pushQueueTable)
	}
}

// TestMigration000029_NoPublicRoleGrants asserts the other half of the control.
// The table GRANT is what decides whether PostgREST surfaces a table at all, so
// revoking it from anon and authenticated matters independently of any policy —
// and for this table it is the half that closes the original finding: an
// anon/authenticated caller flipping a queued row to approved and having the
// drain loop push it to the live PSA portal.
//
// Uses has_table_privilege rather than information_schema.role_table_grants:
// the latter lists only direct grants, so a privilege reaching anon through a
// grant to PUBLIC or through role inheritance would slip past it.
func TestMigration000029_NoPublicRoleGrants(t *testing.T) {
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
		t.Run(role, func(t *testing.T) {
			for _, privilege := range tablePrivileges {
				var allowed bool
				require.NoError(t, db.QueryRowContext(ctx,
					`SELECT has_table_privilege($1::text, format('public.%I', $2::text), $3::text)`,
					role, pushQueueTable, privilege).Scan(&allowed),
					"probe %s on %s for %s", privilege, pushQueueTable, role)
				assert.False(t, allowed,
					"%s still has effective %s on %s; migration 000029 should have revoked it "+
						"(check for grants to PUBLIC or an inherited role, not just direct grants)",
					role, privilege, pushQueueTable)
			}
		})
	}
}
