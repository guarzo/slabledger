package favorites

import (
	"time"
)

// Favorite represents a user's saved card
type Favorite struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	CardName   string    `json:"card_name"`
	SetName    string    `json:"set_name"`
	CardNumber string    `json:"card_number"`
	ImageURL   string    `json:"image_url,omitempty"`
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// FavoriteInput represents the data needed to create a favorite
type FavoriteInput struct {
	CardName   string `json:"card_name"`
	SetName    string `json:"set_name"`
	CardNumber string `json:"card_number"`
	ImageURL   string `json:"image_url,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// FavoriteCheck represents the result of checking favorite status
type FavoriteCheck struct {
	CardName   string `json:"card_name"`
	SetName    string `json:"set_name"`
	CardNumber string `json:"card_number"`
	IsFavorite bool   `json:"is_favorite"`
}

// FavoritesList represents a paginated list of favorites
type FavoritesList struct {
	Favorites  []Favorite `json:"favorites"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
}
