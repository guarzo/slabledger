package sqlite

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// RecordCardAccess records when a card is accessed for priority refresh
func (r *PriceRepository) RecordCardAccess(ctx context.Context, cardName, setName, accessType string) error {
	query := `
		INSERT INTO card_access_log (card_name, set_name, access_type, accessed_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`

	_, err := r.db.ExecContext(ctx, query, cardName, setName, accessType)
	if err != nil {
		// Non-critical - don't fail if access logging fails
		r.logger.Debug(ctx, "failed to record card access",
			observability.Err(err),
			observability.String("card", cardName))
		return nil
	}

	return nil
}

// CleanupOldAccessLogs removes access logs older than the specified retention period.
// Returns the number of deleted records.
func (r *PriceRepository) CleanupOldAccessLogs(ctx context.Context, retentionDays int) (int64, error) {
	query := `
		DELETE FROM card_access_log
		WHERE accessed_at < DATETIME('now', ? || ' days')
	`

	result, err := r.db.ExecContext(ctx, query, fmt.Sprintf("-%d", retentionDays))
	if err != nil {
		r.logger.Error(ctx, "failed to cleanup old access logs",
			observability.Err(err),
			observability.Int("retention_days", retentionDays))
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		r.logger.Warn(ctx, "failed to get rows affected count",
			observability.Err(err))
		return 0, nil
	}

	if rowsAffected > 0 {
		r.logger.Info(ctx, "cleaned up old access logs",
			observability.Int("deleted", int(rowsAffected)),
			observability.Int("retention_days", retentionDays))
	}

	return rowsAffected, nil
}
