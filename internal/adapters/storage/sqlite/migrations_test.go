package sqlite

import (
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	logger := mocks.NewMockLogger()
	db, err := Open(":memory:", logger)
	require.NoError(t, err)
	return db
}

func TestRunMigrations(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	err := RunMigrations(db, "migrations")
	require.NoError(t, err)

	// Verify all tables exist
	tables := []string{
		"price_history",
		"api_calls",
		"api_rate_limits",
		"price_refresh_queue",
		"card_access_log",
		"users",
		"user_tokens",
		"user_sessions",
		"oauth_states",
		"favorites",
		"allowed_emails",
		"campaigns",
		"campaign_purchases",
		"campaign_sales",
		"card_id_mappings",
		"sync_state",
	}

	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		require.NoError(t, err, "table %s should exist", table)
		require.Equal(t, table, name)
	}

	// Verify all views exist
	views := []string{
		"stale_prices",
		"api_usage_summary",
		"api_hourly_distribution",
		"api_daily_summary",
		"active_sessions",
		"expired_sessions",
	}
	for _, view := range views {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='view' AND name=?", view).Scan(&name)
		require.NoError(t, err, "view %s should exist", view)
		require.Equal(t, view, name)
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	err := RunMigrations(db, "migrations")
	require.NoError(t, err)

	err = RunMigrations(db, "migrations")
	require.NoError(t, err, "migrations should be idempotent")
}

func TestMigrations_Schema(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	err := RunMigrations(db, "migrations")
	require.NoError(t, err)

	t.Run("price_history_schema", func(t *testing.T) {
		rows, err := db.Query("PRAGMA table_info(price_history)")
		require.NoError(t, err)
		defer rows.Close()

		columns := make(map[string]bool)
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dfltValue interface{}
			err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk)
			require.NoError(t, err)
			columns[name] = true
		}
		require.NoError(t, rows.Err())

		expectedColumns := []string{
			"id", "card_name", "set_name", "card_number", "grade",
			"price_cents", "confidence", "source",
			"fusion_source_count", "fusion_outliers_removed", "fusion_method",
			"price_date", "created_at", "updated_at",
		}
		for _, col := range expectedColumns {
			require.True(t, columns[col], "column %s should exist", col)
		}
	})

	t.Run("api_calls_schema", func(t *testing.T) {
		rows, err := db.Query("PRAGMA table_info(api_calls)")
		require.NoError(t, err)
		defer rows.Close()

		columns := make(map[string]bool)
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dfltValue interface{}
			err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk)
			require.NoError(t, err)
			columns[name] = true
		}
		require.NoError(t, rows.Err())

		expectedColumns := []string{"id", "provider", "endpoint", "status_code", "error", "latency_ms", "timestamp"}
		for _, col := range expectedColumns {
			require.True(t, columns[col], "column %s should exist", col)
		}
	})

	t.Run("campaign_purchases_schema", func(t *testing.T) {
		rows, err := db.Query("PRAGMA table_info(campaign_purchases)")
		require.NoError(t, err)
		defer rows.Close()

		columns := make(map[string]bool)
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dfltValue interface{}
			err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk)
			require.NoError(t, err)
			columns[name] = true
		}
		require.NoError(t, rows.Err())

		expectedColumns := []string{
			"id", "campaign_id", "card_name", "cert_number", "grade_value",
			"cl_value_cents", "buy_cost_cents", "psa_sourcing_fee_cents", "purchase_date",
			"last_sold_cents", "lowest_list_cents", "conservative_cents", "median_cents",
			"active_listings", "sales_last_30d", "trend_30d", "snapshot_date",
		}
		for _, col := range expectedColumns {
			require.True(t, columns[col], "column %s should exist", col)
		}
	})

	t.Run("indexes", func(t *testing.T) {
		indexes := []string{
			"idx_price_history_card",
			"idx_price_history_staleness",
			"idx_price_history_date",
			"idx_price_history_lookup",
			"idx_api_calls_provider",
			"idx_api_calls_timestamp",
			"idx_refresh_queue_priority",
			"idx_access_log_card",
			"idx_access_log_covering",
		}

		for _, index := range indexes {
			var name string
			err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&name)
			require.NoError(t, err, "index %s should exist", index)
		}
	})
}

func TestMigrations_Constraints(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	err := RunMigrations(db, "migrations")
	require.NoError(t, err)

	t.Run("price_history_unique_constraint", func(t *testing.T) {
		_, err := db.Exec(`
			INSERT INTO price_history (card_name, set_name, grade, source, price_cents, price_date)
			VALUES ('Charizard', 'Base Set', 'PSA 10', 'doubleholo', 50000, '2025-01-01')
		`)
		require.NoError(t, err)

		_, err = db.Exec(`
			INSERT INTO price_history (card_name, set_name, grade, source, price_cents, price_date)
			VALUES ('Charizard', 'Base Set', 'PSA 10', 'doubleholo', 60000, '2025-01-01')
		`)
		require.Error(t, err, "should fail due to UNIQUE constraint")
	})

	t.Run("refresh_queue_status_check", func(t *testing.T) {
		_, err := db.Exec(`
			INSERT INTO api_rate_limits (provider, calls_last_minute)
			VALUES ('doubleholo', 0)
		`)
		// doubleholo is seeded by migration 000028, so conflict is expected — that's fine
		_ = err

		_, err = db.Exec(`
			INSERT INTO price_refresh_queue (card_name, set_name, grade, source, status)
			VALUES ('Pikachu', 'Base Set', 'PSA 9', 'doubleholo', 'pending')
		`)
		require.NoError(t, err)

		_, err = db.Exec(`
			INSERT INTO price_refresh_queue (card_name, set_name, grade, source, status)
			VALUES ('Pikachu', 'Base Set', 'PSA 10', 'doubleholo', 'invalid_status')
		`)
		require.Error(t, err, "should fail due to CHECK constraint")
	})
}
