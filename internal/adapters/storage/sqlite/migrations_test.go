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
		"api_calls",
		"api_rate_limits",
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

	t.Run("api_calls_schema", func(t *testing.T) {
		rows, err := db.Query("PRAGMA table_info(api_calls)")
		require.NoError(t, err)
		defer rows.Close()

		columns := make(map[string]bool)
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dfltValue any
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
			var dfltValue any
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
			"idx_api_calls_provider",
			"idx_api_calls_timestamp",
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

	t.Run("api_rate_limits_primary_key", func(t *testing.T) {
		// doubleholo is seeded by a migration; inserting a duplicate provider must fail
		// with a UNIQUE / PRIMARY KEY constraint violation.
		_, err := db.Exec(`
			INSERT INTO api_rate_limits (provider, calls_last_minute)
			VALUES ('doubleholo', 0)
		`)
		require.Error(t, err, "duplicate provider insert should fail on PRIMARY KEY / UNIQUE constraint")
		require.Contains(t, err.Error(), "UNIQUE constraint failed",
			"expected UNIQUE constraint error, got: %v", err)
	})
}
