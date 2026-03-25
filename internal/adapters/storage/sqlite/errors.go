package sqlite

import (
	"errors"
	"strings"

	"github.com/mattn/go-sqlite3"
)

// isUniqueConstraintError checks if err is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
	}
	// Fallback for wrapped errors that lose type info
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
