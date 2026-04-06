package sqlite

import (
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	logger := mocks.NewMockLogger()

	db, err := Open(":memory:", logger)
	require.NoError(t, err)

	err = RunMigrations(db, "migrations")
	require.NoError(t, err)

	return db
}
