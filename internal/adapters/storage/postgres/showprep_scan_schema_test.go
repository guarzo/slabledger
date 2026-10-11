package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Dropping/recreating scan safety state must leave legacy confirmation history untouched.
func TestShowPrepScanMigrationRetainsLegacyHistory(t *testing.T) {
	for _, tt := range []struct{ name, packed string }{
		{"packed and acknowledged", "2026-09-14T12:00:00Z"}, {"unpacked and acknowledged", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			up, err := MigrationsFS.ReadFile("migrations/000049_showprep_scan_confirmation.up.sql")
			require.NoError(t, err)
			down, err := MigrationsFS.ReadFile("migrations/000049_showprep_scan_confirmation.down.sql")
			require.NoError(t, err)
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, `INSERT INTO showprep_lists(id,name,created_at,updated_at) VALUES($1,'Old show','2026-09-01','2026-09-02');`, showList)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,packed_at,version,acknowledged_price_cents,acknowledged_status,last_command) VALUES($1,$2,'deleted-purchase','Old card','123','PSA',10,'2026-09-01',NULLIF($3,'')::timestamptz,7,12345,'thin_evidence','old-command')`, showPurchase, showList, tt.packed)
			require.NoError(t, err)
			var before string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT to_jsonb(i)::text FROM showprep_items i WHERE id=$1`, showPurchase).Scan(&before))
			_, err = tx.ExecContext(ctx, string(up))
			require.NoError(t, err)
			var after string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT to_jsonb(i)::text FROM showprep_items i WHERE id=$1`, showPurchase).Scan(&after))
			require.Equal(t, before, after)
			var generation int64
			var name, created, updated string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT withdrawal_generation,name,created_at::date::text,updated_at::date::text FROM showprep_lists WHERE id=$1`, showList).Scan(&generation, &name, &created, &updated))
			require.Zero(t, generation)
			require.Equal(t, "Old show", name)
			require.Equal(t, "2026-09-01", created)
			require.Equal(t, "2026-09-02", updated)
		})
	}
}

func TestShowPrepScanMigrationSecurityAndBounds(t *testing.T) {
	db := setupShowPrepTestDB(t)
	ctx := context.Background()
	up, err := MigrationsFS.ReadFile("migrations/000049_showprep_scan_confirmation.up.sql")
	require.NoError(t, err)
	down, err := MigrationsFS.ReadFile("migrations/000049_showprep_scan_confirmation.down.sql")
	require.NoError(t, err)
	for _, tt := range []struct {
		name  string
		roles bool
	}{{"local", false}, {"Supabase", true}} {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			if tt.roles {
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
			for _, table := range []string{"showprep_command_receipts", "showprep_retired_lists"} {
				var enabled bool
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT relrowsecurity FROM pg_class WHERE oid=$1::regclass`, table).Scan(&enabled))
				require.True(t, enabled)
				var unsafe int
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE tablename=$1 AND roles::text<>'{service_role}'`, table).Scan(&unsafe))
				require.Zero(t, unsafe)
				if tt.roles {
					for _, role := range []string{"anon", "authenticated", "service_role"} {
						var allowed bool
						require.NoError(t, tx.QueryRowContext(ctx, `SELECT has_table_privilege($1,$2,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')`, role, table).Scan(&allowed))
						require.Equal(t, role == "service_role", allowed)
					}
				}
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO showprep_lists(id,name) VALUES($1,'Bounds')`, showList)
			require.NoError(t, err)
			for _, value := range []int64{-1, 9007199254740992} {
				_, err = tx.ExecContext(ctx, `SAVEPOINT bounds`)
				require.NoError(t, err)
				_, err = tx.ExecContext(ctx, `UPDATE showprep_lists SET withdrawal_generation=$2 WHERE id=$1`, showList, value)
				require.Error(t, err)
				_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT bounds`)
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, string(down))
			require.NoError(t, err)
			var count int
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name IN ('showprep_command_receipts','showprep_retired_lists')`).Scan(&count))
			require.Zero(t, count)
		})
	}
}
