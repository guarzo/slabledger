package sqlite

import (
	"os"
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDB_Open(t *testing.T) {
	logger := mocks.NewMockLogger()

	// Test in-memory database
	db, err := Open(":memory:", logger)
	require.NoError(t, err)
	require.NotNil(t, db)

	// Test ping
	err = db.Ping()
	require.NoError(t, err)

	// Test close
	err = db.Close()
	require.NoError(t, err)
}

func TestDB_Open_FileDatabase(t *testing.T) {
	logger := mocks.NewMockLogger()

	// Test file database
	tmpFile := "/tmp/test_slabledger.db"
	t.Cleanup(func() {
		// Remove main DB file and WAL/SHM counterparts; ignore errors for missing files
		_ = os.Remove(tmpFile)
		_ = os.Remove(tmpFile + "-wal")
		_ = os.Remove(tmpFile + "-shm")
	})

	db, err := Open(tmpFile, logger)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	// Verify WAL mode
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	require.Equal(t, "wal", journalMode)

	// Verify foreign keys
	var foreignKeys int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys)
	require.NoError(t, err)
	require.Equal(t, 1, foreignKeys)
}
