package favorites

import "context"

// Repository defines the interface for favorites persistence
type Repository interface {
	// Add adds a card to user's favorites
	Add(ctx context.Context, userID int64, input FavoriteInput) (*Favorite, error)

	// Remove removes a card from user's favorites
	Remove(ctx context.Context, userID int64, cardName, setName, cardNumber string) error

	// List returns all favorites for a user, ordered by created_at DESC
	List(ctx context.Context, userID int64, limit, offset int) ([]Favorite, error)

	// Count returns the total count of favorites for a user
	Count(ctx context.Context, userID int64) (int, error)

	// IsFavorite checks if a specific card is favorited by user
	IsFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error)

	// CheckMultiple checks favorite status for multiple cards at once
	CheckMultiple(ctx context.Context, userID int64, cards []FavoriteInput) ([]FavoriteCheck, error)
}
