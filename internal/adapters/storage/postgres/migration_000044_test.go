package postgres

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

// TestMigration000044_AddDHSaleRecording exercises migration 000044 in
// isolation: it lands a legacy row at v43 (pre-DH-sale-recording schema),
// steps up to v44, and confirms the new columns land at their documented
// defaults for that pre-existing row -- the whole reason this migration
// carries no backfill (see the migration's own comment).
func TestMigration000044_AddDHSaleRecording(t *testing.T) {
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

	// Migrate to v43 (pre-DH-sale-recording), seed a legacy row there.
	require.NoError(t, m.Migrate(43))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp1', 'Test Campaign')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date)
		VALUES ('p1', 'camp1', 'Card One', 'CERT1', '2026-01-01')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date)
		VALUES ('s1', 'p1', 'inperson', 45000, '2026-01-15')`)
	require.NoError(t, err)

	// Step up to v44.
	require.NoError(t, m.Steps(1))

	var idempotencyKey, saleID string
	var recordedAt *string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT dh_idempotency_key, dh_sale_id, dh_sale_recorded_at::text FROM campaign_sales WHERE id = 's1'`,
	).Scan(&idempotencyKey, &saleID, &recordedAt))
	require.Equal(t, "", idempotencyKey, "legacy row must land at the '' sentinel, not a minted key")
	require.Equal(t, "", saleID)
	require.Nil(t, recordedAt, "dh_sale_recorded_at must be NULL, not a synthesized timestamp")

	var conflict string
	var conflictAt *string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT dh_sale_conflict, dh_sale_conflict_at::text FROM campaign_purchases WHERE id = 'p1'`,
	).Scan(&conflict, &conflictAt))
	require.Equal(t, "", conflict)
	require.Nil(t, conflictAt)

	// Step down to v43 and confirm the columns are gone.
	require.NoError(t, m.Steps(-1))

	var colCount int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE (table_name = 'campaign_sales' AND column_name IN ('dh_idempotency_key', 'dh_sale_id', 'dh_sale_recorded_at'))
		   OR (table_name = 'campaign_purchases' AND column_name IN ('dh_sale_conflict', 'dh_sale_conflict_at'))`,
	).Scan(&colCount))
	require.Equal(t, 0, colCount, "down migration must drop all five columns")
}
