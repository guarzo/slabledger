package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

const defaultTestDatabaseURL = "postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable"

// setupTestDB opens the Postgres test database, applies embedded migrations,
// and truncates tables seeded by prior tests. Tests are skipped if Postgres
// cannot be reached — CI must supply a reachable POSTGRES_TEST_URL.
func setupTestDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		url = defaultTestDatabaseURL
	}

	logger := mocks.NewMockLogger()
	db, err := Open(url, logger)
	if err != nil {
		t.Skipf("Postgres not reachable at %q: %v (set POSTGRES_TEST_URL to override)", url, err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	require.NoError(t, RunMigrations(db, ""), "run embedded migrations")
	_, err = db.ExecContext(context.Background(), `TRUNCATE TABLE campaigns RESTART IDENTITY CASCADE`)
	require.NoError(t, err, "truncate campaigns")

	return db
}
