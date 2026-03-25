package sqlite

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Ping checks database connectivity
func (r *PriceRepository) Ping(ctx context.Context) error {
	err := r.db.PingContext(ctx)
	if err != nil {
		r.logger.Error(ctx, "database ping failed",
			observability.Err(err),
			observability.String("operation", "health_check"))
		return err
	}

	r.logger.Debug(ctx, "database ping succeeded",
		observability.String("operation", "health_check"))
	return nil
}
