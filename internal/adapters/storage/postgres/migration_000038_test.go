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

// The four views 000038 removes. All four were created by 000001, recreated by
// 000003 WITH (security_invoker = true), and never read by any Go or TypeScript
// file in the repository.
var unusedViews = []string{
	"active_sessions",
	"expired_sessions",
	"api_hourly_distribution",
	"api_daily_summary",
}

// apiUsageSummaryView is the fifth view, deliberately kept: GetAPIUsage selects
// from it (api_tracker.go:52).
const apiUsageSummaryView = "api_usage_summary"

// TestMigration000038_ViewsDropped is the acceptance criterion: after the full
// embedded migration set applies, the four consumerless views are gone.
//
// api_usage_summary is asserted present in the same test rather than a separate
// one, because "dropped the unused views" and "kept the used one" are the same
// decision. Two of the dropped views read the same api_calls table it does and
// look interchangeable with it at a glance; an assertion that only checked the
// absences would pass just as happily if all five had gone.
func TestMigration000038_ViewsDropped(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	for _, view := range unusedViews {
		assert.False(t, viewExists(ctx, t, db, view),
			"view %s should not exist after migration 000038", view)
	}

	require.True(t, viewExists(ctx, t, db, apiUsageSummaryView),
		"%s has a consumer (GetAPIUsage) and must survive 000038", apiUsageSummaryView)
}

// TestMigration000038_ApiUsageStillSelectable proves the surviving view is not
// merely present but still resolves — 000038 touches four views over the same
// two tables, and a stray drop or a dependency error would surface here rather
// than at runtime in the price-refresh scheduler's usage logging.
func TestMigration000038_ApiUsageStillSelectable(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	var n int64
	require.NoError(t,
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+apiUsageSummaryView).Scan(&n),
		"%s should still be selectable after 000038", apiUsageSummaryView)
}

// TestMigration000038_DownRestoresViews pins the shape the rollback restores,
// which the generic up/down/up roundtrip cannot: that test proves the SQL is
// valid, not that the views come back correctly configured.
//
// Three things are worth pinning:
//
//  1. The views exist again and are selectable, not merely present in pg_class.
//  2. security_invoker = true. 000003's Fix 1 set it because Supabase flags a
//     view without it as SECURITY DEFINER, enforcing the creator's permissions
//     rather than the querier's. A down migration written from 000001's plain
//     CREATE VIEW would roll the database back past that security fix.
//  3. anon and authenticated hold no privilege. A view is a separate grantable
//     object and Supabase's default privileges grant on it, so a restored view
//     is readable by anon unless the REVOKE is repeated — which is why 000028
//     listed all four by name.
//
// Rolls back one step rather than to a hard-coded version number: this branch
// and the 000037 branch are in flight together, so the version immediately
// below head depends on merge order, while "undo the newest migration" does not.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000038_DownRestoresViews(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Restore the package's shared database no matter how this test exits.
	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateSteps(db, -1), "roll back the newest migration")

	for _, view := range unusedViews {
		require.True(t, viewExists(ctx, t, db, view),
			"000038's down migration should recreate view %s", view)

		var n int64
		require.NoError(t,
			db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+view).Scan(&n),
			"restored view %s should be selectable, not merely present", view)

		assert.True(t, viewIsSecurityInvoker(ctx, t, db, view),
			"restored view %s must keep security_invoker = true, as 000003 set it; "+
				"without it Supabase treats the view as SECURITY DEFINER", view)
	}

	// Uses has_table_privilege rather than information_schema.role_table_grants,
	// matching migration_000027_test.go:108-141 and for the reason documented
	// there: role_table_grants lists only *direct* grants, so a privilege
	// reaching anon through a grant to PUBLIC or through role inheritance slips
	// past it and the assertion passes while the access is still live.
	privileges := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "REFERENCES", "TRIGGER"}
	for _, role := range []string{"anon", "authenticated"} {
		if !roleExists(ctx, t, db, role) {
			continue
		}
		for _, view := range unusedViews {
			for _, privilege := range privileges {
				var allowed bool
				require.NoError(t, db.QueryRowContext(ctx,
					`SELECT has_table_privilege($1::text, format('public.%I', $2::text), $3::text)`,
					role, view, privilege).Scan(&allowed),
					"probe %s on %s for %s", privilege, view, role)
				assert.False(t, allowed,
					"restored view %s must not be reachable by %s (effective %s remains; "+
						"check for grants to PUBLIC or an inherited role, not just direct grants)",
					view, role, privilege)
			}
		}
	}

	// Re-applying removes all four again, proving the rollback is not one-way —
	// up-then-down-then-up is idempotent.
	require.NoError(t, RunMigrations(db, ""), "re-apply the newest migration")
	for _, view := range unusedViews {
		assert.False(t, viewExists(ctx, t, db, view),
			"view %s should be gone again after re-applying 000038", view)
	}
}

// viewIsSecurityInvoker reports whether a view carries the security_invoker
// reloption. Postgres stores it in pg_class.reloptions as the literal text
// "security_invoker=true", so the probe is a membership test on that array.
func viewIsSecurityInvoker(ctx context.Context, t *testing.T, db *DB, view string) bool {
	t.Helper()
	var invoker bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COALESCE('security_invoker=true' = ANY(c.reloptions), false)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1 AND c.relkind = 'v'`,
		view).Scan(&invoker), "read reloptions for view %s", view)
	return invoker
}

// migrateSteps moves the database n migrations in either direction, relative to
// wherever it currently is. Negative n rolls back.
//
// Distinct from migrateToVersion (migration_000030_test.go:117), which needs an
// absolute version number. A relative step is what "undo the newest migration"
// actually means, and it stays correct when the migration set gains a file.
func migrateSteps(db *DB, n int) error {
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
	if err := m.Steps(n); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate %d steps: %w", n, err)
	}
	return nil
}
