package postgres

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DBTracker is the Postgres counterpart to the sqlite DBTracker. The full
// definition (with its pricing.APITracker / AccessTracker interface methods)
// lands in the wave 3 port of prices.go; for wave 1 we only need the fields
// health_checker.go touches so the postgres package builds.
type DBTracker struct {
	db     *DB
	logger observability.Logger
}

// NewDBTracker creates a new tracker backed by the given database.
func NewDBTracker(db *DB) *DBTracker {
	ctx := context.Background()
	return &DBTracker{
		db:     db,
		logger: db.logger.With(ctx, observability.String("component", "db_tracker")),
	}
}

// Ping checks database connectivity
func (r *DBTracker) Ping(ctx context.Context) error {
	err := r.db.PingContext(ctx)
	if err != nil {
		r.logger.Error(ctx, "database ping failed",
			observability.Err(err),
			observability.String("operation", "health_check"))
		return fmt.Errorf("health check: %w", err)
	}

	r.logger.Debug(ctx, "database ping succeeded",
		observability.String("operation", "health_check"))
	return nil
}
