package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/picks"
)

var _ picks.Repository = (*PicksRepository)(nil)

// PicksRepository implements picks.Repository using SQLite.
type PicksRepository struct {
	db *sql.DB
}

// NewPicksRepository creates a new picks repository.
func NewPicksRepository(db *sql.DB) *PicksRepository {
	return &PicksRepository{db: db}
}

// SavePicks inserts a batch of picks inside a single transaction.
func (r *PicksRepository) SavePicks(ctx context.Context, ps []picks.Pick) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ai_picks
			(pick_date, card_name, set_name, grade, direction, confidence, buy_thesis,
			 target_buy_price, expected_sell_price, signals_json, rank, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close() //nolint:errcheck

	for _, p := range ps {
		sigJSON, err := json.Marshal(p.Signals)
		if err != nil {
			return fmt.Errorf("marshal signals for %q: %w", p.CardName, err)
		}
		_, err = stmt.ExecContext(ctx,
			p.Date.Format("2006-01-02"),
			p.CardName,
			p.SetName,
			p.Grade,
			string(p.Direction),
			string(p.Confidence),
			p.BuyThesis,
			p.TargetBuyPrice,
			p.ExpectedSellPrice,
			string(sigJSON),
			p.Rank,
			string(p.Source),
		)
		if err != nil {
			return fmt.Errorf("insert pick %q: %w", p.CardName, err)
		}
	}
	return tx.Commit()
}

// GetPicksByDate returns all picks for a specific date, ordered by rank.
func (r *PicksRepository) GetPicksByDate(ctx context.Context, date time.Time) ([]picks.Pick, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pick_date, card_name, set_name, grade, direction, confidence,
		       buy_thesis, target_buy_price, expected_sell_price, signals_json, rank, source, created_at
		FROM ai_picks
		WHERE pick_date = ?
		ORDER BY rank`,
		date.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	return scanPicks(ctx, rows)
}

// GetPicksRange returns picks in the given date range (inclusive), ordered by pick_date DESC then rank.
func (r *PicksRepository) GetPicksRange(ctx context.Context, from, to time.Time) ([]picks.Pick, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pick_date, card_name, set_name, grade, direction, confidence,
		       buy_thesis, target_buy_price, expected_sell_price, signals_json, rank, source, created_at
		FROM ai_picks
		WHERE pick_date BETWEEN ? AND ?
		ORDER BY pick_date DESC, rank`,
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	return scanPicks(ctx, rows)
}

// PicksExistForDate reports whether any picks have been stored for date.
func (r *PicksRepository) PicksExistForDate(ctx context.Context, date time.Time) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM ai_picks WHERE pick_date = ?)`,
		date.Format("2006-01-02"),
	).Scan(&exists)
	return exists, err
}

// SaveWatchlistItem inserts a new active watchlist entry.
// Returns picks.ErrWatchlistDuplicate if the card/set/grade is already active.
func (r *PicksRepository) SaveWatchlistItem(ctx context.Context, item picks.WatchlistItem) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO acquisition_watchlist (card_name, set_name, grade, source, active, added_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)`,
		item.CardName,
		item.SetName,
		item.Grade,
		string(item.Source),
		time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return picks.ErrWatchlistDuplicate
		}
		return err
	}
	return nil
}

// DeleteWatchlistItem soft-deletes (sets active=0) a watchlist entry.
// Returns picks.ErrWatchlistItemNotFound if no active row exists with that id.
func (r *PicksRepository) DeleteWatchlistItem(ctx context.Context, id int) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE acquisition_watchlist SET active = 0, updated_at = ? WHERE id = ? AND active = 1`,
		time.Now().UTC().Format(time.RFC3339),
		id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return picks.ErrWatchlistItemNotFound
	}
	return nil
}

// GetActiveWatchlist returns all active watchlist items, each optionally populated
// with its LatestAssessment if a latest_pick_id is set.
func (r *PicksRepository) GetActiveWatchlist(ctx context.Context) ([]picks.WatchlistItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			w.id, w.card_name, w.set_name, w.grade, w.source, w.active, w.added_at, w.updated_at,
			p.id, p.pick_date, p.card_name, p.set_name, p.grade, p.direction, p.confidence,
			p.buy_thesis, p.target_buy_price, p.expected_sell_price, p.signals_json, p.rank, p.source, p.created_at
		FROM acquisition_watchlist w
		LEFT JOIN ai_picks p ON p.id = w.latest_pick_id
		WHERE w.active = 1
		ORDER BY w.added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var items []picks.WatchlistItem
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var (
			item      picks.WatchlistItem
			addedAt   string
			updatedAt string
			active    int
			source    string

			// nullable pick columns (LEFT JOIN)
			pickID              sql.NullInt64
			pickDate            sql.NullString
			pickCardName        sql.NullString
			pickSetName         sql.NullString
			pickGrade           sql.NullString
			pickDirection       sql.NullString
			pickConfidence      sql.NullString
			pickBuyThesis       sql.NullString
			pickTargetBuy       sql.NullInt64
			pickExpectedSell    sql.NullInt64
			pickSignalsJSON     sql.NullString
			pickRank            sql.NullInt64
			pickSource          sql.NullString
			pickCreatedAt       sql.NullString
		)

		if err := rows.Scan(
			&item.ID, &item.CardName, &item.SetName, &item.Grade, &source, &active, &addedAt, &updatedAt,
			&pickID, &pickDate, &pickCardName, &pickSetName, &pickGrade,
			&pickDirection, &pickConfidence, &pickBuyThesis,
			&pickTargetBuy, &pickExpectedSell, &pickSignalsJSON, &pickRank, &pickSource, &pickCreatedAt,
		); err != nil {
			return nil, err
		}

		item.Source = picks.WatchlistSource(source)
		item.Active = active != 0
		item.AddedAt = parseSQLiteTime(addedAt)
		item.UpdatedAt = parseSQLiteTime(updatedAt)

		if pickID.Valid {
			p := &picks.Pick{
				ID:                int(pickID.Int64),
				CardName:          pickCardName.String,
				SetName:           pickSetName.String,
				Grade:             pickGrade.String,
				Direction:         picks.Direction(pickDirection.String),
				Confidence:        picks.Confidence(pickConfidence.String),
				BuyThesis:         pickBuyThesis.String,
				TargetBuyPrice:    int(pickTargetBuy.Int64),
				ExpectedSellPrice: int(pickExpectedSell.Int64),
				Rank:              int(pickRank.Int64),
				Source:            picks.PickSource(pickSource.String),
			}
			if pickDate.Valid {
				if t, err := time.Parse("2006-01-02", pickDate.String); err == nil {
					p.Date = t
				}
			}
			if pickCreatedAt.Valid {
				p.CreatedAt = parseSQLiteTime(pickCreatedAt.String)
			}
			if pickSignalsJSON.Valid && pickSignalsJSON.String != "" {
				var sigs []picks.Signal
				if err := json.Unmarshal([]byte(pickSignalsJSON.String), &sigs); err != nil {
					return nil, fmt.Errorf("unmarshal signals for pick %d: %w", pickID.Int64, err)
				}
				p.Signals = sigs
			}
			item.LatestAssessment = p
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// UpdateWatchlistAssessment links a watchlist entry to a new pick assessment.
func (r *PicksRepository) UpdateWatchlistAssessment(ctx context.Context, watchlistID int, pickID int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE acquisition_watchlist SET latest_pick_id = ?, updated_at = ? WHERE id = ?`,
		pickID,
		time.Now().UTC().Format(time.RFC3339),
		watchlistID,
	)
	return err
}

// scanPicks is a shared helper that reads rows from ai_picks queries.
func scanPicks(ctx context.Context, rows *sql.Rows) ([]picks.Pick, error) {
	return scanRows(ctx, rows, func(rows *sql.Rows) (picks.Pick, error) {
		var (
			p           picks.Pick
			dateStr     string
			createdAt   string
			direction   string
			confidence  string
			source      string
			signalsJSON string
		)
		if err := rows.Scan(
			&p.ID, &dateStr, &p.CardName, &p.SetName, &p.Grade,
			&direction, &confidence, &p.BuyThesis,
			&p.TargetBuyPrice, &p.ExpectedSellPrice,
			&signalsJSON, &p.Rank, &source, &createdAt,
		); err != nil {
			return p, err
		}

		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			p.Date = t
		}
		p.CreatedAt = parseSQLiteTime(createdAt)
		p.Direction = picks.Direction(direction)
		p.Confidence = picks.Confidence(confidence)
		p.Source = picks.PickSource(source)

		if signalsJSON != "" {
			if err := json.Unmarshal([]byte(signalsJSON), &p.Signals); err != nil {
				return p, fmt.Errorf("unmarshal signals: %w", err)
			}
		}
		return p, nil
	})
}
