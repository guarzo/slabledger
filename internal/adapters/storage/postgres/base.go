package postgres

import (
	"database/sql"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// base provides shared db and logger to all specialized Postgres stores.
type base struct {
	db     *sql.DB
	logger observability.Logger
}
