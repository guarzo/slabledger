package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration000047RetainsOriginalDataAndRestrictsRoles(t *testing.T) {
	db, _ := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	store := NewShowPrepStore(db.DB)
	seedShowEvidence(t, store)
	// Existing records, including a safety hold, must survive both directions.
	_, err := store.CreateList(ctx, showList, "Retained list")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,version,acknowledged_price_cents,acknowledged_status) VALUES('44444444-4444-4444-8444-444444444444',$1,$2,'Card','cert','PSA',10,now(),1,30000,'supported')`, showList, showPurchase)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO showprep_price_holds(purchase_id,cert_number,grader) VALUES('held','cert','PSA')`)
	require.NoError(t, err)
	up, err := MigrationsFS.ReadFile("migrations/000047_showprep_worker.up.sql")
	require.NoError(t, err)
	down, err := MigrationsFS.ReadFile("migrations/000047_showprep_worker.down.sql")
	require.NoError(t, err)
	for _, roles := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "Supabase roles"}[roles], func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			originals := map[string]string{}
			for _, table := range []string{"showprep_lists", "showprep_items", "showprep_price_holds"} {
				var data string
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT json_agg(t)::text FROM `+table+` t`).Scan(&data))
				originals[table] = data
			}
			var payload string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT payload::text FROM showprep_evidence`).Scan(&payload))
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
			var enabled bool
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT relrowsecurity FROM pg_class WHERE oid='showprep_worker'::regclass`).Scan(&enabled))
			require.True(t, enabled)
			var unsafe int
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE tablename='showprep_worker' AND roles::text<>'{service_role}'`).Scan(&unsafe))
			require.Zero(t, unsafe)
			if roles {
				for _, role := range []string{"anon", "authenticated"} {
					var allowed bool
					require.NoError(t, tx.QueryRowContext(ctx, `SELECT has_table_privilege($1,'showprep_worker','SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')`, role).Scan(&allowed))
					require.False(t, allowed)
					// Even an accidentally reintroduced table grant cannot bypass RLS.
					_, err = tx.ExecContext(ctx, `GRANT SELECT,UPDATE ON showprep_worker TO `+role+`; SET LOCAL ROLE `+role)
					require.NoError(t, err)
					var visible int
					require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM showprep_worker`).Scan(&visible))
					require.Zero(t, visible)
					res, err := tx.ExecContext(ctx, `UPDATE showprep_worker SET requested=true`)
					require.NoError(t, err)
					n, err := res.RowsAffected()
					require.NoError(t, err)
					require.Zero(t, n)
					_, err = tx.ExecContext(ctx, `RESET ROLE`)
					require.NoError(t, err)
				}
				_, err = tx.ExecContext(ctx, `SET LOCAL ROLE service_role`)
				require.NoError(t, err)
				var count int
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM showprep_worker`).Scan(&count))
				require.Equal(t, 1, count)
				_, err = tx.ExecContext(ctx, `UPDATE showprep_worker SET requested=true; RESET ROLE`)
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			for table, before := range originals {
				var after string
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT json_agg(t)::text FROM `+table+` t`).Scan(&after))
				require.Equal(t, before, after)
			}
			var after string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT payload::text FROM showprep_evidence`).Scan(&after))
			require.Equal(t, payload, after)
			_, err = tx.ExecContext(ctx, string(up))
			require.NoError(t, err)
		})
	}
}
