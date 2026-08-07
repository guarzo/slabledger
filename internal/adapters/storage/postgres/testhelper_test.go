package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// TestMain rebuilds the test database from an empty schema before any test
// runs.
//
// Without this, the package's only schema setup is setupTestDB's RunMigrations,
// and golang-migrate skips versions the database has already applied. Editing
// an existing migration file — appending a constraint to the migration that
// introduced its column, say — therefore has no effect on a local database that
// already recorded that version, and the whole suite passes against schema that
// does not match the files in the tree. CI starts from an empty database and
// does apply the edit, so the failure surfaces there instead. That gap cost a
// green local `make test-postgres` and a red CI run on PR #538.
//
// Resetting here rather than in the Makefile covers a direct
// `go test ./internal/adapters/storage/postgres/...` with POSTGRES_TEST_URL set,
// which is how the same stale schema was read a second time. Once per package
// run, not once per test: the 28 setupTestDB callers keep their cheap truncate.
//
// Dropping the schema outright is safe because this package already refuses to
// run anywhere but the dedicated throwaway database — see requireTestDB.
func TestMain(m *testing.M) {
	if err := resetTestSchema(); err != nil {
		fmt.Fprintf(os.Stderr, "postgres test setup: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// resetTestSchema drops and re-migrates POSTGRES_TEST_URL's public schema. A
// no-op when the variable is unset, which is the ordinary `go test ./...` case:
// every test in this package skips, so there is nothing to prepare.
func resetTestSchema() error {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		return nil
	}
	ctx := context.Background()
	db, err := Open(ctx, url, mocks.NewMockLogger())
	if err != nil {
		return fmt.Errorf("open POSTGRES_TEST_URL %q: %w", url, err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		return fmt.Errorf("reset public schema: %w", err)
	}
	if err := RunMigrations(db, ""); err != nil {
		return fmt.Errorf("migrate from empty schema: %w", err)
	}
	return nil
}

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
