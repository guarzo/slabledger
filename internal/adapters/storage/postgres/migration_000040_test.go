package postgres

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

// TestMigration000040_PriceSourceBackfill exercises migration 000040 in
// isolation: it lands legacy rows at v39 (pre-provenance schema), steps up to
// v40 to run the price_source backfill, then verifies each of the three
// backfill outcomes and that their_comp_cents is never synthesized.
func TestMigration000040_PriceSourceBackfill(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	// Known baseline.
	_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err)

	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	require.NoError(t, err)
	src, err := iofs.New(MigrationsFS, "migrations")
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	require.NoError(t, err)

	// Migrate to v39 (pre-provenance), seed legacy rows there.
	require.NoError(t, m.Migrate(39))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp1', 'Test Campaign')`)
	require.NoError(t, err)

	for _, id := range []string{"p1", "p2", "p3"} {
		_, err = db.ExecContext(ctx, `
			INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
			VALUES ($1, 'camp1', 'Card', $2, '2026-01-01', 10000)`, id, id)
		require.NoError(t, err)
	}

	// S1: has an order_id -> itemized, regardless of channel.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, order_id)
		VALUES ('s1', 'p1', 'ebay', 7000, '2026-01-15', 'dh-order-1')`)
	require.NoError(t, err)
	// S2: in-person, no order_id -> estimated.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date)
		VALUES ('s2', 'p2', 'inperson', 6000, '2026-01-15')`)
	require.NoError(t, err)
	// S3: ebay, no order_id -> stays unknown ('').
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date)
		VALUES ('s3', 'p3', 'ebay', 5000, '2026-01-15')`)
	require.NoError(t, err)

	// Step up to v40 — runs the backfill.
	require.NoError(t, m.Steps(1))

	tests := []struct {
		id            string
		wantSource    string
		wantCompCents int64
	}{
		{"s1", "itemized", 0},
		{"s2", "estimated", 0},
		{"s3", "", 0},
	}
	for _, tt := range tests {
		var source string
		var comp int64
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT price_source, their_comp_cents FROM campaign_sales WHERE id = $1`, tt.id).
			Scan(&source, &comp))
		require.Equal(t, tt.wantSource, source, "sale %s price_source", tt.id)
		require.Equal(t, tt.wantCompCents, comp, "sale %s their_comp_cents must not be synthesized", tt.id)
	}

	// Round-trip down/up to confirm reversibility.
	require.NoError(t, m.Steps(-1))
	require.NoError(t, m.Steps(1))
}
