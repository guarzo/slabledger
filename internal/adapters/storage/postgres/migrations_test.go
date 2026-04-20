package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrations_UpDownUpRoundtrip exercises the full migration cycle against a
// real Postgres instance: fresh schema → Up → Down → Up. Each step must succeed
// and leave the DB in the expected shape. Catches broken SQL in either
// direction, missing DROPs in down migrations, and type mismatches between up
// and down.
//
// This test explicitly drops and recreates the public schema at the start to
// guarantee a known baseline. It does NOT use t.Parallel — its schema-drop
// would interfere with any parallel test that expects state.
func TestMigrations_UpDownUpRoundtrip(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		url = "postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable"
	}

	logger := mocks.NewMockLogger()
	db, err := Open(url, logger)
	if err != nil {
		t.Skipf("Postgres not reachable at %q: %v (set POSTGRES_TEST_URL to override)", url, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()

	// Guaranteed-empty baseline.
	_, err = db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err, "reset public schema")

	// UP #1
	require.NoError(t, RunMigrations(db, ""), "first up migration")
	maxVersion := countEmbeddedMigrations(t)
	version := currentMigrationVersion(t, db)
	assert.Equal(t, maxVersion, version, "after first Up, version should equal max embedded migration")
	assert.Positive(t, countPublicTables(t, db), "after Up, public schema should have at least one user table")

	// DOWN
	require.NoError(t, runMigrationsDown(db), "down migration")

	// After Down, only schema_migrations (golang-migrate's tracking table) should
	// remain in the public schema. All application tables must be dropped.
	remaining := listPublicTables(t, db)
	for _, table := range remaining {
		assert.Equal(t, "schema_migrations", table,
			"unexpected table remaining after Down: %s", table)
	}

	// UP #2 — proves Down didn't leave the DB in a state that blocks a re-up
	require.NoError(t, RunMigrations(db, ""), "second up migration")
	version = currentMigrationVersion(t, db)
	assert.Equal(t, maxVersion, version, "after second Up, version should equal max embedded migration")
	assert.Positive(t, countPublicTables(t, db), "after second Up, public schema should have user tables again")
}

// runMigrationsDown applies all down migrations until the DB is at version 0.
// Mirrors RunMigrations' setup but calls Down() instead of Up().
func runMigrationsDown(db *DB) error {
	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}
	iofsDriver, err := iofs.New(MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", iofsDriver, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create migration instance: %w", err)
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("down: %w", err)
	}
	return nil
}

// countEmbeddedMigrations returns the highest migration number in MigrationsFS,
// by counting *.up.sql files.
func countEmbeddedMigrations(t *testing.T) uint {
	t.Helper()
	entries, err := MigrationsFS.ReadDir("migrations")
	require.NoError(t, err)
	var count uint
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			count++
		}
	}
	return count
}

// currentMigrationVersion returns the migration version recorded by golang-migrate.
func currentMigrationVersion(t *testing.T, db *DB) uint {
	t.Helper()
	var version uint
	var dirty bool
	err := db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	require.NoError(t, err, "read schema_migrations")
	require.False(t, dirty, "schema_migrations.dirty should be false")
	return version
}

// countPublicTables returns the number of BASE TABLE rows in the public schema.
func countPublicTables(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`,
	).Scan(&n)
	require.NoError(t, err)
	return n
}

// listPublicTables returns BASE TABLE names in the public schema.
func listPublicTables(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		 ORDER BY table_name`,
	)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}
