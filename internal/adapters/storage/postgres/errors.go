package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueConstraintError reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505). Used by stores that translate collisions into
// domain-level "already exists" errors.
func isUniqueConstraintError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
