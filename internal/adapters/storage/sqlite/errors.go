package sqlite

import (
	"strings"
)

// isUniqueConstraintError checks if err is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	// Check error message for UNIQUE constraint violation
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
