package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
)

// Compile-time interface check.
var _ intelligence.SuggestionsRepository = (*DHSuggestionsRepository)(nil)

// DHSuggestionsRepository provides access to the dh_suggestions table.
type DHSuggestionsRepository struct {
	db *sql.DB
}

// NewDHSuggestionsRepository creates a new repository backed by the given database.
func NewDHSuggestionsRepository(db *sql.DB) *DHSuggestionsRepository {
	return &DHSuggestionsRepository{db: db}
}

// StoreSuggestions replaces all suggestions for the given date in a single transaction.
// It deletes existing rows for the date, then inserts the new set.
func (r *DHSuggestionsRepository) StoreSuggestions(ctx context.Context, suggestions []intelligence.Suggestion) error {
	if len(suggestions) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	date := suggestions[0].SuggestionDate

	if _, err := tx.ExecContext(ctx, `DELETE FROM dh_suggestions WHERE suggestion_date = ?`, date); err != nil {
		return fmt.Errorf("delete old suggestions: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO dh_suggestions (
			suggestion_date, type, category, rank, is_manual,
			dh_card_id, card_name, set_name, card_number,
			image_url, current_price_cents, confidence_score,
			reasoning, structured_reasoning, metrics,
			sentiment_score, sentiment_trend, sentiment_mentions,
			fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert suggestions statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range suggestions {
		s := &suggestions[i]
		if _, err := stmt.ExecContext(ctx,
			s.SuggestionDate, s.Type, s.Category, s.Rank, s.IsManual,
			s.DHCardID, s.CardName, s.SetName, s.CardNumber,
			toNullString(s.ImageURL), s.CurrentPriceCents, s.ConfidenceScore,
			toNullString(s.Reasoning), toNullString(s.StructuredReasoning), toNullString(s.Metrics),
			s.SentimentScore, s.SentimentTrend, s.SentimentMentions,
			s.FetchedAt,
		); err != nil {
			return fmt.Errorf("insert suggestion: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction saving suggestions: %w", err)
	}
	return nil
}

// GetByDate returns all suggestions for a specific date.
func (r *DHSuggestionsRepository) GetByDate(ctx context.Context, date string) (_ []intelligence.Suggestion, err error) {
	return r.queryMany(ctx,
		`SELECT suggestion_date, type, category, rank, is_manual,
			dh_card_id, card_name, set_name, card_number,
			image_url, current_price_cents, confidence_score,
			reasoning, structured_reasoning, metrics,
			sentiment_score, sentiment_trend, sentiment_mentions,
			fetched_at
		FROM dh_suggestions
		WHERE suggestion_date = ?
		ORDER BY type, category, rank`,
		date,
	)
}

// GetLatest returns suggestions from the most recent date.
func (r *DHSuggestionsRepository) GetLatest(ctx context.Context) (_ []intelligence.Suggestion, err error) {
	return r.queryMany(ctx,
		`SELECT suggestion_date, type, category, rank, is_manual,
			dh_card_id, card_name, set_name, card_number,
			image_url, current_price_cents, confidence_score,
			reasoning, structured_reasoning, metrics,
			sentiment_score, sentiment_trend, sentiment_mentions,
			fetched_at
		FROM dh_suggestions
		WHERE suggestion_date = (SELECT MAX(suggestion_date) FROM dh_suggestions)
		ORDER BY type, category, rank`,
	)
}

// GetCardSuggestions returns all suggestions matching the given card name and set.
func (r *DHSuggestionsRepository) GetCardSuggestions(ctx context.Context, cardName, setName string) (_ []intelligence.Suggestion, err error) {
	return r.queryMany(ctx,
		`SELECT suggestion_date, type, category, rank, is_manual,
			dh_card_id, card_name, set_name, card_number,
			image_url, current_price_cents, confidence_score,
			reasoning, structured_reasoning, metrics,
			sentiment_score, sentiment_trend, sentiment_mentions,
			fetched_at
		FROM dh_suggestions
		WHERE card_name = ? AND set_name = ?
		ORDER BY suggestion_date DESC, type, category, rank`,
		cardName, setName,
	)
}

// CountLatest returns the number of suggestions for the most recent suggestion_date.
func (r *DHSuggestionsRepository) CountLatest(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM dh_suggestions
		 WHERE suggestion_date = (SELECT MAX(suggestion_date) FROM dh_suggestions)`).Scan(&count)
	return count, err
}

// LatestFetchedAt returns the most recent fetched_at timestamp across all suggestions, or empty string if none exist.
func (r *DHSuggestionsRepository) LatestFetchedAt(ctx context.Context) (string, error) {
	var ts *string
	err := r.db.QueryRowContext(ctx, `SELECT MAX(fetched_at) FROM dh_suggestions`).Scan(&ts)
	if err != nil || ts == nil {
		return "", err
	}
	return *ts, nil
}

// queryMany executes a query and scans all rows into Suggestion slices.
func (r *DHSuggestionsRepository) queryMany(ctx context.Context, query string, args ...any) (_ []intelligence.Suggestion, err error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	results := make([]intelligence.Suggestion, 0, 32)
	for rows.Next() {
		s, err := scanSuggestionRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *s)
	}
	return results, rows.Err()
}

// scanSuggestionRow scans a single row into a Suggestion.
func scanSuggestionRow(row scanner) (*intelligence.Suggestion, error) {
	var (
		s                   intelligence.Suggestion
		imageURL            sql.NullString
		reasoning           sql.NullString
		structuredReasoning sql.NullString
		metrics             sql.NullString
	)

	if err := row.Scan(
		&s.SuggestionDate, &s.Type, &s.Category, &s.Rank, &s.IsManual,
		&s.DHCardID, &s.CardName, &s.SetName, &s.CardNumber,
		&imageURL, &s.CurrentPriceCents, &s.ConfidenceScore,
		&reasoning, &structuredReasoning, &metrics,
		&s.SentimentScore, &s.SentimentTrend, &s.SentimentMentions,
		&s.FetchedAt,
	); err != nil {
		return nil, err
	}

	s.ImageURL = imageURL.String
	s.Reasoning = reasoning.String
	s.StructuredReasoning = structuredReasoning.String
	s.Metrics = metrics.String

	return &s, nil
}
