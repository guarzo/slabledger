package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

// TestMigration000043_DropOldPolicyConfidenceColumn exercises the contract half
// of the cl_confidence_at_purchase rename. It seeds TWO rows at v42: one
// dual-written (both columns set, as a deploy-N instance would write) and one
// old-only (new column NULL, as a deploy-(N-1) instance writing after 000042's
// backfill would leave it). Stepping to v43 must reconcile the second row
// before dropping the column -- that row is the whole reason the migration
// carries an UPDATE and not just a DROP.
func TestMigration000043_DropOldPolicyConfidenceColumn(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err)

	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	require.NoError(t, err)
	src, err := iofs.New(MigrationsFS, "migrations")
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	require.NoError(t, err)

	// Migrate to v42 (post-expand).
	require.NoError(t, m.Migrate(42))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp1', 'Test Campaign')`)
	require.NoError(t, err)

	// Row 1: dual-written by a deploy-N instance.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date,
			cl_confidence_at_purchase, cl_policy_confidence_min_at_purchase)
		VALUES ('p1', 'camp1', 'Card', 'cert1', '2026-01-01', 65, 65)`)
	require.NoError(t, err)

	// Row 2: written by a deploy-(N-1) instance AFTER 000042's backfill ran --
	// old column set, new column still NULL. This row is why the reconcile
	// UPDATE exists; without it, 40 is lost at the DROP.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date,
			cl_confidence_at_purchase, cl_policy_confidence_min_at_purchase)
		VALUES ('p2', 'camp1', 'Card', 'cert2', '2026-01-02', 40, NULL)`)
	require.NoError(t, err)

	// Row 3: genuinely unknown on both sides -- must stay NULL, not become 0.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date,
			cl_confidence_at_purchase, cl_policy_confidence_min_at_purchase)
		VALUES ('p3', 'camp1', 'Card', 'cert3', '2026-01-03', NULL, NULL)`)
	require.NoError(t, err)

	// Step up to v43 -- reconciles, then drops the old column.
	require.NoError(t, m.Steps(1))

	var exists int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'campaign_purchases' AND column_name = 'cl_confidence_at_purchase'`,
	).Scan(&exists))
	require.Equal(t, 0, exists, "cl_confidence_at_purchase must be dropped")

	readNew := func(id string) sql.NullInt64 {
		t.Helper()
		var v sql.NullInt64
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT cl_policy_confidence_min_at_purchase FROM campaign_purchases WHERE id = $1`, id,
		).Scan(&v))
		return v
	}

	v1 := readNew("p1")
	require.True(t, v1.Valid)
	require.Equal(t, int64(65), v1.Int64, "dual-written row must be untouched")

	v2 := readNew("p2")
	require.True(t, v2.Valid, "old-only row must be reconciled before the DROP, not lost")
	require.Equal(t, int64(40), v2.Int64)

	v3 := readNew("p3")
	require.False(t, v3.Valid, "genuinely-unknown row must stay NULL, never coerced to 0")

	// Round-trip down: the column returns, repopulated from the new one.
	require.NoError(t, m.Steps(-1))
	var restored sql.NullInt64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cl_confidence_at_purchase FROM campaign_purchases WHERE id = 'p2'`,
	).Scan(&restored))
	require.True(t, restored.Valid, "down migration must restore the column's shape")
	require.Equal(t, int64(40), restored.Int64)

	require.NoError(t, m.Steps(1))
}
