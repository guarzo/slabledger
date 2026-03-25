package favorites

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// mockRepository implements Repository for testing
type mockRepository struct {
	favorites   map[string]*Favorite
	shouldError error
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		favorites: make(map[string]*Favorite),
	}
}

func makeTestKey(userID int64, cardName, setName, cardNumber string) string {
	return fmt.Sprintf("%d|%s|%s|%s", userID, cardName, setName, cardNumber)
}

func (m *mockRepository) Add(ctx context.Context, userID int64, input FavoriteInput) (*Favorite, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if m.shouldError != nil {
		return nil, m.shouldError
	}
	key := makeTestKey(userID, input.CardName, input.SetName, input.CardNumber)
	if _, exists := m.favorites[key]; exists {
		return nil, ErrFavoriteAlreadyExists
	}
	fav := &Favorite{
		ID:         int64(len(m.favorites) + 1),
		UserID:     userID,
		CardName:   input.CardName,
		SetName:    input.SetName,
		CardNumber: input.CardNumber,
		ImageURL:   input.ImageURL,
		Notes:      input.Notes,
	}
	m.favorites[key] = fav
	return fav, nil
}

func (m *mockRepository) Remove(ctx context.Context, userID int64, cardName, setName, cardNumber string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if m.shouldError != nil {
		return m.shouldError
	}
	key := makeTestKey(userID, cardName, setName, cardNumber)
	if _, exists := m.favorites[key]; !exists {
		return ErrFavoriteNotFound
	}
	delete(m.favorites, key)
	return nil
}

func (m *mockRepository) List(ctx context.Context, userID int64, limit, offset int) ([]Favorite, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if m.shouldError != nil {
		return nil, m.shouldError
	}
	var result []Favorite
	for _, f := range m.favorites {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if f.UserID == userID {
			result = append(result, *f)
		}
	}
	// Apply offset and limit
	if offset >= len(result) {
		return []Favorite{}, nil
	}
	result = result[offset:]
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockRepository) Count(ctx context.Context, userID int64) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	if m.shouldError != nil {
		return 0, m.shouldError
	}
	count := 0
	for _, f := range m.favorites {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		if f.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) IsFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	if m.shouldError != nil {
		return false, m.shouldError
	}
	key := makeTestKey(userID, cardName, setName, cardNumber)
	_, exists := m.favorites[key]
	return exists, nil
}

func (m *mockRepository) CheckMultiple(ctx context.Context, userID int64, cards []FavoriteInput) ([]FavoriteCheck, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if m.shouldError != nil {
		return nil, m.shouldError
	}
	results := make([]FavoriteCheck, len(cards))
	for i, card := range cards {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		key := makeTestKey(userID, card.CardName, card.SetName, card.CardNumber)
		_, exists := m.favorites[key]
		results[i] = FavoriteCheck{
			CardName:   card.CardName,
			SetName:    card.SetName,
			CardNumber: card.CardNumber,
			IsFavorite: exists,
		}
	}
	return results, nil
}

// Tests

func TestService_AddFavorite(t *testing.T) {
	tests := []struct {
		name      string
		userID    int64
		input     FavoriteInput
		wantErr   bool
		errType   error
		setupMock func(*mockRepository)
	}{
		{
			name:   "successful add",
			userID: 1,
			input: FavoriteInput{
				CardName:   "Charizard ex",
				SetName:    "Obsidian Flames",
				CardNumber: "125",
			},
			wantErr: false,
		},
		{
			name:   "missing card name",
			userID: 1,
			input: FavoriteInput{
				SetName:    "Obsidian Flames",
				CardNumber: "125",
			},
			wantErr: true,
			errType: ErrCardNameRequired,
		},
		{
			name:   "missing set name",
			userID: 1,
			input: FavoriteInput{
				CardName:   "Charizard ex",
				CardNumber: "125",
			},
			wantErr: true,
			errType: ErrSetNameRequired,
		},
		{
			name:   "duplicate favorite",
			userID: 1,
			input: FavoriteInput{
				CardName:   "Charizard ex",
				SetName:    "Obsidian Flames",
				CardNumber: "125",
			},
			wantErr: true,
			errType: ErrFavoriteAlreadyExists,
			setupMock: func(m *mockRepository) {
				m.Add(context.Background(), 1, FavoriteInput{
					CardName:   "Charizard ex",
					SetName:    "Obsidian Flames",
					CardNumber: "125",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}
			svc := NewService(repo)

			_, err := svc.AddFavorite(context.Background(), tt.userID, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("AddFavorite() expected error, got nil")
				} else if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Errorf("AddFavorite() error = %v, want %v", err, tt.errType)
				}
			} else if err != nil {
				t.Errorf("AddFavorite() unexpected error = %v", err)
			}
		})
	}
}

func TestService_ToggleFavorite(t *testing.T) {
	tests := []struct {
		name      string
		userID    int64
		input     FavoriteInput
		wantState bool
		setupMock func(*mockRepository)
	}{
		{
			name:   "toggle on (add)",
			userID: 1,
			input: FavoriteInput{
				CardName:   "Pikachu",
				SetName:    "Base Set",
				CardNumber: "58",
			},
			wantState: true,
		},
		{
			name:   "toggle off (remove)",
			userID: 1,
			input: FavoriteInput{
				CardName:   "Pikachu",
				SetName:    "Base Set",
				CardNumber: "58",
			},
			wantState: false,
			setupMock: func(m *mockRepository) {
				m.Add(context.Background(), 1, FavoriteInput{
					CardName:   "Pikachu",
					SetName:    "Base Set",
					CardNumber: "58",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}
			svc := NewService(repo)

			gotState, err := svc.ToggleFavorite(context.Background(), tt.userID, tt.input)

			if err != nil {
				t.Errorf("ToggleFavorite() unexpected error = %v", err)
			}
			if gotState != tt.wantState {
				t.Errorf("ToggleFavorite() = %v, want %v", gotState, tt.wantState)
			}
		})
	}
}

func TestService_ToggleFavorite_ValidationError(t *testing.T) {
	repo := newMockRepository()
	svc := NewService(repo)

	// Empty card name should fail validation
	_, err := svc.ToggleFavorite(context.Background(), 1, FavoriteInput{
		CardName:   "",
		SetName:    "Base Set",
		CardNumber: "58",
	})

	if !errors.Is(err, ErrCardNameRequired) {
		t.Errorf("ToggleFavorite() error = %v, want %v", err, ErrCardNameRequired)
	}
}

func TestService_GetFavorites_Pagination(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
		wantPage int
		wantSize int
	}{
		{
			name:     "default pagination (zero values)",
			page:     0,
			pageSize: 0,
			wantPage: 1,
			wantSize: 20,
		},
		{
			name:     "negative values use defaults",
			page:     -1,
			pageSize: -5,
			wantPage: 1,
			wantSize: 20,
		},
		{
			name:     "custom pagination",
			page:     2,
			pageSize: 50,
			wantPage: 2,
			wantSize: 50,
		},
		{
			name:     "page size capped at 100",
			page:     1,
			pageSize: 200,
			wantPage: 1,
			wantSize: 20, // Falls back to default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			svc := NewService(repo)

			result, err := svc.GetFavorites(context.Background(), 1, tt.page, tt.pageSize)

			if err != nil {
				t.Errorf("GetFavorites() unexpected error = %v", err)
			}
			if result.Page != tt.wantPage {
				t.Errorf("GetFavorites() page = %v, want %v", result.Page, tt.wantPage)
			}
			if result.PageSize != tt.wantSize {
				t.Errorf("GetFavorites() pageSize = %v, want %v", result.PageSize, tt.wantSize)
			}
		})
	}
}

func TestService_IsFavorite(t *testing.T) {
	repo := newMockRepository()
	repo.Add(context.Background(), 1, FavoriteInput{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
	})
	svc := NewService(repo)

	// Test favorited card
	isFav, err := svc.IsFavorite(context.Background(), 1, "Charizard", "Base Set", "4")
	if err != nil {
		t.Fatalf("IsFavorite() error = %v", err)
	}
	if !isFav {
		t.Error("IsFavorite() = false, want true")
	}

	// Test non-favorited card
	isFav, err = svc.IsFavorite(context.Background(), 1, "Pikachu", "Base Set", "58")
	if err != nil {
		t.Fatalf("IsFavorite() error = %v", err)
	}
	if isFav {
		t.Error("IsFavorite() = true, want false")
	}
}

func TestService_CheckFavorites(t *testing.T) {
	repo := newMockRepository()
	repo.Add(context.Background(), 1, FavoriteInput{CardName: "Card1", SetName: "Set", CardNumber: "1"})
	repo.Add(context.Background(), 1, FavoriteInput{CardName: "Card3", SetName: "Set", CardNumber: "3"})
	svc := NewService(repo)

	cards := []FavoriteInput{
		{CardName: "Card1", SetName: "Set", CardNumber: "1"},
		{CardName: "Card2", SetName: "Set", CardNumber: "2"},
		{CardName: "Card3", SetName: "Set", CardNumber: "3"},
	}

	checks, err := svc.CheckFavorites(context.Background(), 1, cards)
	if err != nil {
		t.Fatalf("CheckFavorites() error = %v", err)
	}

	expected := map[string]bool{
		"Card1": true,
		"Card2": false,
		"Card3": true,
	}

	for _, check := range checks {
		if check.IsFavorite != expected[check.CardName] {
			t.Errorf("CheckFavorites() %s = %v, want %v",
				check.CardName, check.IsFavorite, expected[check.CardName])
		}
	}
}

func TestService_RemoveFavorite(t *testing.T) {
	repo := newMockRepository()
	repo.Add(context.Background(), 1, FavoriteInput{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
	})
	svc := NewService(repo)

	// Remove existing favorite
	err := svc.RemoveFavorite(context.Background(), 1, "Charizard", "Base Set", "4")
	if err != nil {
		t.Errorf("RemoveFavorite() error = %v", err)
	}

	// Try to remove non-existent favorite
	err = svc.RemoveFavorite(context.Background(), 1, "Pikachu", "Base Set", "58")
	if !errors.Is(err, ErrFavoriteNotFound) {
		t.Errorf("RemoveFavorite() error = %v, want %v", err, ErrFavoriteNotFound)
	}
}

func TestService_ContextCancellation(t *testing.T) {
	repo := newMockRepository()
	// Pre-populate with some favorites
	repo.Add(context.Background(), 1, FavoriteInput{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
	})
	repo.Add(context.Background(), 1, FavoriteInput{
		CardName:   "Pikachu",
		SetName:    "Base Set",
		CardNumber: "58",
	})
	svc := NewService(repo)

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Call GetFavorites with canceled context
	_, err := svc.GetFavorites(ctx, 1, 1, 20)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("GetFavorites() with canceled context: error = %v, want %v", err, context.Canceled)
	}
}
