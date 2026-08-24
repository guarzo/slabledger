package postgres

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

// TestMigration000045_AddTermsAtPurchase exercises migration 000045 in
// isolation: it lands a legacy row at v44 (pre-terms-freeze schema), steps up
// to v45, and confirms the new column lands NULL for that pre-existing row --
// the whole point of the migration carrying no backfill. Because the buy-price
// identity is invertible, historical terms COULD be solved for; the migration
// comment explains at length why a fitted value is worse than a hole, and this
// test is the executable form of that ban.
func TestMigration000045_AddTermsAtPurchase(t *testing.T) {
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

	// Migrate to v44 (pre-terms-freeze), seed a legacy row there. The campaign
	// carries real terms specifically so a backfill, if one were ever added,
	// would have something to copy and would fail this test.
	require.NoError(t, m.Migrate(44))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name, buy_terms_cl_pct) VALUES ('camp1', 'Test Campaign', 0.78)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, buy_cost_cents, psa_sourcing_fee_cents)
		VALUES ('p1', 'camp1', 'Card One', 'CERT1', '2026-01-01', 39100, 500)`)
	require.NoError(t, err)

	// Step up to v45.
	require.NoError(t, m.Steps(1))

	var terms *float64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT buy_terms_cl_pct_at_purchase FROM campaign_purchases WHERE id = 'p1'`,
	).Scan(&terms))
	require.Nil(t, terms, "legacy row must land NULL, not the campaign's current terms and not a solved-for fit")

	// The column must be nullable rather than NOT NULL DEFAULT 0: studies read it
	// through `IS NOT NULL`, which a 0-default would make useless.
	var isNullable, dataType string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT is_nullable, data_type FROM information_schema.columns
		WHERE table_name = 'campaign_purchases' AND column_name = 'buy_terms_cl_pct_at_purchase'`,
	).Scan(&isNullable, &dataType))
	require.Equal(t, "YES", isNullable)
	require.Equal(t, "double precision", dataType, "must match campaigns.buy_terms_cl_pct's type")

	// Step down to v44 and confirm the column is gone.
	require.NoError(t, m.Steps(-1))

	var colCount int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'campaign_purchases' AND column_name = 'buy_terms_cl_pct_at_purchase'`,
	).Scan(&colCount))
	require.Equal(t, 0, colCount, "down migration must drop the column")
}
