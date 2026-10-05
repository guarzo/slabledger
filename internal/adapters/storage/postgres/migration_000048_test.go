package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration000048UpDownRLS(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	up, err := MigrationsFS.ReadFile("migrations/000048_confirmed_dh_returns.up.sql")
	require.NoError(t, err)
	down, err := MigrationsFS.ReadFile("migrations/000048_confirmed_dh_returns.down.sql")
	require.NoError(t, err)
	for _, roles := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "Supabase roles"}[roles], func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			if roles {
				for _, role := range []string{"service_role", "anon", "authenticated"} {
					var exists bool
					require.NoError(t, tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role).Scan(&exists))
					if !exists {
						_, err = tx.ExecContext(ctx, `CREATE ROLE `+role)
						require.NoError(t, err)
					}
				}
				_, err = tx.ExecContext(ctx, `GRANT USAGE ON SCHEMA public TO service_role,anon,authenticated; ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO anon,authenticated,PUBLIC`)
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, string(up))
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, `INSERT INTO dh_target_watermarks(grader,cert_number) VALUES('PSA','retained-rls'); INSERT INTO confirmed_dh_returns(id,idempotency_key,captured_purchase_id,dh_inventory_id,cert_number,grader) VALUES('rls-op','rls-key','old-purchase',1,'retained-rls','PSA'); INSERT INTO dh_mutation_attempts(id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('rls-attempt','old-purchase',1,'retained-rls','PSA','listing','patch','payload')`)
			require.NoError(t, err)
			for _, table := range []string{"confirmed_dh_returns", "dh_mutation_attempts", "dh_target_watermarks"} {
				var enabled bool
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT relrowsecurity FROM pg_class WHERE oid=$1::regclass`, table).Scan(&enabled))
				require.True(t, enabled)
				var unsafe int
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE tablename=$1 AND roles::text<>'{service_role}'`, table).Scan(&unsafe))
				require.Zero(t, unsafe)
				if roles {
					for _, role := range []string{"anon", "authenticated"} {
						var allowed bool
						require.NoError(t, tx.QueryRowContext(ctx, `SELECT has_table_privilege($1,$2,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')`, role, table).Scan(&allowed))
						require.False(t, allowed)
						_, err = tx.ExecContext(ctx, `GRANT SELECT,DELETE ON `+table+` TO `+role+`; SET LOCAL ROLE `+role)
						require.NoError(t, err)
						var visible int
						require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&visible))
						require.Zero(t, visible)
						res, err := tx.ExecContext(ctx, `DELETE FROM `+table)
						require.NoError(t, err)
						n, err := res.RowsAffected()
						require.NoError(t, err)
						require.Zero(t, n)
						_, err = tx.ExecContext(ctx, `RESET ROLE`)
						require.NoError(t, err)
					}
					_, err = tx.ExecContext(ctx, `SET LOCAL ROLE service_role`)
					require.NoError(t, err)
					var visible int
					require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&visible))
					require.Equal(t, 1, visible)
					_, err = tx.ExecContext(ctx, `RESET ROLE`)
					require.NoError(t, err)
				}
			}
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			var absent bool
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT to_regclass('confirmed_dh_returns') IS NULL AND to_regclass('dh_mutation_attempts') IS NULL AND to_regclass('dh_target_watermarks') IS NULL`).Scan(&absent))
			require.True(t, absent)
		})
	}
}
