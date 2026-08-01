package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// requireTestDB opens the dedicated Postgres test database. It NEVER falls back
// to a default DSN: the devcontainer's DATABASE_URL points at the developer's
// real database, and this package drops and truncates schemas. Tests skip
// unless POSTGRES_TEST_URL is set explicitly (see `make test-postgres`).
func requireTestDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL not set; run `make test-postgres` to exercise the Postgres package against a throwaway database")
	}

	logger := mocks.NewMockLogger()
	db, err := Open(context.Background(), url, logger)
	// Unreachable is a FAILURE, not a skip: the operator explicitly asked for
	// this database. Skipping here would let `make test-postgres` report
	// success while running nothing.
	require.NoError(t, err, "open POSTGRES_TEST_URL %q", url)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// setupTestDB opens the test database, applies embedded migrations, and
// truncates tables seeded by prior tests.
func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db := requireTestDB(t)
	require.NoError(t, RunMigrations(db, ""), "run embedded migrations")
	_, err := db.ExecContext(context.Background(), `TRUNCATE TABLE campaigns RESTART IDENTITY CASCADE`)
	require.NoError(t, err, "truncate campaigns")
	return db
}

// resetSchemaAndMigrate returns the test database to a known-good state:
// a fresh public schema migrated to the latest version. Safe to call after a
// test has left schema_migrations dirty, which RunMigrations alone cannot
// recover from.
func resetSchemaAndMigrate(t *testing.T, db *DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err, "reset public schema")
	require.NoError(t, RunMigrations(db, ""), "migrate to latest after reset")
}
