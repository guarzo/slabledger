package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/favorites"
)

// favoriteMapKey is a composite key used for O(1) lookups in batch favorite
// checks.  Using a struct avoids the delimiter-collision bugs inherent in
// string concatenation (e.g. "a|b" + "|c" vs "a" + "|b|c").
type favoriteMapKey struct {
	cardName   string
	setName    string
	cardNumber string
}

var (
	rowValueOnce       sync.Once
	supportsRowValueIn bool
)

// probeRowValueIn tests whether the SQLite driver supports tuple-IN syntax.
// The result is cached so the probe runs at most once per process.
func probeRowValueIn(db *sql.DB) {
	rowValueOnce.Do(func() {
		// A minimal query that uses row-value IN.  If the driver/version
		// does not support it the query will return a syntax error.
		_, err := db.Exec("SELECT 1 WHERE (1, 2) IN ((1, 2))")
		supportsRowValueIn = err == nil
	})
}

// FavoritesRepository implements favorites.Repository using SQLite
type FavoritesRepository struct {
	db *sql.DB
}

// NewFavoritesRepository creates a new SQLite favorites repository.
// It probes the database once to determine whether row-value IN is supported.
func NewFavoritesRepository(db *sql.DB) *FavoritesRepository {
	probeRowValueIn(db)
	return &FavoritesRepository{db: db}
}

// Ensure FavoritesRepository implements favorites.Repository
var _ favorites.Repository = (*FavoritesRepository)(nil)

// Add adds a card to user's favorites
func (r *FavoritesRepository) Add(ctx context.Context, userID int64, input favorites.FavoriteInput) (*favorites.Favorite, error) {
	query := `
		INSERT INTO favorites (user_id, card_name, set_name, card_number, image_url, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		userID,
		input.CardName,
		input.SetName,
		input.CardNumber,
		toNullString(input.ImageURL),
		toNullString(input.Notes),
		now,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, favorites.ErrFavoriteAlreadyExists
		}
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &favorites.Favorite{
		ID:         id,
		UserID:     userID,
		CardName:   input.CardName,
		SetName:    input.SetName,
		CardNumber: input.CardNumber,
		ImageURL:   input.ImageURL,
		Notes:      input.Notes,
		CreatedAt:  now,
	}, nil
}

// Remove removes a card from user's favorites
func (r *FavoritesRepository) Remove(ctx context.Context, userID int64, cardName, setName, cardNumber string) error {
	query := `
		DELETE FROM favorites
		WHERE user_id = ? AND card_name = ? AND set_name = ? AND card_number = ?
	`

	result, err := r.db.ExecContext(ctx, query, userID, cardName, setName, cardNumber)
	if err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return favorites.ErrFavoriteNotFound
	}
	return nil
}

// List returns all favorites for a user, ordered by created_at DESC
func (r *FavoritesRepository) List(ctx context.Context, userID int64, limit, offset int) (favs []favorites.Favorite, err error) {
	query := `
		SELECT id, user_id, card_name, set_name, card_number, image_url, notes, created_at
		FROM favorites
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	for rows.Next() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var f favorites.Favorite
		var imageURL, notes sql.NullString

		err := rows.Scan(
			&f.ID,
			&f.UserID,
			&f.CardName,
			&f.SetName,
			&f.CardNumber,
			&imageURL,
			&notes,
			&f.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		f.ImageURL = imageURL.String
		f.Notes = notes.String
		favs = append(favs, f)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favs, err
}

// Count returns the total count of favorites for a user
func (r *FavoritesRepository) Count(ctx context.Context, userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM favorites WHERE user_id = ?`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

// IsFavorite checks if a specific card is favorited by user
func (r *FavoritesRepository) IsFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM favorites
			WHERE user_id = ? AND card_name = ? AND set_name = ? AND card_number = ?
		)
	`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, cardName, setName, cardNumber).Scan(&exists)
	return exists, err
}

// DistinctFavoriteCard represents a unique card from the favorites table.
type DistinctFavoriteCard struct {
	CardName   string
	SetName    string
	CardNumber string
}

// ListAllDistinctCards returns all unique (card_name, set_name, card_number) tuples
// across all users. Used for background batch processing.
func (r *FavoritesRepository) ListAllDistinctCards(ctx context.Context) (_ []DistinctFavoriteCard, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT card_name, set_name, card_number FROM favorites`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var cards []DistinctFavoriteCard
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var c DistinctFavoriteCard
		if err := rows.Scan(&c.CardName, &c.SetName, &c.CardNumber); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// CheckMultiple checks favorite status for multiple cards at once.
// It uses a tuple-IN query when the SQLite driver supports it, otherwise
// falls back to individual lookups.  The capability is probed once at
// repository creation time so we never accidentally swallow real SQL errors.
func (r *FavoritesRepository) CheckMultiple(ctx context.Context, userID int64, cards []favorites.FavoriteInput) (results []favorites.FavoriteCheck, err error) {
	if len(cards) == 0 {
		return []favorites.FavoriteCheck{}, nil
	}

	if !supportsRowValueIn {
		return r.checkMultipleFallback(ctx, userID, cards)
	}

	// Build query with placeholders for batch lookup
	// SECURITY NOTE: This is safe because placeholders are programmatically
	// generated "(?, ?, ?)" strings. Actual user values are passed through
	// the parameterized args slice, preventing SQL injection.
	placeholders := make([]string, len(cards))
	args := make([]any, 0, len(cards)*3+1)
	args = append(args, userID)

	for i, card := range cards {
		placeholders[i] = "(?, ?, ?)"
		args = append(args, card.CardName, card.SetName, card.CardNumber)
	}

	query := `
		SELECT card_name, set_name, card_number
		FROM favorites
		WHERE user_id = ? AND (card_name, set_name, card_number) IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	// Build set of favorited cards
	favoritedSet := make(map[favoriteMapKey]bool)
	for rows.Next() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var cardName, setName, cardNumber string
		if err := rows.Scan(&cardName, &setName, &cardNumber); err != nil {
			return nil, err
		}
		favoritedSet[favoriteMapKey{cardName, setName, cardNumber}] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Build result
	results = make([]favorites.FavoriteCheck, len(cards))
	for i, card := range cards {
		results[i] = favorites.FavoriteCheck{
			CardName:   card.CardName,
			SetName:    card.SetName,
			CardNumber: card.CardNumber,
			IsFavorite: favoritedSet[favoriteMapKey{card.CardName, card.SetName, card.CardNumber}],
		}
	}

	return results, err
}

// checkMultipleFallback checks favorite status one by one (for older SQLite versions)
func (r *FavoritesRepository) checkMultipleFallback(ctx context.Context, userID int64, cards []favorites.FavoriteInput) ([]favorites.FavoriteCheck, error) {
	results := make([]favorites.FavoriteCheck, len(cards))

	for i, card := range cards {
		isFav, err := r.IsFavorite(ctx, userID, card.CardName, card.SetName, card.CardNumber)
		if err != nil {
			return nil, err
		}

		results[i] = favorites.FavoriteCheck{
			CardName:   card.CardName,
			SetName:    card.SetName,
			CardNumber: card.CardNumber,
			IsFavorite: isFav,
		}
	}

	return results, nil
}
