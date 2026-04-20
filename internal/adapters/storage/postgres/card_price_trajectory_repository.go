package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
)

// Compile-time interface check.
var _ intelligence.TrajectoryRepository = (*CardPriceTrajectoryRepository)(nil)

// weekStartFormat is the storage format for Monday-00:00-UTC anchor dates.
// ISO-8601 keeps lexical order aligned with chronological order.
const weekStartFormat = "2006-01-02"

// CardPriceTrajectoryRepository provides access to the card_price_trajectory table.
type CardPriceTrajectoryRepository struct {
	db *sql.DB
}

// NewCardPriceTrajectoryRepository creates a new repository.
func NewCardPriceTrajectoryRepository(db *sql.DB) *CardPriceTrajectoryRepository {
	return &CardPriceTrajectoryRepository{db: db}
}

// Upsert writes the given buckets for a card. Existing rows with matching
// (dh_card_id, week_start) are overwritten.
func (r *CardPriceTrajectoryRepository) Upsert(ctx context.Context, dhCardID string, buckets []intelligence.WeeklyBucket) error {
	if dhCardID == "" || len(buckets) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO card_price_trajectory (
			dh_card_id, week_start, sale_count, avg_price_cents, median_price_cents, refreshed_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(dh_card_id, week_start) DO UPDATE SET
			sale_count = excluded.sale_count,
			avg_price_cents = excluded.avg_price_cents,
			median_price_cents = excluded.median_price_cents,
			refreshed_at = excluded.refreshed_at`)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now()
	for _, b := range buckets {
		if _, err := stmt.ExecContext(ctx,
			dhCardID,
			b.WeekStart.UTC().Format(weekStartFormat),
			b.SaleCount,
			b.AvgPriceCents,
			b.MedianPriceCents,
			now,
		); err != nil {
			return fmt.Errorf("upsert trajectory row: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit trajectory upsert: %w", err)
	}
	return nil
}

// GetByDHCardID returns buckets for a card in chronological order.
func (r *CardPriceTrajectoryRepository) GetByDHCardID(ctx context.Context, dhCardID string) (_ []intelligence.WeeklyBucket, err error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT week_start, sale_count, avg_price_cents, median_price_cents
		FROM card_price_trajectory
		WHERE dh_card_id = $1
		ORDER BY week_start ASC`, dhCardID)
	if err != nil {
		return nil, fmt.Errorf("query trajectory: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var out []intelligence.WeeklyBucket
	for rows.Next() {
		var (
			weekStart string
			b         intelligence.WeeklyBucket
		)
		if err := rows.Scan(&weekStart, &b.SaleCount, &b.AvgPriceCents, &b.MedianPriceCents); err != nil {
			return nil, fmt.Errorf("scan trajectory row: %w", err)
		}
		t, parseErr := time.Parse(weekStartFormat, weekStart)
		if parseErr != nil {
			return nil, fmt.Errorf("parse week_start %q: %w", weekStart, parseErr)
		}
		b.WeekStart = t
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trajectory rows: %w", err)
	}
	return out, nil
}

// LatestWeekStart returns the most recent week_start stored for the card, or
// zero time when nothing is stored.
func (r *CardPriceTrajectoryRepository) LatestWeekStart(ctx context.Context, dhCardID string) (time.Time, error) {
	var ts sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(week_start) FROM card_price_trajectory WHERE dh_card_id = $1`,
		dhCardID,
	).Scan(&ts)
	if err != nil {
		return time.Time{}, fmt.Errorf("query latest week_start: %w", err)
	}
	if !ts.Valid || ts.String == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(weekStartFormat, ts.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse week_start %q: %w", ts.String, err)
	}
	return t, nil
}
