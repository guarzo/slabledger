package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
)

// Compile-time interface check.
var _ intelligence.Repository = (*MarketIntelligenceRepository)(nil)

// MarketIntelligenceRepository provides access to the market_intelligence table.
type MarketIntelligenceRepository struct {
	db *sql.DB
}

// NewMarketIntelligenceRepository creates a new repository backed by the given database.
func NewMarketIntelligenceRepository(db *sql.DB) *MarketIntelligenceRepository {
	return &MarketIntelligenceRepository{db: db}
}

// Store upserts a MarketIntelligence record.
func (r *MarketIntelligenceRepository) Store(ctx context.Context, intel *intelligence.MarketIntelligence) error {
	sentimentScore, sentimentMentions, sentimentTrend := encodeNullSentiment(intel.Sentiment)
	forecastPrice, forecastConf, forecastDate := encodeNullForecast(intel.Forecast)
	insightsHeadline, insightsDetail := encodeNullInsights(intel.Insights)
	st30, st60, st90, sampleSize, velLastFetch := encodeNullVelocity(intel.Velocity)
	vol7, vol30, vol90 := encodeNullTrend(intel.Trend)

	gradingROI, err := marshalJSON(intel.GradingROI)
	if err != nil {
		return fmt.Errorf("marshal grading roi: %w", err)
	}
	recentSales, err := marshalJSON(intel.RecentSales)
	if err != nil {
		return fmt.Errorf("marshal recent sales: %w", err)
	}
	population, err := marshalJSON(intel.Population)
	if err != nil {
		return fmt.Errorf("marshal population: %w", err)
	}

	now := time.Now()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO market_intelligence (
			card_name, set_name, card_number, dh_card_id,
			sentiment_score, sentiment_mentions, sentiment_trend,
			forecast_price_cents, forecast_confidence, forecast_date,
			grading_roi, recent_sales, population,
			insights_headline, insights_detail,
			volume_7d, volume_30d, volume_90d,
			sell_through_30d_pct, sell_through_60d_pct, sell_through_90d_pct,
			velocity_sample_size, velocity_last_fetch,
			fetched_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
		ON CONFLICT(card_name, set_name, card_number) DO UPDATE SET
			dh_card_id = excluded.dh_card_id,
			sentiment_score = excluded.sentiment_score,
			sentiment_mentions = excluded.sentiment_mentions,
			sentiment_trend = excluded.sentiment_trend,
			forecast_price_cents = excluded.forecast_price_cents,
			forecast_confidence = excluded.forecast_confidence,
			forecast_date = excluded.forecast_date,
			grading_roi = excluded.grading_roi,
			recent_sales = excluded.recent_sales,
			population = excluded.population,
			insights_headline = excluded.insights_headline,
			insights_detail = excluded.insights_detail,
			volume_7d = excluded.volume_7d,
			volume_30d = excluded.volume_30d,
			volume_90d = excluded.volume_90d,
			sell_through_30d_pct = excluded.sell_through_30d_pct,
			sell_through_60d_pct = excluded.sell_through_60d_pct,
			sell_through_90d_pct = excluded.sell_through_90d_pct,
			velocity_sample_size = excluded.velocity_sample_size,
			velocity_last_fetch = excluded.velocity_last_fetch,
			fetched_at = excluded.fetched_at,
			updated_at = excluded.updated_at`,
		intel.CardName, intel.SetName, intel.CardNumber, intel.DHCardID,
		sentimentScore, sentimentMentions, sentimentTrend,
		forecastPrice, forecastConf, forecastDate,
		gradingROI, recentSales, population,
		insightsHeadline, insightsDetail,
		vol7, vol30, vol90,
		st30, st60, st90,
		sampleSize, velLastFetch,
		intel.FetchedAt, now, now,
	)
	if err != nil {
		return fmt.Errorf("store market intelligence: %w", err)
	}
	return nil
}

// GetByCard returns the market intelligence for the given card, or nil if not found.
func (r *MarketIntelligenceRepository) GetByCard(ctx context.Context, cardName, setName, cardNumber string) (*intelligence.MarketIntelligence, error) {
	return r.scanOne(ctx,
		`SELECT card_name, set_name, card_number, dh_card_id,
			sentiment_score, sentiment_mentions, sentiment_trend,
			forecast_price_cents, forecast_confidence, forecast_date,
			grading_roi, recent_sales, population,
			insights_headline, insights_detail,
			volume_7d, volume_30d, volume_90d,
			sell_through_30d_pct, sell_through_60d_pct, sell_through_90d_pct,
			velocity_sample_size, velocity_last_fetch,
			fetched_at
		FROM market_intelligence
		WHERE card_name = $1 AND set_name = $2 AND card_number = $3`,
		cardName, setName, cardNumber,
	)
}

// GetByDHCardID returns the market intelligence for the given DH card ID, or nil if not found.
func (r *MarketIntelligenceRepository) GetByDHCardID(ctx context.Context, dhCardID string) (*intelligence.MarketIntelligence, error) {
	return r.scanOne(ctx,
		`SELECT card_name, set_name, card_number, dh_card_id,
			sentiment_score, sentiment_mentions, sentiment_trend,
			forecast_price_cents, forecast_confidence, forecast_date,
			grading_roi, recent_sales, population,
			insights_headline, insights_detail,
			volume_7d, volume_30d, volume_90d,
			sell_through_30d_pct, sell_through_60d_pct, sell_through_90d_pct,
			velocity_sample_size, velocity_last_fetch,
			fetched_at
		FROM market_intelligence
		WHERE dh_card_id = $1`,
		dhCardID,
	)
}

// GetStale returns market intelligence entries with fetched_at older than maxAge, ordered oldest first.
func (r *MarketIntelligenceRepository) GetStale(ctx context.Context, maxAge time.Duration, limit int) (_ []intelligence.MarketIntelligence, err error) {
	cutoff := time.Now().Add(-maxAge)
	rows, err := r.db.QueryContext(ctx,
		`SELECT card_name, set_name, card_number, dh_card_id,
			sentiment_score, sentiment_mentions, sentiment_trend,
			forecast_price_cents, forecast_confidence, forecast_date,
			grading_roi, recent_sales, population,
			insights_headline, insights_detail,
			volume_7d, volume_30d, volume_90d,
			sell_through_30d_pct, sell_through_60d_pct, sell_through_90d_pct,
			velocity_sample_size, velocity_last_fetch,
			fetched_at
		FROM market_intelligence
		WHERE fetched_at < $1
		ORDER BY fetched_at ASC
		LIMIT $2`,
		cutoff, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	results := make([]intelligence.MarketIntelligence, 0, limit)
	for rows.Next() {
		intel, err := scanIntelRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *intel)
	}
	return results, rows.Err()
}

// CountAll returns the total number of market intelligence records.
func (r *MarketIntelligenceRepository) CountAll(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_intelligence`).Scan(&count)
	return count, err
}

// LatestFetchedAt returns the most recent fetched_at timestamp, or empty string if no records exist.
func (r *MarketIntelligenceRepository) LatestFetchedAt(ctx context.Context) (string, error) {
	var ts sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT MAX(fetched_at) FROM market_intelligence`).Scan(&ts)
	if err != nil {
		return "", err
	}
	if !ts.Valid {
		return "", nil
	}
	return ts.Time.UTC().Format("2006-01-02 15:04:05"), nil
}

// GetByCards returns market intelligence for all matching cards.
// Keys not found in the database are omitted from the result map.
// Large key sets are automatically chunked to stay within Postgres's parameter limit.
func (r *MarketIntelligenceRepository) GetByCards(ctx context.Context, keys []intelligence.CardKey) (_ map[intelligence.CardKey]*intelligence.MarketIntelligence, err error) {
	if len(keys) == 0 {
		return map[intelligence.CardKey]*intelligence.MarketIntelligence{}, nil
	}

	// Postgres's max parameter count is 65535; staying well under that at 333 * 3 = 999.
	const maxKeysPerChunk = 333
	result := make(map[intelligence.CardKey]*intelligence.MarketIntelligence, len(keys))

	for start := 0; start < len(keys); start += maxKeysPerChunk {
		end := start + maxKeysPerChunk
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[start:end]

		if err := r.getByCardsChunk(ctx, chunk, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *MarketIntelligenceRepository) getByCardsChunk(ctx context.Context, keys []intelligence.CardKey, result map[intelligence.CardKey]*intelligence.MarketIntelligence) (err error) {
	args := make([]any, 0, len(keys)*3)
	conditions := make([]string, 0, len(keys))
	for _, k := range keys {
		base := len(args)
		conditions = append(conditions, fmt.Sprintf("(card_name = $%d AND set_name = $%d AND card_number = $%d)", base+1, base+2, base+3))
		args = append(args, k.CardName, k.SetName, k.CardNumber)
	}

	query := `SELECT card_name, set_name, card_number, dh_card_id,
		sentiment_score, sentiment_mentions, sentiment_trend,
		forecast_price_cents, forecast_confidence, forecast_date,
		grading_roi, recent_sales, population,
		insights_headline, insights_detail,
		volume_7d, volume_30d, volume_90d,
		sell_through_30d_pct, sell_through_60d_pct, sell_through_90d_pct,
		velocity_sample_size, velocity_last_fetch,
		fetched_at
	FROM market_intelligence
	WHERE ` + strings.Join(conditions, " OR ")

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query market intelligence: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	for rows.Next() {
		intel, err := scanIntelRow(rows)
		if err != nil {
			return fmt.Errorf("scan market intelligence row: %w", err)
		}
		key := intelligence.CardKey{
			CardName:   intel.CardName,
			SetName:    intel.SetName,
			CardNumber: intel.CardNumber,
		}
		result[key] = intel
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate market intelligence rows in getByCardsChunk: %w", err)
	}
	return nil
}

// scanOne executes a query expected to return zero or one row and scans it into a MarketIntelligence.
func (r *MarketIntelligenceRepository) scanOne(ctx context.Context, query string, args ...any) (_ *intelligence.MarketIntelligence, err error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return scanIntelRow(rows)
}

// scanIntelRow scans a single row into a MarketIntelligence.
func scanIntelRow(row scanner) (*intelligence.MarketIntelligence, error) {
	var (
		intel              intelligence.MarketIntelligence
		sentimentScore     sql.NullFloat64
		sentimentMentions  sql.NullInt64
		sentimentTrend     sql.NullString
		forecastPrice      sql.NullInt64
		forecastConf       sql.NullFloat64
		forecastDate       sql.NullString
		gradingROI         sql.NullString
		recentSales        sql.NullString
		population         sql.NullString
		insightsHeadline   sql.NullString
		insightsDetail     sql.NullString
		volume7d           sql.NullInt64
		volume30d          sql.NullInt64
		volume90d          sql.NullInt64
		sellThrough30d     sql.NullFloat64
		sellThrough60d     sql.NullFloat64
		sellThrough90d     sql.NullFloat64
		velocitySampleSize sql.NullInt64
		velocityLastFetch  sql.NullTime
	)

	if err := row.Scan(
		&intel.CardName, &intel.SetName, &intel.CardNumber, &intel.DHCardID,
		&sentimentScore, &sentimentMentions, &sentimentTrend,
		&forecastPrice, &forecastConf, &forecastDate,
		&gradingROI, &recentSales, &population,
		&insightsHeadline, &insightsDetail,
		&volume7d, &volume30d, &volume90d,
		&sellThrough30d, &sellThrough60d, &sellThrough90d,
		&velocitySampleSize, &velocityLastFetch,
		&intel.FetchedAt,
	); err != nil {
		return nil, err
	}

	intel.Sentiment = decodeNullSentiment(sentimentScore, sentimentMentions, sentimentTrend)
	intel.Forecast = decodeNullForecast(forecastPrice, forecastConf, forecastDate)
	intel.Insights = decodeNullInsights(insightsHeadline, insightsDetail)
	intel.Trend = decodeNullTrend(volume7d, volume30d, volume90d)
	intel.Velocity = decodeNullVelocity(sellThrough30d, sellThrough60d, sellThrough90d, velocitySampleSize, velocityLastFetch)

	if gradingROI.Valid && gradingROI.String != "" {
		if err := json.Unmarshal([]byte(gradingROI.String), &intel.GradingROI); err != nil {
			return nil, err
		}
	}
	if recentSales.Valid && recentSales.String != "" {
		if err := json.Unmarshal([]byte(recentSales.String), &intel.RecentSales); err != nil {
			return nil, err
		}
	}
	if population.Valid && population.String != "" {
		if err := json.Unmarshal([]byte(population.String), &intel.Population); err != nil {
			return nil, err
		}
	}

	return &intel, nil
}

// --- nullable encoding helpers ---

func encodeNullSentiment(s *intelligence.Sentiment) (sql.NullFloat64, sql.NullInt64, sql.NullString) {
	if s == nil {
		return sql.NullFloat64{}, sql.NullInt64{}, sql.NullString{}
	}
	return sql.NullFloat64{Float64: s.Score, Valid: true},
		sql.NullInt64{Int64: int64(s.MentionCount), Valid: true},
		sql.NullString{String: s.Trend, Valid: true}
}

func decodeNullSentiment(score sql.NullFloat64, mentions sql.NullInt64, trend sql.NullString) *intelligence.Sentiment {
	if !score.Valid {
		return nil
	}
	return &intelligence.Sentiment{
		Score:        score.Float64,
		MentionCount: int(mentions.Int64),
		Trend:        trend.String,
	}
}

func encodeNullForecast(f *intelligence.Forecast) (sql.NullInt64, sql.NullFloat64, sql.NullString) {
	if f == nil {
		return sql.NullInt64{}, sql.NullFloat64{}, sql.NullString{}
	}
	return sql.NullInt64{Int64: f.PredictedPriceCents, Valid: true},
		sql.NullFloat64{Float64: f.Confidence, Valid: true},
		sql.NullString{String: f.ForecastDate.Format(time.RFC3339), Valid: true}
}

func decodeNullForecast(price sql.NullInt64, conf sql.NullFloat64, dateStr sql.NullString) *intelligence.Forecast {
	if !price.Valid {
		return nil
	}
	f := &intelligence.Forecast{
		PredictedPriceCents: price.Int64,
		Confidence:          conf.Float64,
	}
	if dateStr.Valid {
		t, err := time.Parse(time.RFC3339, dateStr.String)
		if err == nil {
			f.ForecastDate = t
		}
	}
	return f
}

func encodeNullInsights(i *intelligence.Insights) (sql.NullString, sql.NullString) {
	if i == nil {
		return sql.NullString{}, sql.NullString{}
	}
	return sql.NullString{String: i.Headline, Valid: true},
		sql.NullString{String: i.Detail, Valid: true}
}

func decodeNullInsights(headline, detail sql.NullString) *intelligence.Insights {
	if !headline.Valid {
		return nil
	}
	return &intelligence.Insights{
		Headline: headline.String,
		Detail:   detail.String,
	}
}

func encodeNullTrend(t *intelligence.Trend) (sql.NullInt64, sql.NullInt64, sql.NullInt64) {
	if t == nil {
		return sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(t.Volume7d), Valid: true},
		sql.NullInt64{Int64: int64(t.Volume30d), Valid: true},
		sql.NullInt64{Int64: int64(t.Volume90d), Valid: true}
}

func decodeNullTrend(v7, v30, v90 sql.NullInt64) *intelligence.Trend {
	if !v7.Valid && !v30.Valid && !v90.Valid {
		return nil
	}
	return &intelligence.Trend{
		Volume7d:  int(v7.Int64),
		Volume30d: int(v30.Int64),
		Volume90d: int(v90.Int64),
	}
}

func encodeNullVelocity(v *intelligence.Velocity) (sql.NullFloat64, sql.NullFloat64, sql.NullFloat64, sql.NullInt64, sql.NullTime) {
	if v == nil {
		return sql.NullFloat64{}, sql.NullFloat64{}, sql.NullFloat64{}, sql.NullInt64{}, sql.NullTime{}
	}
	lf := sql.NullTime{Time: v.LastFetch, Valid: !v.LastFetch.IsZero()}
	return sql.NullFloat64{Float64: v.SellThrough30dPct, Valid: true},
		sql.NullFloat64{Float64: v.SellThrough60dPct, Valid: true},
		sql.NullFloat64{Float64: v.SellThrough90dPct, Valid: true},
		sql.NullInt64{Int64: int64(v.SampleSize), Valid: true},
		lf
}

func decodeNullVelocity(st30, st60, st90 sql.NullFloat64, sampleSize sql.NullInt64, lastFetch sql.NullTime) *intelligence.Velocity {
	if !st30.Valid && !st60.Valid && !st90.Valid && !sampleSize.Valid {
		return nil
	}
	v := &intelligence.Velocity{
		SellThrough30dPct: st30.Float64,
		SellThrough60dPct: st60.Float64,
		SellThrough90dPct: st90.Float64,
		SampleSize:        int(sampleSize.Int64),
	}
	if lastFetch.Valid {
		v.LastFetch = lastFetch.Time
	}
	return v
}

// marshalJSON marshals v to a JSON string, returning sql.NullString.
// nil or empty slices produce a NULL column.
func marshalJSON(v any) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}, err
	}
	s := string(data)
	if s == "null" {
		return sql.NullString{}, nil
	}
	return sql.NullString{String: s, Valid: true}, nil
}
