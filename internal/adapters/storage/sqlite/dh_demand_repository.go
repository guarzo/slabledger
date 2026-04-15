package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DHDemandRepository provides access to the dh_card_cache and
// dh_character_cache tables, which persist demand and analytics signals
// fetched from the DoubleHolo enterprise API.
//
// Task 3 will introduce a domain-level interface (in internal/domain/demand)
// that this repository implements. For now it exposes only SQLite-adapter
// row types and CRUD methods.
type DHDemandRepository struct {
	db *sql.DB
}

// NewDHDemandRepository creates a new repository backed by the given database.
func NewDHDemandRepository(db *sql.DB) *DHDemandRepository {
	return &DHDemandRepository{db: db}
}

// DHCardCacheRow is the adapter-level row representation for dh_card_cache.
// Nullable columns map to pointer fields so NULL vs. empty-string can be
// distinguished by callers.
type DHCardCacheRow struct {
	CardID                string
	Window                string // "7d" or "30d"
	DemandScore           *float64
	DemandDataQuality     *string // "proxy" | "full"
	DemandJSON            *string
	VelocityJSON          *string
	TrendJSON             *string
	SaturationJSON        *string
	PriceDistributionJSON *string
	AnalyticsComputedAt   *time.Time
	DemandComputedAt      *time.Time
	FetchedAt             time.Time
}

// DHCharacterCacheRow is the adapter-level row representation for dh_character_cache.
type DHCharacterCacheRow struct {
	Character           string
	Window              string
	DemandJSON          *string
	VelocityJSON        *string
	SaturationJSON      *string
	DemandComputedAt    *time.Time
	AnalyticsComputedAt *time.Time
	FetchedAt           time.Time
}

// DHDemandQualityStats summarises demand_data_quality distribution for a
// given window, used by the daily refresh scheduler metrics.
type DHDemandQualityStats struct {
	ProxyCount       int
	FullCount        int
	NullQualityCount int // rows without demand_data_quality (analytics-only or 404'd)
	TotalRows        int
}

// --- Card cache CRUD ---

// UpsertCardCache inserts or updates a dh_card_cache row keyed by (card_id, window).
func (r *DHDemandRepository) UpsertCardCache(ctx context.Context, row DHCardCacheRow) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO dh_card_cache (
			card_id, window,
			demand_score, demand_data_quality,
			demand_json, velocity_json, trend_json, saturation_json, price_distribution_json,
			analytics_computed_at, demand_computed_at, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(card_id, window) DO UPDATE SET
			demand_score            = excluded.demand_score,
			demand_data_quality     = excluded.demand_data_quality,
			demand_json             = excluded.demand_json,
			velocity_json           = excluded.velocity_json,
			trend_json              = excluded.trend_json,
			saturation_json         = excluded.saturation_json,
			price_distribution_json = excluded.price_distribution_json,
			analytics_computed_at   = excluded.analytics_computed_at,
			demand_computed_at      = excluded.demand_computed_at,
			fetched_at              = excluded.fetched_at`,
		row.CardID, row.Window,
		nullFloat64FromPtr(row.DemandScore), nullStringFromPtr(row.DemandDataQuality),
		nullStringFromPtr(row.DemandJSON), nullStringFromPtr(row.VelocityJSON),
		nullStringFromPtr(row.TrendJSON), nullStringFromPtr(row.SaturationJSON),
		nullStringFromPtr(row.PriceDistributionJSON),
		nullTimeFromPtr(row.AnalyticsComputedAt), nullTimeFromPtr(row.DemandComputedAt),
		row.FetchedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert dh_card_cache: %w", err)
	}
	return nil
}

// GetCardCache returns the cached row for (cardID, window), or (nil, nil) if not found.
func (r *DHDemandRepository) GetCardCache(ctx context.Context, cardID, window string) (*DHCardCacheRow, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT card_id, window,
			demand_score, demand_data_quality,
			demand_json, velocity_json, trend_json, saturation_json, price_distribution_json,
			analytics_computed_at, demand_computed_at, fetched_at
		FROM dh_card_cache
		WHERE card_id = ? AND window = ?`,
		cardID, window,
	)
	result, err := scanCardCacheRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get dh_card_cache: %w", err)
	}
	return result, nil
}

// ListCardCacheByDemandScore returns rows for the given window ordered by
// demand_score DESC. Rows with a NULL demand_score are excluded — they
// cannot be ranked meaningfully and the caller (the niches handler) only
// wants rows that actually carry a demand signal.
func (r *DHDemandRepository) ListCardCacheByDemandScore(ctx context.Context, window string, limit int) (_ []DHCardCacheRow, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT card_id, window,
			demand_score, demand_data_quality,
			demand_json, velocity_json, trend_json, saturation_json, price_distribution_json,
			analytics_computed_at, demand_computed_at, fetched_at
		FROM dh_card_cache
		WHERE window = ? AND demand_score IS NOT NULL
		ORDER BY demand_score DESC
		LIMIT ?`,
		window, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query dh_card_cache by demand_score: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	results := make([]DHCardCacheRow, 0, limit)
	for rows.Next() {
		r, scanErr := scanCardCacheRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan dh_card_cache row: %w", scanErr)
		}
		results = append(results, *r)
	}
	return results, rows.Err()
}

// CardDataQualityStats returns the distribution of demand_data_quality values
// across dh_card_cache rows for a given window.
func (r *DHDemandRepository) CardDataQualityStats(ctx context.Context, window string) (DHDemandQualityStats, error) {
	var stats DHDemandQualityStats
	err := r.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) FILTER (WHERE demand_data_quality = 'proxy') AS proxy_count,
			COUNT(*) FILTER (WHERE demand_data_quality = 'full')  AS full_count,
			COUNT(*) FILTER (WHERE demand_data_quality IS NULL)   AS null_count,
			COUNT(*) AS total
		FROM dh_card_cache
		WHERE window = ?`,
		window,
	).Scan(&stats.ProxyCount, &stats.FullCount, &stats.NullQualityCount, &stats.TotalRows)
	if err != nil {
		return DHDemandQualityStats{}, fmt.Errorf("card data quality stats: %w", err)
	}
	return stats, nil
}

// --- Character cache CRUD ---

// UpsertCharacterCache inserts or updates a dh_character_cache row keyed by (character, window).
func (r *DHDemandRepository) UpsertCharacterCache(ctx context.Context, row DHCharacterCacheRow) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO dh_character_cache (
			character, window,
			demand_json, velocity_json, saturation_json,
			demand_computed_at, analytics_computed_at, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(character, window) DO UPDATE SET
			demand_json           = excluded.demand_json,
			velocity_json         = excluded.velocity_json,
			saturation_json       = excluded.saturation_json,
			demand_computed_at    = excluded.demand_computed_at,
			analytics_computed_at = excluded.analytics_computed_at,
			fetched_at            = excluded.fetched_at`,
		row.Character, row.Window,
		nullStringFromPtr(row.DemandJSON), nullStringFromPtr(row.VelocityJSON),
		nullStringFromPtr(row.SaturationJSON),
		nullTimeFromPtr(row.DemandComputedAt), nullTimeFromPtr(row.AnalyticsComputedAt),
		row.FetchedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert dh_character_cache: %w", err)
	}
	return nil
}

// GetCharacterCache returns the cached row for (character, window), or (nil, nil) if not found.
func (r *DHDemandRepository) GetCharacterCache(ctx context.Context, character, window string) (*DHCharacterCacheRow, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT character, window,
			demand_json, velocity_json, saturation_json,
			demand_computed_at, analytics_computed_at, fetched_at
		FROM dh_character_cache
		WHERE character = ? AND window = ?`,
		character, window,
	)
	result, err := scanCharacterCacheRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get dh_character_cache: %w", err)
	}
	return result, nil
}

// ListCharacterCache returns all character cache rows for the given window,
// ordered by character ascending.
func (r *DHDemandRepository) ListCharacterCache(ctx context.Context, window string) (_ []DHCharacterCacheRow, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT character, window,
			demand_json, velocity_json, saturation_json,
			demand_computed_at, analytics_computed_at, fetched_at
		FROM dh_character_cache
		WHERE window = ?
		ORDER BY character ASC`,
		window,
	)
	if err != nil {
		return nil, fmt.Errorf("query dh_character_cache: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	results := make([]DHCharacterCacheRow, 0, 32)
	for rows.Next() {
		r, scanErr := scanCharacterCacheRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan dh_character_cache row: %w", scanErr)
		}
		results = append(results, *r)
	}
	return results, rows.Err()
}

// --- row scanners ---

func scanCardCacheRow(s scanner) (*DHCardCacheRow, error) {
	var (
		row                   DHCardCacheRow
		demandScore           sql.NullFloat64
		demandDataQuality     sql.NullString
		demandJSON            sql.NullString
		velocityJSON          sql.NullString
		trendJSON             sql.NullString
		saturationJSON        sql.NullString
		priceDistributionJSON sql.NullString
		analyticsComputedAt   sql.NullTime
		demandComputedAt      sql.NullTime
	)

	if err := s.Scan(
		&row.CardID, &row.Window,
		&demandScore, &demandDataQuality,
		&demandJSON, &velocityJSON, &trendJSON, &saturationJSON, &priceDistributionJSON,
		&analyticsComputedAt, &demandComputedAt, &row.FetchedAt,
	); err != nil {
		return nil, err
	}

	row.DemandScore = nullFloat64ToPtr(demandScore)
	row.DemandDataQuality = nullStringToPtr(demandDataQuality)
	row.DemandJSON = nullStringToPtr(demandJSON)
	row.VelocityJSON = nullStringToPtr(velocityJSON)
	row.TrendJSON = nullStringToPtr(trendJSON)
	row.SaturationJSON = nullStringToPtr(saturationJSON)
	row.PriceDistributionJSON = nullStringToPtr(priceDistributionJSON)
	row.AnalyticsComputedAt = nullTimeToPtr(analyticsComputedAt)
	row.DemandComputedAt = nullTimeToPtr(demandComputedAt)
	return &row, nil
}

func scanCharacterCacheRow(s scanner) (*DHCharacterCacheRow, error) {
	var (
		row                 DHCharacterCacheRow
		demandJSON          sql.NullString
		velocityJSON        sql.NullString
		saturationJSON      sql.NullString
		demandComputedAt    sql.NullTime
		analyticsComputedAt sql.NullTime
	)

	if err := s.Scan(
		&row.Character, &row.Window,
		&demandJSON, &velocityJSON, &saturationJSON,
		&demandComputedAt, &analyticsComputedAt, &row.FetchedAt,
	); err != nil {
		return nil, err
	}

	row.DemandJSON = nullStringToPtr(demandJSON)
	row.VelocityJSON = nullStringToPtr(velocityJSON)
	row.SaturationJSON = nullStringToPtr(saturationJSON)
	row.DemandComputedAt = nullTimeToPtr(demandComputedAt)
	row.AnalyticsComputedAt = nullTimeToPtr(analyticsComputedAt)
	return &row, nil
}

// --- pointer <-> sql.Null* helpers ---

func nullStringFromPtr(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func nullStringToPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	s := n.String
	return &s
}

func nullFloat64FromPtr(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

func nullFloat64ToPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func nullTimeFromPtr(p *time.Time) sql.NullTime {
	if p == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *p, Valid: true}
}

func nullTimeToPtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}
