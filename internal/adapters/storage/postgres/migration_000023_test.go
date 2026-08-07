package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMigration000023_AttributionSourceCheck exercises the
// campaign_purchases_attribution_source_check constraint added by migration
// 000023: NULL and each of the three domain values ('psa', 'inferred',
// 'manual') must be accepted, and any other value must be rejected. The
// constraint intentionally allows NULL (no NOT NULL/DEFAULT) — NULL is the
// canary for a write path that forgot to set attribution_source.
func TestMigration000023_AttributionSourceCheck(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	// Reset to a known-empty baseline first: RunMigrations only steps forward
	// from the tracked version, so a stale schema left at v23 by an earlier
	// test run (before this migration was amended) would silently skip the
	// now-added constraint instead of re-applying it.
	_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err, "reset public schema")
	require.NoError(t, RunMigrations(db, ""), "migrate to latest")

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp-000023', 'Test Campaign')`)
	require.NoError(t, err)

	insert := func(id, attributionSource string, null bool) error {
		if null {
			_, err := db.ExecContext(ctx, `
				INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents)
				VALUES ($1, 'camp-000023', 'Card', $2, '2026-01-01', 5000)`,
				id, id)
			return err
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date, cl_value_at_purchase_cents, attribution_source)
			VALUES ($1, 'camp-000023', 'Card', $2, '2026-01-01', 5000, $3)`,
			id, id, attributionSource)
		return err
	}

	t.Run("accepts NULL", func(t *testing.T) {
		require.NoError(t, insert("p-null", "", true))
	})

	for _, valid := range []string{"psa", "inferred", "manual"} {
		t.Run("accepts "+valid, func(t *testing.T) {
			require.NoError(t, insert("p-"+valid, valid, false))
		})
	}

	t.Run("rejects out-of-domain value", func(t *testing.T) {
		err := insert("p-bad", "bogus", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "campaign_purchases_attribution_source_check")
	})
}
