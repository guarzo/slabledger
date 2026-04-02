package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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

	gradingROI, err := marshalJSON(intel.GradingROI)
	if err != nil {
		return err
	}
	recentSales, err := marshalJSON(intel.RecentSales)
	if err != nil {
		return err
	}
	population, err := marshalJSON(intel.Population)
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO market_intelligence (
			card_name, set_name, card_number, dh_card_id,
			sentiment_score, sentiment_mentions, sentiment_trend,
			forecast_price_cents, forecast_confidence, forecast_date,
			grading_roi, recent_sales, population,
			insights_headline, insights_detail,
			fetched_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			fetched_at = excluded.fetched_at,
			updated_at = excluded.updated_at`,
		intel.CardName, intel.SetName, intel.CardNumber, intel.DHCardID,
		sentimentScore, sentimentMentions, sentimentTrend,
		forecastPrice, forecastConf, forecastDate,
		gradingROI, recentSales, population,
		insightsHeadline, insightsDetail,
		intel.FetchedAt, now, now,
	)
	return err
}

// GetByCard returns the market intelligence for the given card, or nil if not found.
func (r *MarketIntelligenceRepository) GetByCard(ctx context.Context, cardName, setName, cardNumber string) (*intelligence.MarketIntelligence, error) {
	return r.scanOne(ctx,
		`SELECT card_name, set_name, card_number, dh_card_id,
			sentiment_score, sentiment_mentions, sentiment_trend,
			forecast_price_cents, forecast_confidence, forecast_date,
			grading_roi, recent_sales, population,
			insights_headline, insights_detail, fetched_at
		FROM market_intelligence
		WHERE card_name = ? AND set_name = ? AND card_number = ?`,
		cardName, setName, cardNumber,
	)
}

// GetByDHCardID returns the market intelligence for the given DoubleHolo card ID, or nil if not found.
func (r *MarketIntelligenceRepository) GetByDHCardID(ctx context.Context, dhCardID string) (*intelligence.MarketIntelligence, error) {
	return r.scanOne(ctx,
		`SELECT card_name, set_name, card_number, dh_card_id,
			sentiment_score, sentiment_mentions, sentiment_trend,
			forecast_price_cents, forecast_confidence, forecast_date,
			grading_roi, recent_sales, population,
			insights_headline, insights_detail, fetched_at
		FROM market_intelligence
		WHERE dh_card_id = ?`,
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
			insights_headline, insights_detail, fetched_at
		FROM market_intelligence
		WHERE fetched_at < ?
		ORDER BY fetched_at ASC
		LIMIT ?`,
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

// GetByCards returns market intelligence for all matching cards.
// Keys not found in the database are omitted from the result map.
// Large key sets are automatically chunked to stay within SQLite's parameter limit.
func (r *MarketIntelligenceRepository) GetByCards(ctx context.Context, keys []intelligence.CardKey) (_ map[intelligence.CardKey]*intelligence.MarketIntelligence, err error) {
	if len(keys) == 0 {
		return map[intelligence.CardKey]*intelligence.MarketIntelligence{}, nil
	}

	const maxKeysPerChunk = 333 // floor(999 / 3 placeholders per key)
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
	var args []any
	conditions := make([]string, 0, len(keys))
	for _, k := range keys {
		conditions = append(conditions, "(card_name = ? AND set_name = ? AND card_number = ?)")
		args = append(args, k.CardName, k.SetName, k.CardNumber)
	}

	query := `SELECT card_name, set_name, card_number, dh_card_id,
		sentiment_score, sentiment_mentions, sentiment_trend,
		forecast_price_cents, forecast_confidence, forecast_date,
		grading_roi, recent_sales, population,
		insights_headline, insights_detail, fetched_at
	FROM market_intelligence
	WHERE ` + strings.Join(conditions, " OR ")

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	for rows.Next() {
		intel, err := scanIntelRow(rows)
		if err != nil {
			return err
		}
		key := intelligence.CardKey{
			CardName:   intel.CardName,
			SetName:    intel.SetName,
			CardNumber: intel.CardNumber,
		}
		result[key] = intel
	}
	return rows.Err()
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
		intel             intelligence.MarketIntelligence
		sentimentScore    sql.NullFloat64
		sentimentMentions sql.NullInt64
		sentimentTrend    sql.NullString
		forecastPrice     sql.NullInt64
		forecastConf      sql.NullFloat64
		forecastDate      sql.NullString
		gradingROI        sql.NullString
		recentSales       sql.NullString
		population        sql.NullString
		insightsHeadline  sql.NullString
		insightsDetail    sql.NullString
	)

	if err := row.Scan(
		&intel.CardName, &intel.SetName, &intel.CardNumber, &intel.DHCardID,
		&sentimentScore, &sentimentMentions, &sentimentTrend,
		&forecastPrice, &forecastConf, &forecastDate,
		&gradingROI, &recentSales, &population,
		&insightsHeadline, &insightsDetail, &intel.FetchedAt,
	); err != nil {
		return nil, err
	}

	intel.Sentiment = decodeNullSentiment(sentimentScore, sentimentMentions, sentimentTrend)
	intel.Forecast = decodeNullForecast(forecastPrice, forecastConf, forecastDate)
	intel.Insights = decodeNullInsights(insightsHeadline, insightsDetail)

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
