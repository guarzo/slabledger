package favorites

import (
	"context"
	"errors"
)

// Service defines the business logic for favorites
type Service interface {
	// AddFavorite adds a card to user's favorites with validation
	AddFavorite(ctx context.Context, userID int64, input FavoriteInput) (*Favorite, error)

	// RemoveFavorite removes a card from user's favorites
	RemoveFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) error

	// GetFavorites returns paginated list of user's favorites
	GetFavorites(ctx context.Context, userID int64, page, pageSize int) (*FavoritesList, error)

	// IsFavorite checks if a specific card is favorited
	IsFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error)

	// CheckFavorites checks favorite status for multiple cards
	CheckFavorites(ctx context.Context, userID int64, cards []FavoriteInput) ([]FavoriteCheck, error)

	// ToggleFavorite adds if not exists, removes if exists. Returns new state.
	ToggleFavorite(ctx context.Context, userID int64, input FavoriteInput) (bool, error)
}

// service implements the Service interface
type service struct {
	repo Repository
}

// NewService creates a new favorites service
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// Compile-time interface check
var _ Service = (*service)(nil)

func (s *service) AddFavorite(ctx context.Context, userID int64, input FavoriteInput) (*Favorite, error) {
	if err := ValidateAndNormalizeInput(&input); err != nil {
		return nil, err
	}
	return s.repo.Add(ctx, userID, input)
}

func (s *service) RemoveFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) error {
	return s.repo.Remove(ctx, userID, cardName, setName, cardNumber)
}

func (s *service) GetFavorites(ctx context.Context, userID int64, page, pageSize int) (*FavoritesList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	favs, err := s.repo.List(ctx, userID, pageSize, offset)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, userID)
	if err != nil {
		return nil, err
	}

	totalPages := max((total+pageSize-1)/pageSize, 1)

	return &FavoritesList{
		Favorites:  favs,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *service) IsFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error) {
	return s.repo.IsFavorite(ctx, userID, cardName, setName, cardNumber)
}

func (s *service) CheckFavorites(ctx context.Context, userID int64, cards []FavoriteInput) ([]FavoriteCheck, error) {
	return s.repo.CheckMultiple(ctx, userID, cards)
}

func (s *service) ToggleFavorite(ctx context.Context, userID int64, input FavoriteInput) (bool, error) {
	// Validate input first
	if err := ValidateAndNormalizeInput(&input); err != nil {
		return false, err
	}

	// Try to add first - this is atomic due to UNIQUE constraint
	_, err := s.repo.Add(ctx, userID, input)
	if err == nil {
		// Successfully added - it's now a favorite
		return true, nil
	}

	// If it already exists, remove it
	if errors.Is(err, ErrFavoriteAlreadyExists) {
		removeErr := s.repo.Remove(ctx, userID, input.CardName, input.SetName, input.CardNumber)
		if removeErr != nil {
			return false, removeErr
		}
		return false, nil
	}

	// Some other error occurred
	return false, err
}
