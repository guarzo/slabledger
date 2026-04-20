package postgres

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// DBTracker implements pricing.APITracker, pricing.AccessTracker, and
// pricing.HealthChecker using the shared Postgres database handle.
// Previously PriceRepository — renamed after price_history was dropped.
type DBTracker struct {
	db     *DB
	logger observability.Logger
}

// Compile-time interface checks
var _ pricing.APITracker = (*DBTracker)(nil)
var _ pricing.AccessTracker = (*DBTracker)(nil)
var _ pricing.HealthChecker = (*DBTracker)(nil)

// NewDBTracker creates a new tracker backed by the given database.
func NewDBTracker(db *DB) *DBTracker {
	ctx := context.Background()
	return &DBTracker{
		db:     db,
		logger: db.logger.With(ctx, observability.String("component", "db_tracker")),
	}
}
