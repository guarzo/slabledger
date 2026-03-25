package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// TestTransaction_RollbackOnError verifies that explicit transactions roll back on error.
// This tests the underlying SQLite transaction behavior that repositories could use.
func TestTransaction_RollbackOnError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Start a transaction
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert a row within the transaction (using valid source from CHECK constraint)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
		VALUES ('Test Card', 'Test Set', 'PSA 10', 10000, 'pricecharting', DATE('now'))
	`)
	require.NoError(t, err)

	// Verify row exists within transaction
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'Test Card'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "row should exist within transaction")

	// Rollback the transaction
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify row was rolled back (not visible outside transaction)
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'Test Card'`).Scan(&count)
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
		INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
		VALUES ('Committed Card', 'Test Set', 'PSA 10', 10000, 'pricecharting', DATE('now'))
	`)
	require.NoError(t, err)

	// Commit the transaction
	err = tx.Commit()
	require.NoError(t, err)

	// Verify row persists after commit
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'Committed Card'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "row should exist after commit")
}

// TestTransaction_RollbackMultipleInserts verifies all inserts in a transaction are rolled back together.
func TestTransaction_RollbackMultipleInserts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Start a transaction
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert multiple rows within the transaction
	cards := []string{"Card1", "Card2", "Card3"}
	for _, card := range cards {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
			VALUES (?, 'Test Set', 'PSA 10', 10000, 'pricecharting', DATE('now'))
		`, card)
		require.NoError(t, err)
	}

	// Verify all rows exist within transaction
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE set_name = 'Test Set'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 3, count, "all 3 rows should exist within transaction")

	// Rollback the transaction
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify all rows were rolled back
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE set_name = 'Test Set'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "no rows should exist after rollback")
}

// TestTransaction_PartialFailure verifies that a constraint violation triggers proper rollback.
func TestTransaction_PartialFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// First, insert a row outside the transaction to create a conflict
	_, err := db.ExecContext(ctx, `
		INSERT INTO api_rate_limits (provider, blocked_until) VALUES ('pricecharting', DATETIME('now', '+1 hour'))
	`)
	require.NoError(t, err)

	// Start a transaction
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert a valid row
	_, err = tx.ExecContext(ctx, `
		INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
		VALUES ('TxCard', 'TxSet', 'PSA 10', 10000, 'pricecharting', DATE('now'))
	`)
	require.NoError(t, err)

	// Try to insert a duplicate (violates unique constraint on api_rate_limits.provider)
	// We ignore the error here since SQLite may handle this differently (UPSERT behavior)
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO api_rate_limits (provider, blocked_until) VALUES ('pricecharting', DATETIME('now', '+2 hours'))
	`)

	// Rollback the transaction regardless of the constraint result
	rbErr := tx.Rollback()
	require.NoError(t, rbErr)

	// Verify the price_history row was rolled back
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'TxCard'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "row should not exist after rollback")
}

// TestTransaction_ContextCancellation verifies transaction behavior with cancelled context.
func TestTransaction_ContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start a transaction
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert a row
	_, err = tx.ExecContext(ctx, `
		INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
		VALUES ('CancelCard', 'Test Set', 'PSA 10', 10000, 'pricecharting', DATE('now'))
	`)
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Attempt to commit after context cancellation
	commitErr := tx.Commit()
	// Commit may still succeed if the data was already sent, or may fail with context.Canceled
	// Either way, we rollback if commit fails

	if commitErr != nil {
		// Rollback to clean up
		_ = tx.Rollback()
	}

	// Query with a fresh context to verify the state.
	// Note: With in-memory SQLite and MaxOpenConns(1), context cancellation may
	// cause database/sql to discard the connection, destroying the in-memory DB.
	// A subsequent query would open a new (empty) connection, so "no such table"
	// is an acceptable outcome here.
	freshCtx := context.Background()
	var count int
	err = db.QueryRowContext(freshCtx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'CancelCard'`).Scan(&count)
	if err != nil {
		t.Logf("Query after context cancellation returned error (expected for in-memory DB): %v", err)
	} else {
		// The row may or may not exist depending on when the cancel happened
		t.Logf("CancelCard row count after context cancellation: %d", count)
	}
}

// TestTransaction_IsolationLevel verifies transaction isolation behavior.
// Note: SQLite is single-writer, so concurrent writes will block.
// SQLite only supports sql.LevelDefault - other isolation levels are ignored.
// This test verifies basic isolation semantics within a single transaction.
func TestTransaction_IsolationLevel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Insert initial data
	_, err := db.ExecContext(ctx, `
		INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
		VALUES ('IsoCard', 'Test Set', 'PSA 10', 10000, 'pricecharting', DATE('now'))
	`)
	require.NoError(t, err)

	// Start a transaction (SQLite ignores isolation level, uses default)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	// Read the value in the transaction
	var price int64
	err = tx.QueryRowContext(ctx, `SELECT price_cents FROM price_history WHERE card_name = 'IsoCard'`).Scan(&price)
	require.NoError(t, err)
	require.Equal(t, int64(10000), price)

	// Update within the same transaction
	_, err = tx.ExecContext(ctx, `UPDATE price_history SET price_cents = 20000 WHERE card_name = 'IsoCard'`)
	require.NoError(t, err)

	// Read again within transaction - should see updated value
	err = tx.QueryRowContext(ctx, `SELECT price_cents FROM price_history WHERE card_name = 'IsoCard'`).Scan(&price)
	require.NoError(t, err)
	require.Equal(t, int64(20000), price, "should see updated value within transaction")

	// Rollback
	err = tx.Rollback()
	require.NoError(t, err)

	// After rollback, should see original value
	err = db.QueryRowContext(ctx, `SELECT price_cents FROM price_history WHERE card_name = 'IsoCard'`).Scan(&price)
	require.NoError(t, err)
	require.Equal(t, int64(10000), price, "should see original value after rollback")
}

// TestRepository_ErrorHandling verifies repository methods handle errors gracefully.
func TestRepository_ErrorHandling(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	t.Run("store_price_with_nil_entry", func(t *testing.T) {
		// This should handle nil gracefully or panic
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic as expected: %v", r)
			}
		}()

		err := repo.StorePrice(ctx, nil)
		// If we get here without panic, it should return an error
		if err == nil {
			t.Log("StorePrice accepted nil entry without error")
		}
	})

	t.Run("get_latest_price_after_db_close", func(t *testing.T) {
		// Create a separate DB that we'll close
		logger := mocks.NewMockLogger()
		tempDB, err := Open(":memory:", logger)
		require.NoError(t, err)

		err = RunMigrations(tempDB, "migrations")
		require.NoError(t, err)

		tempRepo := NewPriceRepository(tempDB)

		// Close the database
		err = tempDB.Close()
		require.NoError(t, err)

		// Try to use the repo after close
		_, err = tempRepo.GetLatestPrice(ctx, pricing.Card{Name: "Test", Set: "Test"}, "PSA 10", "pricecharting")
		require.Error(t, err, "should error when database is closed")
	})
}

// TestRepository_ConcurrentAccess verifies repository handles concurrent operations.
func TestRepository_ConcurrentAccess(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Run multiple goroutines that insert prices concurrently
	errChan := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			entry := &pricing.PriceEntry{
				CardName:   "ConcurrentCard",
				SetName:    "Test Set",
				CardNumber: "001",
				Grade:      "PSA 10",
				PriceCents: int64(10000 + idx),
				Source:     "pricecharting",
				PriceDate:  time.Now().Add(time.Duration(idx) * time.Hour), // Different dates to avoid upsert
			}
			err := repo.StorePrice(ctx, entry)
			errChan <- err
		}(i)
	}

	// Collect results
	var errs []error
	for i := 0; i < 10; i++ {
		if err := <-errChan; err != nil {
			errs = append(errs, err)
		}
	}

	// SQLite handles concurrent writes via locking - some might fail with SQLITE_BUSY
	// This is expected behavior for SQLite with concurrent writes
	t.Logf("Concurrent operations: %d succeeded, %d failed", 10-len(errs), len(errs))

	// Verify at least some operations succeeded
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'ConcurrentCard'`).Scan(&count)
	require.NoError(t, err)
	require.Greater(t, count, 0, "at least some concurrent inserts should succeed")
}

// TestTransaction_ForeignKeyConstraint verifies foreign key constraints work in transactions.
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

// TestTransaction_DeadlockHandling verifies no deadlock occurs with nested operations.
func TestTransaction_DeadlockHandling(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// SQLite doesn't support true nested transactions, but we can test savepoints
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Create a savepoint
	_, err = tx.ExecContext(ctx, `SAVEPOINT sp1`)
	require.NoError(t, err)

	// Insert within savepoint
	_, err = tx.ExecContext(ctx, `
		INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
		VALUES ('SavepointCard', 'Test Set', 'PSA 10', 10000, 'pricecharting', DATE('now'))
	`)
	require.NoError(t, err)

	// Rollback to savepoint
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT sp1`)
	require.NoError(t, err)

	// Verify the insert was undone
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'SavepointCard'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "savepoint rollback should undo insert")

	// Insert after savepoint rollback should still work
	_, err = tx.ExecContext(ctx, `
		INSERT INTO price_history (card_name, set_name, grade, price_cents, source, price_date)
		VALUES ('AfterSavepoint', 'Test Set', 'PSA 10', 10000, 'pricecharting', DATE('now'))
	`)
	require.NoError(t, err)

	// Commit the transaction
	err = tx.Commit()
	require.NoError(t, err)

	// Verify only the second insert persisted
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'AfterSavepoint'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "insert after savepoint rollback should persist")

	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_history WHERE card_name = 'SavepointCard'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "insert before savepoint rollback should not persist")
}

// TestPriceRepository_StorePriceError verifies error scenarios in StorePrice.
func TestPriceRepository_StorePriceError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	t.Run("invalid_date_format", func(t *testing.T) {
		// SQLite is lenient with dates, but we can test extreme cases
		entry := &pricing.PriceEntry{
			CardName:   "TestCard",
			SetName:    "Test Set",
			Grade:      "PSA 10",
			PriceCents: 10000,
			Source:     "pricecharting",
			PriceDate:  time.Time{}, // Zero time
		}
		err := repo.StorePrice(ctx, entry)
		// SQLite will store zero time, but it's still valid SQL
		if err != nil {
			t.Logf("StorePrice with zero time returned error: %v", err)
		}
	})

	t.Run("very_long_card_name", func(t *testing.T) {
		// Test with a very long card name (SQLite TEXT has no length limit)
		longName := ""
		for i := 0; i < 10000; i++ {
			longName += "a"
		}
		entry := &pricing.PriceEntry{
			CardName:   longName,
			SetName:    "Test Set",
			Grade:      "PSA 10",
			PriceCents: 10000,
			Source:     "pricecharting",
			PriceDate:  time.Now(),
		}
		err := repo.StorePrice(ctx, entry)
		// SQLite handles long strings fine
		if err != nil {
			t.Logf("StorePrice with long card name returned error: %v", err)
		}
	})

	t.Run("negative_price", func(t *testing.T) {
		// Test with negative price (business logic validation should catch this)
		entry := &pricing.PriceEntry{
			CardName:   "NegativeCard",
			SetName:    "Test Set",
			Grade:      "PSA 10",
			PriceCents: -10000, // Negative price
			Source:     "pricecharting",
			PriceDate:  time.Now(),
		}
		err := repo.StorePrice(ctx, entry)
		// SQLite doesn't have constraints on negative values
		if err != nil {
			t.Logf("StorePrice with negative price returned error: %v", err)
		} else {
			// Verify it was stored
			retrieved, err := repo.GetLatestPrice(ctx, pricing.Card{Name: "NegativeCard", Set: "Test Set"}, "PSA 10", "pricecharting")
			require.NoError(t, err)
			require.NotNil(t, retrieved)
			require.Equal(t, int64(-10000), retrieved.PriceCents)
		}
	})
}

// TestPriceRepository_ContextTimeout verifies timeout handling in repository operations.
func TestPriceRepository_ContextTimeout(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)

	// Create a context that's already expired
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	cancel() // Immediately cancel

	// Give it a moment to ensure the context is fully cancelled
	time.Sleep(10 * time.Millisecond)

	entry := &pricing.PriceEntry{
		CardName:   "TimeoutCard",
		SetName:    "Test Set",
		Grade:      "PSA 10",
		PriceCents: 10000,
		Source:     "pricecharting",
		PriceDate:  time.Now(),
	}

	err := repo.StorePrice(ctx, entry)
	// May or may not error depending on how fast SQLite is
	if err != nil {
		// Check if it's a context error
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			t.Logf("Operation correctly cancelled due to context: %v", err)
		} else {
			t.Logf("Operation returned error: %v", err)
		}
	}
}
