package postgres

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

// TestMigration000022_SaleReasonBackfill exercises migration 000022 in isolation:
// it lands legacy rows at v21 (pre-provenance schema), steps up to v22 to run
// the sale_reason backfill + forced_liquidation re-sync, then verifies the
// rollback-window compatibility trigger derives sale_reason from
// forced_liquidation for legacy-shaped inserts (no sale_reason column supplied).
func TestMigration000022_SaleReasonBackfill(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	// Known baseline.
	_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err)

	// Build a migrate instance on the embedded source.
	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	require.NoError(t, err)
	src, err := iofs.New(MigrationsFS, "migrations")
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	require.NoError(t, err)

	// Migrate to v21 (pre-provenance), seed legacy rows there.
	require.NoError(t, m.Migrate(21))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp1', 'Test Campaign')`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
		VALUES ('p1', 'camp1', 'Card One', 'CERT1', '2026-01-01', 10000)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
		VALUES ('p2', 'camp1', 'Card Two', 'CERT2', '2026-01-01', 5000)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
		VALUES ('p3', 'camp1', 'Card Three', 'CERT3', '2026-01-01', 5000)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
		VALUES ('p4', 'camp1', 'Card Four', 'CERT4', '2026-01-01', 5000)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
		VALUES ('p5', 'camp1', 'Card Five', 'CERT5', '2026-01-01', 5000)`)
	require.NoError(t, err)

	// S1: in-person sale below 80% of CL value at purchase -> should backfill to invoice_pressure.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, forced_liquidation)
		VALUES ('s1', 'p1', 'inperson', 7000, '2026-01-15', FALSE)`)
	require.NoError(t, err)
	// S2: full-price ebay sale -> should backfill to discretionary.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, forced_liquidation)
		VALUES ('s2', 'p2', 'ebay', 6000, '2026-01-15', FALSE)`)
	require.NoError(t, err)

	// Step up to v22 — runs the backfill.
	require.NoError(t, m.Steps(1))

	var reason string
	var forced bool

	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sale_reason, forced_liquidation FROM campaign_sales WHERE id = 's1'`).Scan(&reason, &forced))
	require.Equal(t, "invoice_pressure", reason)
	require.True(t, forced)

	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sale_reason, forced_liquidation FROM campaign_sales WHERE id = 's2'`).Scan(&reason, &forced))
	require.Equal(t, "discretionary", reason)
	require.False(t, forced)

	// Rollback-window compatibility: simulate the OLD image inserting a sale after
	// v22 — it writes forced_liquidation only, no sale_reason. The BEFORE trigger
	// must derive sale_reason from the boolean.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, forced_liquidation)
		VALUES ('s3', 'p3', 'inperson', 100, '2026-02-01', TRUE)`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sale_reason FROM campaign_sales WHERE id = 's3'`).Scan(&reason))
	require.Equal(t, "invoice_pressure", reason)

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, forced_liquidation)
		VALUES ('s4', 'p4', 'ebay', 6000, '2026-02-01', FALSE)`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sale_reason FROM campaign_sales WHERE id = 's4'`).Scan(&reason))
	require.Equal(t, "discretionary", reason)

	// Explicit richer reason must be preserved (trigger only fills empty).
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, forced_liquidation, sale_reason)
		VALUES ('s5', 'p5', 'ebay', 9000, '2026-02-01', FALSE, 'aging_policy')`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sale_reason FROM campaign_sales WHERE id = 's5'`).Scan(&reason))
	require.Equal(t, "aging_policy", reason)

	// Round-trip down/up to confirm reversibility (down drops the trigger+function
	// and the 000022-added columns, keeps forced_liquidation; up re-adds them).
	require.NoError(t, m.Steps(-1))
	require.NoError(t, m.Steps(1))
}
