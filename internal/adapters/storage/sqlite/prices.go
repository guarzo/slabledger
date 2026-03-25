package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PriceRepository implements pricing.PriceRepository interface using SQLite
type PriceRepository struct {
	db     *DB
	logger observability.Logger
}

// Compile-time interface check
var _ pricing.PriceRepository = (*PriceRepository)(nil)

// NewPriceRepository creates a new price repository
func NewPriceRepository(db *DB) *PriceRepository {
	ctx := context.Background()
	return &PriceRepository{
		db:     db,
		logger: db.logger.With(ctx, observability.String("component", "price_repo")),
	}
}

// StorePrice stores or updates a price entry
func (r *PriceRepository) StorePrice(ctx context.Context, entry *pricing.PriceEntry) error {

	query := `
		INSERT INTO price_history (
			card_name, set_name, card_number, grade,
			price_cents, confidence, source,
			fusion_source_count, fusion_outliers_removed, fusion_method,
			price_date, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(card_name, set_name, card_number, grade, source, price_date)
		DO UPDATE SET
			price_cents = excluded.price_cents,
			confidence = excluded.confidence,
			fusion_source_count = excluded.fusion_source_count,
			fusion_outliers_removed = excluded.fusion_outliers_removed,
			fusion_method = excluded.fusion_method,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := r.db.ExecContext(ctx, query,
		entry.CardName,
		entry.SetName,
		entry.CardNumber,
		entry.Grade,
		entry.PriceCents,
		entry.Confidence,
		entry.Source,
		entry.FusionSourceCount,
		entry.FusionOutliersRemoved,
		entry.FusionMethod,
		entry.PriceDate,
	)

	if err != nil {
		r.logger.Error(ctx, "failed to store price",
			observability.Err(err),
			observability.String("card", entry.CardName),
			observability.String("source", entry.Source))
		return fmt.Errorf("store price: %w", err)
	}

	return nil
}

// GetLatestPrice retrieves the most recent price for a card.
// When card.Number is provided, it is included in the WHERE clause to avoid
// returning prices for a different card variant that shares the same name and set.
func (r *PriceRepository) GetLatestPrice(ctx context.Context, card pricing.Card, grade string, source string) (*pricing.PriceEntry, error) {

	var entry pricing.PriceEntry
	entry.CardName = card.Name
	entry.SetName = card.Set
	entry.Grade = grade
	entry.Source = source

	baseQuery := `
		SELECT
			card_number,
			price_cents,
			confidence,
			fusion_source_count,
			fusion_outliers_removed,
			fusion_method,
			price_date,
			created_at,
			updated_at
		FROM price_history
		WHERE card_name = ? AND set_name = ? AND grade = ? AND source = ?`

	args := []interface{}{card.Name, card.Set, grade, source}

	if card.Number != "" {
		baseQuery += " AND card_number = ?"
		args = append(args, card.Number)
	}

	baseQuery += `
		ORDER BY price_date DESC, updated_at DESC
		LIMIT 1
	`

	err := r.db.QueryRowContext(ctx, baseQuery, args...).Scan(
		&entry.CardNumber,
		&entry.PriceCents,
		&entry.Confidence,
		&entry.FusionSourceCount,
		&entry.FusionOutliersRemoved,
		&entry.FusionMethod,
		&entry.PriceDate,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No cached price
	}
	if err != nil {
		r.logger.Error(ctx, "failed to get latest price",
			observability.Err(err),
			observability.String("card", card.Name))
		return nil, fmt.Errorf("get latest price: %w", err)
	}

	return &entry, nil
}

// GetLatestPricesBySource retrieves the latest price entry per grade for a given
// card/source combination, filtering to entries updated within maxAge.
// Returns a map keyed by grade (e.g. "PSA 10", "Raw"). Only the most recent
// entry per grade is kept (ORDER BY price_date DESC, updated_at DESC).
func (r *PriceRepository) GetLatestPricesBySource(ctx context.Context, cardName, setName, cardNumber, source string, maxAge time.Duration) (map[string]pricing.PriceEntry, error) {
	cutoff := time.Now().UTC().Add(-maxAge).Format("2006-01-02 15:04:05")

	query := `
		SELECT grade, card_number, price_cents, confidence,
		       fusion_source_count, fusion_outliers_removed, fusion_method,
		       price_date, created_at, updated_at
		FROM price_history
		WHERE card_name = ? AND set_name = ? AND source = ?
		  AND updated_at >= ?`

	args := []interface{}{cardName, setName, source, cutoff}

	if cardNumber != "" {
		query += " AND card_number = ?"
		args = append(args, cardNumber)
	}

	query += `
		ORDER BY grade, price_date DESC, updated_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.logger.Error(ctx, "failed to get latest prices by source",
			observability.Err(err),
			observability.String("card", cardName),
			observability.String("source", source))
		return nil, fmt.Errorf("get latest prices by source: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.Warn(ctx, "failed to close rows", observability.Err(err))
		}
	}()

	result := make(map[string]pricing.PriceEntry)
	for rows.Next() {
		var entry pricing.PriceEntry
		if err := rows.Scan(
			&entry.Grade,
			&entry.CardNumber,
			&entry.PriceCents,
			&entry.Confidence,
			&entry.FusionSourceCount,
			&entry.FusionOutliersRemoved,
			&entry.FusionMethod,
			&entry.PriceDate,
			&entry.CreatedAt,
			&entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan price by source row: %w", err)
		}
		// Keep only the first (latest) entry per grade
		if _, exists := result[entry.Grade]; !exists {
			entry.CardName = cardName
			entry.SetName = setName
			entry.Source = source
			result[entry.Grade] = entry
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prices by source rows: %w", err)
	}

	return result, nil
}

// DeletePricesByCard removes all price history entries for a specific card identity.
// When cardNumber is non-empty, only the specific variant is deleted.
func (r *PriceRepository) DeletePricesByCard(ctx context.Context, cardName, setName, cardNumber string) (int64, error) {
	query := `DELETE FROM price_history WHERE card_name = ? AND set_name = ?`
	args := []any{cardName, setName}
	if cardNumber != "" {
		query += " AND card_number = ?"
		args = append(args, cardNumber)
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		r.logger.Error(ctx, "failed to delete prices by card",
			observability.Err(err),
			observability.String("card", cardName),
			observability.String("set", setName),
			observability.String("number", cardNumber))
		return 0, fmt.Errorf("delete prices by card: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return deleted, nil
}

// GetStalePrices retrieves prices that need refreshing based on the stale_prices VIEW.
// Staleness thresholds are value-based and defined in the VIEW:
// - High value (>$100): stale after 12 hours
// - Medium value ($50-$100): stale after 24 hours
// - Low value (<$50): stale after 48 hours
func (r *PriceRepository) GetStalePrices(ctx context.Context, source string, limit int) ([]pricing.StalePrice, error) {

	args := []interface{}{}

	sourceFilter := ""
	if source != "" {
		sourceFilter = " WHERE source = ?"
		args = append(args, source)
	}

	query := `
		SELECT card_name, card_number, set_name, grade, source,
		       hours_old, price_cents, priority, recently_accessed, psa_listing_title
		FROM (
			SELECT
				card_name, card_number, set_name, grade, source,
				hours_old, price_cents, priority, recently_accessed, updated_at,
				psa_listing_title,
				ROW_NUMBER() OVER (
					PARTITION BY card_name, COALESCE(card_number, ''), set_name
					ORDER BY recently_accessed DESC, priority ASC, updated_at ASC
				) AS rn
			FROM stale_prices` + sourceFilter + `
		) sub
		WHERE rn = 1
	`

	// Apply ordering: recently accessed first, then by priority (high value first), then oldest first
	query += " ORDER BY recently_accessed DESC, priority ASC, updated_at ASC"

	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.logger.Error(ctx, "failed to get stale prices",
			observability.Err(err))
		return nil, fmt.Errorf("get stale prices: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.Warn(ctx, "failed to close rows", observability.Err(err))
		}
	}()

	// H.4: Pre-allocate slice with known capacity from LIMIT clause to avoid repeated growth.
	stalePrices := make([]pricing.StalePrice, 0, limit)
	for rows.Next() {
		var sp pricing.StalePrice
		var recentlyAccessed int
		var cardNumber sql.NullString
		if err := rows.Scan(
			&sp.CardName,
			&cardNumber,
			&sp.SetName,
			&sp.Grade,
			&sp.Source,
			&sp.HoursOld,
			&sp.LastPrice,
			&sp.Priority,
			&recentlyAccessed,
			&sp.PSAListingTitle,
		); err != nil {
			return nil, fmt.Errorf("scan stale price row: %w", err)
		}
		sp.CardNumber = cardNumber.String // Empty string if NULL
		sp.DaysOld = sp.HoursOld / 24.0
		stalePrices = append(stalePrices, sp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale price rows: %w", err)
	}

	r.logger.Debug(ctx, "retrieved stale prices",
		observability.Int("count", len(stalePrices)))

	return stalePrices, nil
}
