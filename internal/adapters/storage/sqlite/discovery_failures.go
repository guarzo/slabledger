package sqlite

import (
	"context"
	"database/sql"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// DiscoveryFailureRepository implements pricing.DiscoveryFailureTracker using SQLite.
type DiscoveryFailureRepository struct {
	db *sql.DB
}

// Compile-time interface check
var _ pricing.DiscoveryFailureTracker = (*DiscoveryFailureRepository)(nil)

// NewDiscoveryFailureRepository creates a new repository backed by the given database.
func NewDiscoveryFailureRepository(db *sql.DB) *DiscoveryFailureRepository {
	return &DiscoveryFailureRepository{db: db}
}

// RecordDiscoveryFailure upserts a discovery failure record. On conflict it increments
// attempts and updates the failure reason, query, and timestamp.
func (r *DiscoveryFailureRepository) RecordDiscoveryFailure(ctx context.Context, f *pricing.DiscoveryFailure) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO discovery_failures (card_name, set_name, card_number, provider, failure_reason, query_attempted, attempts, last_attempted_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(card_name, set_name, card_number, provider)
		DO UPDATE SET
			failure_reason = excluded.failure_reason,
			query_attempted = excluded.query_attempted,
			attempts = discovery_failures.attempts + 1,
			last_attempted_at = CURRENT_TIMESTAMP
	`, f.CardName, f.SetName, f.CardNumber, f.Provider, f.FailureReason, f.Query)
	return err
}

// ClearDiscoveryFailure removes a failure record when a card is successfully discovered.
func (r *DiscoveryFailureRepository) ClearDiscoveryFailure(ctx context.Context, cardName, setName, cardNumber, provider string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM discovery_failures
		WHERE card_name = ? AND set_name = ? AND card_number = ? AND provider = ?
	`, cardName, setName, cardNumber, provider)
	return err
}

// ListDiscoveryFailures returns recent discovery failures for a provider, ordered by most recent first.
func (r *DiscoveryFailureRepository) ListDiscoveryFailures(ctx context.Context, provider string, limit int) ([]pricing.DiscoveryFailure, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT card_name, set_name, card_number, provider, failure_reason, query_attempted, attempts, last_attempted_at, created_at
		FROM discovery_failures
		WHERE provider = ?
		ORDER BY last_attempted_at DESC
		LIMIT ?
	`, provider, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var results []pricing.DiscoveryFailure
	for rows.Next() {
		var f pricing.DiscoveryFailure
		var lastAttempted, created string
		if err := rows.Scan(&f.CardName, &f.SetName, &f.CardNumber, &f.Provider, &f.FailureReason, &f.Query, &f.Attempts, &lastAttempted, &created); err != nil {
			return nil, err
		}
		f.LastAttempted = parseSQLiteTime(lastAttempted)
		f.CreatedAt = parseSQLiteTime(created)
		results = append(results, f)
	}
	return results, rows.Err()
}

// CountDiscoveryFailures returns the total number of discovery failures for a provider.
func (r *DiscoveryFailureRepository) CountDiscoveryFailures(ctx context.Context, provider string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM discovery_failures WHERE provider = ?
	`, provider).Scan(&count)
	return count, err
}
