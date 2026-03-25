package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/guarzo/slabledger/internal/domain/favorites"
)

func setupFavoritesTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			google_id TEXT UNIQUE,
			email TEXT
		);
		CREATE TABLE favorites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			card_name TEXT NOT NULL,
			set_name TEXT NOT NULL,
			card_number TEXT NOT NULL DEFAULT '',
			image_url TEXT,
			notes TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			UNIQUE(user_id, card_name, set_name, card_number)
		);
		CREATE INDEX idx_favorites_user_created ON favorites(user_id, created_at DESC);
		CREATE INDEX idx_favorites_user_card ON favorites(user_id, card_name, set_name, card_number);
		INSERT INTO users (id, google_id, email) VALUES (1, 'test-google-id', 'test@example.com');
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	return db
}

func TestFavoritesRepository_Add(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()
	repo := NewFavoritesRepository(db)

	input := favorites.FavoriteInput{
		CardName:   "Charizard ex",
		SetName:    "Obsidian Flames",
		CardNumber: "125",
		ImageURL:   "https://example.com/image.png",
	}

	// Test successful add
	fav, err := repo.Add(context.Background(), 1, input)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if fav.ID == 0 {
		t.Error("Add() returned favorite with zero ID")
	}
	if fav.CardName != input.CardName {
		t.Errorf("Add() CardName = %v, want %v", fav.CardName, input.CardName)
	}
	if fav.ImageURL != input.ImageURL {
		t.Errorf("Add() ImageURL = %v, want %v", fav.ImageURL, input.ImageURL)
	}

	// Test duplicate
	_, err = repo.Add(context.Background(), 1, input)
	if err != favorites.ErrFavoriteAlreadyExists {
		t.Errorf("Add() duplicate error = %v, want %v", err, favorites.ErrFavoriteAlreadyExists)
	}
}

func TestFavoritesRepository_Remove(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()
	repo := NewFavoritesRepository(db)

	input := favorites.FavoriteInput{
		CardName:   "Pikachu",
		SetName:    "Base Set",
		CardNumber: "58",
	}

	// Add first
	_, _ = repo.Add(context.Background(), 1, input)

	// Test successful remove
	err := repo.Remove(context.Background(), 1, input.CardName, input.SetName, input.CardNumber)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Test remove non-existent
	err = repo.Remove(context.Background(), 1, input.CardName, input.SetName, input.CardNumber)
	if err != favorites.ErrFavoriteNotFound {
		t.Errorf("Remove() non-existent error = %v, want %v", err, favorites.ErrFavoriteNotFound)
	}
}

func TestFavoritesRepository_List(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()
	repo := NewFavoritesRepository(db)

	// Add multiple favorites
	inputs := []favorites.FavoriteInput{
		{CardName: "Card 1", SetName: "Set A", CardNumber: "1"},
		{CardName: "Card 2", SetName: "Set A", CardNumber: "2"},
		{CardName: "Card 3", SetName: "Set B", CardNumber: "1"},
	}
	for _, input := range inputs {
		_, _ = repo.Add(context.Background(), 1, input)
	}

	// Test list all
	favs, err := repo.List(context.Background(), 1, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(favs) != 3 {
		t.Errorf("List() returned %d favorites, want 3", len(favs))
	}

	// Test pagination with limit
	favs, err = repo.List(context.Background(), 1, 2, 0)
	if err != nil {
		t.Fatalf("List() with limit error = %v", err)
	}
	if len(favs) != 2 {
		t.Errorf("List() with limit returned %d favorites, want 2", len(favs))
	}

	// Test pagination with offset
	favs, err = repo.List(context.Background(), 1, 10, 2)
	if err != nil {
		t.Fatalf("List() with offset error = %v", err)
	}
	if len(favs) != 1 {
		t.Errorf("List() with offset returned %d favorites, want 1", len(favs))
	}
}

func TestFavoritesRepository_Count(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()
	repo := NewFavoritesRepository(db)

	// Empty count
	count, err := repo.Count(context.Background(), 1)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Count() empty = %d, want 0", count)
	}

	// Add some
	_, _ = repo.Add(context.Background(), 1, favorites.FavoriteInput{CardName: "Card 1", SetName: "Set A", CardNumber: "1"})
	_, _ = repo.Add(context.Background(), 1, favorites.FavoriteInput{CardName: "Card 2", SetName: "Set A", CardNumber: "2"})

	count, err = repo.Count(context.Background(), 1)
	if err != nil {
		t.Fatalf("Count() after add error = %v", err)
	}
	if count != 2 {
		t.Errorf("Count() = %d, want 2", count)
	}
}

func TestFavoritesRepository_IsFavorite(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()
	repo := NewFavoritesRepository(db)

	input := favorites.FavoriteInput{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
	}

	// Not favorited yet
	isFav, err := repo.IsFavorite(context.Background(), 1, input.CardName, input.SetName, input.CardNumber)
	if err != nil {
		t.Fatalf("IsFavorite() error = %v", err)
	}
	if isFav {
		t.Error("IsFavorite() = true, want false")
	}

	// Add and check again
	_, _ = repo.Add(context.Background(), 1, input)
	isFav, err = repo.IsFavorite(context.Background(), 1, input.CardName, input.SetName, input.CardNumber)
	if err != nil {
		t.Fatalf("IsFavorite() after add error = %v", err)
	}
	if !isFav {
		t.Error("IsFavorite() after add = false, want true")
	}
}

func TestFavoritesRepository_CheckMultiple(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()
	repo := NewFavoritesRepository(db)

	// Add some favorites
	_, _ = repo.Add(context.Background(), 1, favorites.FavoriteInput{CardName: "Card 1", SetName: "Set A", CardNumber: "1"})
	_, _ = repo.Add(context.Background(), 1, favorites.FavoriteInput{CardName: "Card 3", SetName: "Set A", CardNumber: "3"})

	// Check multiple
	cards := []favorites.FavoriteInput{
		{CardName: "Card 1", SetName: "Set A", CardNumber: "1"},
		{CardName: "Card 2", SetName: "Set A", CardNumber: "2"},
		{CardName: "Card 3", SetName: "Set A", CardNumber: "3"},
	}

	checks, err := repo.CheckMultiple(context.Background(), 1, cards)
	if err != nil {
		t.Fatalf("CheckMultiple() error = %v", err)
	}
	if len(checks) != 3 {
		t.Fatalf("CheckMultiple() returned %d checks, want 3", len(checks))
	}

	expected := map[string]bool{
		"Card 1|Set A|1": true,
		"Card 2|Set A|2": false,
		"Card 3|Set A|3": true,
	}

	for _, check := range checks {
		key := check.CardName + "|" + check.SetName + "|" + check.CardNumber
		if check.IsFavorite != expected[key] {
			t.Errorf("CheckMultiple() %s = %v, want %v", key, check.IsFavorite, expected[key])
		}
	}
}

func TestFavoritesRepository_CheckMultiple_Empty(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()
	repo := NewFavoritesRepository(db)

	checks, err := repo.CheckMultiple(context.Background(), 1, []favorites.FavoriteInput{})
	if err != nil {
		t.Fatalf("CheckMultiple() empty error = %v", err)
	}
	if len(checks) != 0 {
		t.Errorf("CheckMultiple() empty returned %d checks, want 0", len(checks))
	}
}

func TestFavoritesRepository_IsolatedUsers(t *testing.T) {
	db := setupFavoritesTestDB(t)
	defer db.Close()

	// Add a second user
	_, err := db.Exec(`INSERT INTO users (id, google_id, email) VALUES (2, 'test-google-id-2', 'test2@example.com');`)
	if err != nil {
		t.Fatalf("failed to insert second user: %v", err)
	}

	repo := NewFavoritesRepository(db)

	// Add favorite for user 1
	_, _ = repo.Add(context.Background(), 1, favorites.FavoriteInput{CardName: "Card 1", SetName: "Set A", CardNumber: "1"})

	// Add favorite for user 2
	_, _ = repo.Add(context.Background(), 2, favorites.FavoriteInput{CardName: "Card 2", SetName: "Set B", CardNumber: "2"})

	// User 1 should only see their favorite
	count1, _ := repo.Count(context.Background(), 1)
	if count1 != 1 {
		t.Errorf("User 1 count = %d, want 1", count1)
	}

	// User 2 should only see their favorite
	count2, _ := repo.Count(context.Background(), 2)
	if count2 != 1 {
		t.Errorf("User 2 count = %d, want 1", count2)
	}

	// User 1 should not see User 2's favorite
	isFav, _ := repo.IsFavorite(context.Background(), 1, "Card 2", "Set B", "2")
	if isFav {
		t.Error("User 1 should not see User 2's favorite")
	}
}
