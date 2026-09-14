package postgres

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMigration000046SecurityAndRollback(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()
	up, err := MigrationsFS.ReadFile("migrations/000046_show_preparation.up.sql")
	require.NoError(t, err)
	down, err := MigrationsFS.ReadFile("migrations/000046_show_preparation.down.sql")
	require.NoError(t, err)
	for _, tt := range []struct {
		name        string
		createRoles bool
	}{{"local roles absent", false}, {"Supabase role grants", true}} {
		t.Run(tt.name, func(t *testing.T) {
			// Rollback restores the schema and any test-created roles/grants. Do not use
			// the fixture helper while tables are intentionally absent during migration.
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			if tt.createRoles {
				for _, role := range []string{"service_role", "anon", "authenticated"} {
					var exists bool
					require.NoError(t, tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role).Scan(&exists))
					if !exists {
						_, err = tx.ExecContext(ctx, `CREATE ROLE `+role)
						require.NoError(t, err)
					}
				}
				_, err = tx.ExecContext(ctx, `ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO anon, authenticated`)
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, string(up))
			require.NoError(t, err)
			for _, table := range []string{"showprep_lists", "showprep_items", "showprep_evidence", "showprep_price_holds"} {
				var enabled bool
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT relrowsecurity FROM pg_class WHERE oid=$1::regclass`, table).Scan(&enabled))
				require.True(t, enabled)
				var unsafePolicies int
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE tablename=$1 AND roles::text<>'{service_role}'`, table).Scan(&unsafePolicies))
				require.Zero(t, unsafePolicies)
				if tt.createRoles {
					for _, role := range []string{"anon", "authenticated"} {
						var allowed bool
						require.NoError(t, tx.QueryRowContext(ctx, `SELECT has_table_privilege($1,$2,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')`, role, table).Scan(&allowed))
						require.False(t, allowed)
					}
				}
			}
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			var count int
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name LIKE 'showprep_%'`).Scan(&count))
			require.Zero(t, count)
		})
	}
}
