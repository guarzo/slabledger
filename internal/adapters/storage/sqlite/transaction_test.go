package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTransaction_ForeignKeyConstraint verifies foreign key constraints are enabled.
func TestTransaction_ForeignKeyConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Verify foreign keys are enabled
	var fkEnabled int
	err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fkEnabled)
	require.NoError(t, err)
	require.Equal(t, 1, fkEnabled, "foreign keys should be enabled")
}

// TestTransaction_RollbackOnError verifies that explicit transactions roll back on error.
func TestTransaction_RollbackOnError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Start a transaction
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert a row within the transaction (api_rate_limits survives migration 000038)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO api_rate_limits (provider, blocked_until)
		VALUES ('txtest_provider', DATETIME('now', '+1 hour'))
	`)
	require.NoError(t, err)

	// Verify row exists within transaction
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_rate_limits WHERE provider = 'txtest_provider'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "row should exist within transaction")

	// Rollback the transaction
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify row was rolled back (not visible outside transaction)
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_rate_limits WHERE provider = 'txtest_provider'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "row should not exist after rollback")
}

// TestTransaction_CommitSuccess verifies that committed transactions persist.
func TestTransaction_CommitSuccess(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Start a transaction
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert a row within the transaction
	_, err = tx.ExecContext(ctx, `
		INSERT INTO api_rate_limits (provider, blocked_until)
		VALUES ('commit_provider', DATETIME('now', '+1 hour'))
	`)
	require.NoError(t, err)

	// Commit the transaction
	err = tx.Commit()
	require.NoError(t, err)

	// Verify row persists after commit
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_rate_limits WHERE provider = 'commit_provider'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "row should exist after commit")
}
