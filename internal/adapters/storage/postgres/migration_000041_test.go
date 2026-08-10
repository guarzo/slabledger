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

// TestMigration000041_NoBackfill exercises migration 000041 in isolation: it
// lands a legacy purchase and its sale at v40 (pre-provenance-source schema),
// steps up to v41, and verifies that the new columns land at their
// "provenance unknown" defaults rather than being backfilled from existing
// data. This is the D3/D5 no-backfill guarantee: the migration adds a place
// to record provenance, it does not manufacture provenance for rows that
// never had it.
func TestMigration000041_NoBackfill(t *testing.T) {
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

	// Migrate to v40 (pre-provenance-source), seed a legacy row there. This
	// purchase already carries a frozen cl_value_at_purchase_cents from before
	// this change existed -- exactly the shape of the 795-row backlog the
	// design spec says must NOT be relabeled with manufactured provenance.
	require.NoError(t, m.Migrate(40))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp1', 'Test Campaign')`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
		VALUES ('p1', 'camp1', 'Card', 'p1', '2026-01-01', 10000)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, cl_value_at_sale_cents)
		VALUES ('s1', 'p1', 'ebay', 7000, '2026-01-15', 9000)`)
	require.NoError(t, err)

	// Step up to v41 -- adds the columns, does not touch existing rows beyond
	// their new-column defaults.
	require.NoError(t, m.Steps(1))

	var (
		purchaseSource     string
		purchaseObservedAt string
		purchaseConfidence sql.NullInt64
		saleSource         string
		saleObservedAt     string
	)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cl_value_at_purchase_source, cl_value_at_purchase_observed_at, cl_card_confidence_at_purchase
		 FROM campaign_purchases WHERE id = 'p1'`).
		Scan(&purchaseSource, &purchaseObservedAt, &purchaseConfidence))
	require.Equal(t, "", purchaseSource, "legacy purchase must not be backfilled with a fabricated source")
	require.Equal(t, "", purchaseObservedAt, "legacy purchase must not be backfilled with a fabricated observed_at")
	require.False(t, purchaseConfidence.Valid, "legacy purchase must have no card confidence, not a fabricated one")

	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cl_value_at_sale_source, cl_value_at_sale_observed_at
		 FROM campaign_sales WHERE id = 's1'`).
		Scan(&saleSource, &saleObservedAt))
	require.Equal(t, "", saleSource, "legacy sale must not be backfilled with a fabricated source")
	require.Equal(t, "", saleObservedAt, "legacy sale must not be backfilled with a fabricated observed_at")

	// Round-trip down/up to confirm reversibility.
	require.NoError(t, m.Steps(-1))
	require.NoError(t, m.Steps(1))
}
