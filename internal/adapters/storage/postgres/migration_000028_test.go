package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRLSPolicies_NoPublicPassthrough asserts the invariant migration 000028
// establishes: no policy in the public schema is scoped TO PUBLIC.
//
// 000003 created 36 policies of the form `USING (true) WITH CHECK (true)` with
// no TO clause, which defaults to TO PUBLIC and therefore passes anon and
// authenticated — enabling RLS while gating nothing. A policy that names a role
// is fine; a policy that names none is the bug.
func TestRLSPolicies_NoPublicPassthrough(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	rows, err := db.QueryContext(ctx, `
		SELECT tablename, policyname
		FROM pg_policies
		WHERE schemaname = 'public' AND roles::text = '{public}'
		ORDER BY tablename, policyname`)
	require.NoError(t, err, "query TO PUBLIC policies")
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var table, policy string
		require.NoError(t, rows.Scan(&table, &policy))
		assert.Fail(t, "policy is TO PUBLIC",
			"policy %q on %q has no TO clause, so it passes anon and authenticated; "+
				"scope it to a role (see migration 000028)", policy, table)
	}
	require.NoError(t, rows.Err())
}
