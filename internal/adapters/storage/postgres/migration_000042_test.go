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

// TestMigration000042_RenamePolicyConfidence exercises the expand half of the
// cl_confidence_at_purchase rename in isolation: it lands a legacy row at v41
// (pre-rename schema, post the cl_card_confidence_at_purchase addition), steps
// up to v42 to add cl_policy_confidence_min_at_purchase and backfill it from
// the old column, and verifies the old column is untouched (deploy N is
// additive-only; Task 12 drops it, two deploys later).
func TestMigration000042_RenamePolicyConfidence(t *testing.T) {
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

	// Migrate to v41 (pre-rename), seed a purchase carrying the misnamed
	// policy-confidence value there.
	require.NoError(t, m.Migrate(41))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp1', 'Test Campaign')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_confidence_at_purchase)
		VALUES ('p1', 'camp1', 'Card', 'cert1', '2026-01-01', 65)`)
	require.NoError(t, err)
	// A row with no policy-confidence value must backfill to NULL, not 0.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date)
		VALUES ('p2', 'camp1', 'Card 2', 'cert2', '2026-01-01')`)
	require.NoError(t, err)

	// Step up to v42 — adds the new column and backfills it.
	require.NoError(t, m.Steps(1))

	var (
		newVal   sql.NullInt64
		oldVal   sql.NullInt64
		newValP2 sql.NullInt64
	)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cl_policy_confidence_min_at_purchase, cl_confidence_at_purchase FROM campaign_purchases WHERE id = 'p1'`,
	).Scan(&newVal, &oldVal))
	require.True(t, newVal.Valid, "new column must be backfilled")
	require.Equal(t, int64(65), newVal.Int64)
	require.True(t, oldVal.Valid, "old column must survive deploy N (dropped only in the contract deploy)")
	require.Equal(t, int64(65), oldVal.Int64)

	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cl_policy_confidence_min_at_purchase FROM campaign_purchases WHERE id = 'p2'`,
	).Scan(&newValP2))
	require.False(t, newValP2.Valid, "a row with no legacy value must backfill to NULL, not 0")

	// Round-trip down/up to confirm reversibility.
	require.NoError(t, m.Steps(-1))
	require.NoError(t, m.Steps(1))
}
