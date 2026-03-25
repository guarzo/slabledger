package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/favorites"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// MockFavoritesService implements favorites.Service using the functional-field pattern.
// Each method delegates to a function field; nil fields return zero values.
type MockFavoritesService struct {
	AddFavoriteFn    func(ctx context.Context, userID int64, input favorites.FavoriteInput) (*favorites.Favorite, error)
	RemoveFavoriteFn func(ctx context.Context, userID int64, cardName, setName, cardNumber string) error
	GetFavoritesFn   func(ctx context.Context, userID int64, page, pageSize int) (*favorites.FavoritesList, error)
	IsFavoriteFn     func(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error)
	CheckFavoritesFn func(ctx context.Context, userID int64, cards []favorites.FavoriteInput) ([]favorites.FavoriteCheck, error)
	ToggleFavoriteFn func(ctx context.Context, userID int64, input favorites.FavoriteInput) (bool, error)
}

var _ favorites.Service = (*MockFavoritesService)(nil)

func (m *MockFavoritesService) AddFavorite(ctx context.Context, userID int64, input favorites.FavoriteInput) (*favorites.Favorite, error) {
	if m.AddFavoriteFn != nil {
		return m.AddFavoriteFn(ctx, userID, input)
	}
	return &favorites.Favorite{}, nil
}

func (m *MockFavoritesService) RemoveFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) error {
	if m.RemoveFavoriteFn != nil {
		return m.RemoveFavoriteFn(ctx, userID, cardName, setName, cardNumber)
	}
	return nil
}

func (m *MockFavoritesService) GetFavorites(ctx context.Context, userID int64, page, pageSize int) (*favorites.FavoritesList, error) {
	if m.GetFavoritesFn != nil {
		return m.GetFavoritesFn(ctx, userID, page, pageSize)
	}
	return &favorites.FavoritesList{Favorites: []favorites.Favorite{}}, nil
}

func (m *MockFavoritesService) IsFavorite(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error) {
	if m.IsFavoriteFn != nil {
		return m.IsFavoriteFn(ctx, userID, cardName, setName, cardNumber)
	}
	return false, nil
}

func (m *MockFavoritesService) CheckFavorites(ctx context.Context, userID int64, cards []favorites.FavoriteInput) ([]favorites.FavoriteCheck, error) {
	if m.CheckFavoritesFn != nil {
		return m.CheckFavoritesFn(ctx, userID, cards)
	}
	return []favorites.FavoriteCheck{}, nil
}

func (m *MockFavoritesService) ToggleFavorite(ctx context.Context, userID int64, input favorites.FavoriteInput) (bool, error) {
	if m.ToggleFavoriteFn != nil {
		return m.ToggleFavoriteFn(ctx, userID, input)
	}
	return false, nil
}

// newAuthenticatedContext returns a context with a test user attached.
func newAuthenticatedContext() context.Context {
	user := &auth.User{ID: 42, Username: "testuser", Email: "test@example.com"}
	return context.WithValue(context.Background(), middleware.UserContextKey, user)
}

// newFavoritesHandlers creates a FavoritesHandlers with the given mock service.
func newFavoritesHandlers(svc favorites.Service) *FavoritesHandlers {
	return NewFavoritesHandlers(svc, mocks.NewMockLogger())
}

// decodeFavErrorBody decodes a JSON error response body and returns the "error" value.
func decodeFavErrorBody(t *testing.T, body []byte) string {
	t.Helper()
	var resp map[string]string
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	msg, ok := resp["error"]
	if !ok {
		t.Fatal("expected 'error' key in JSON response")
	}
	return msg
}

// ---------------------------------------------------------------------------
// HandleListFavorites
// ---------------------------------------------------------------------------

func TestHandleListFavorites(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		withAuth   bool
		setupSvc   func() *MockFavoritesService
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:     "GET success",
			method:   http.MethodGet,
			withAuth: true,
			setupSvc: func() *MockFavoritesService {
				return &MockFavoritesService{
					GetFavoritesFn: func(_ context.Context, _ int64, _, _ int) (*favorites.FavoritesList, error) {
						return &favorites.FavoritesList{
							Favorites:  []favorites.Favorite{{ID: 1, CardName: "Charizard", SetName: "Base Set"}},
							Total:      1,
							Page:       1,
							PageSize:   20,
							TotalPages: 1,
						}, nil
					},
				}
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp favorites.FavoritesList
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.Total != 1 {
					t.Errorf("expected total=1, got %d", resp.Total)
				}
				if len(resp.Favorites) != 1 {
					t.Fatalf("expected 1 favorite, got %d", len(resp.Favorites))
				}
				if resp.Favorites[0].CardName != "Charizard" {
					t.Errorf("expected card_name=Charizard, got %s", resp.Favorites[0].CardName)
				}
			},
		},
		{
			name:       "GET unauthenticated",
			method:     http.MethodGet,
			withAuth:   false,
			setupSvc:   func() *MockFavoritesService { return &MockFavoritesService{} },
			wantStatus: http.StatusUnauthorized,
			checkBody: func(t *testing.T, body []byte) {
				decodeFavErrorBody(t, body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newFavoritesHandlers(tc.setupSvc())

			req := httptest.NewRequest(tc.method, "/api/favorites", nil)
			if tc.withAuth {
				req = req.WithContext(newAuthenticatedContext())
			}

			w := httptest.NewRecorder()
			h.HandleListFavorites(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, w.Body.Bytes())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HandleAddFavorite
// ---------------------------------------------------------------------------

func TestHandleAddFavorite(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		withAuth   bool
		body       any
		setupSvc   func() *MockFavoritesService
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:     "POST success",
			method:   http.MethodPost,
			withAuth: true,
			body:     favorites.FavoriteInput{CardName: "Pikachu", SetName: "Base Set", CardNumber: "58"},
			setupSvc: func() *MockFavoritesService {
				return &MockFavoritesService{
					AddFavoriteFn: func(_ context.Context, _ int64, input favorites.FavoriteInput) (*favorites.Favorite, error) {
						return &favorites.Favorite{ID: 1, CardName: input.CardName, SetName: input.SetName, CardNumber: input.CardNumber}, nil
					},
				}
			},
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var fav favorites.Favorite
				if err := json.Unmarshal(body, &fav); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if fav.CardName != "Pikachu" {
					t.Errorf("expected card_name=Pikachu, got %s", fav.CardName)
				}
			},
		},
		{
			name:       "POST missing body",
			method:     http.MethodPost,
			withAuth:   true,
			body:       nil,
			setupSvc:   func() *MockFavoritesService { return &MockFavoritesService{} },
			wantStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				msg := decodeFavErrorBody(t, body)
				if msg != "Invalid request body" {
					t.Errorf("expected error 'Invalid request body', got %q", msg)
				}
			},
		},
		{
			name:       "POST unauthenticated",
			method:     http.MethodPost,
			withAuth:   false,
			body:       favorites.FavoriteInput{CardName: "Pikachu", SetName: "Base Set"},
			setupSvc:   func() *MockFavoritesService { return &MockFavoritesService{} },
			wantStatus: http.StatusUnauthorized,
			checkBody: func(t *testing.T, body []byte) {
				decodeFavErrorBody(t, body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newFavoritesHandlers(tc.setupSvc())

			var bodyBytes []byte
			if tc.body != nil {
				var err error
				bodyBytes, err = json.Marshal(tc.body)
				if err != nil {
					t.Fatalf("failed to marshal body: %v", err)
				}
			} else {
				// Send invalid JSON to trigger decode error
				bodyBytes = []byte("{invalid")
			}

			req := httptest.NewRequest(tc.method, "/api/favorites", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			if tc.withAuth {
				req = req.WithContext(newAuthenticatedContext())
			}

			w := httptest.NewRecorder()
			h.HandleAddFavorite(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, w.Body.Bytes())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HandleRemoveFavorite
// ---------------------------------------------------------------------------

func TestHandleRemoveFavorite(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		withAuth   bool
		query      string
		setupSvc   func() *MockFavoritesService
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:     "DELETE success",
			method:   http.MethodDelete,
			withAuth: true,
			query:    "card_name=Pikachu&set_name=Base+Set&card_number=58",
			setupSvc: func() *MockFavoritesService {
				return &MockFavoritesService{
					RemoveFavoriteFn: func(_ context.Context, _ int64, _, _, _ string) error {
						return nil
					},
				}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "DELETE missing required params",
			method:     http.MethodDelete,
			withAuth:   true,
			query:      "",
			setupSvc:   func() *MockFavoritesService { return &MockFavoritesService{} },
			wantStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				msg := decodeFavErrorBody(t, body)
				if msg != "card_name and set_name are required" {
					t.Errorf("expected error about required params, got %q", msg)
				}
			},
		},
		{
			name:       "DELETE unauthenticated",
			method:     http.MethodDelete,
			withAuth:   false,
			query:      "card_name=Pikachu&set_name=Base+Set",
			setupSvc:   func() *MockFavoritesService { return &MockFavoritesService{} },
			wantStatus: http.StatusUnauthorized,
			checkBody: func(t *testing.T, body []byte) {
				decodeFavErrorBody(t, body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newFavoritesHandlers(tc.setupSvc())

			url := "/api/favorites"
			if tc.query != "" {
				url += "?" + tc.query
			}
			req := httptest.NewRequest(tc.method, url, nil)
			if tc.withAuth {
				req = req.WithContext(newAuthenticatedContext())
			}

			w := httptest.NewRecorder()
			h.HandleRemoveFavorite(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, w.Body.Bytes())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HandleToggleFavorite
// ---------------------------------------------------------------------------

func TestHandleToggleFavorite(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		withAuth   bool
		body       any
		setupSvc   func() *MockFavoritesService
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:     "POST success - toggled on",
			method:   http.MethodPost,
			withAuth: true,
			body:     favorites.FavoriteInput{CardName: "Charizard", SetName: "Base Set"},
			setupSvc: func() *MockFavoritesService {
				return &MockFavoritesService{
					ToggleFavoriteFn: func(_ context.Context, _ int64, _ favorites.FavoriteInput) (bool, error) {
						return true, nil
					},
				}
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]bool
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if !resp["is_favorite"] {
					t.Error("expected is_favorite=true")
				}
			},
		},
		{
			name:       "POST unauthenticated",
			method:     http.MethodPost,
			withAuth:   false,
			body:       favorites.FavoriteInput{CardName: "Charizard", SetName: "Base Set"},
			setupSvc:   func() *MockFavoritesService { return &MockFavoritesService{} },
			wantStatus: http.StatusUnauthorized,
			checkBody: func(t *testing.T, body []byte) {
				decodeFavErrorBody(t, body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newFavoritesHandlers(tc.setupSvc())

			bodyBytes, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("failed to marshal body: %v", err)
			}

			req := httptest.NewRequest(tc.method, "/api/favorites/toggle", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			if tc.withAuth {
				req = req.WithContext(newAuthenticatedContext())
			}

			w := httptest.NewRecorder()
			h.HandleToggleFavorite(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, w.Body.Bytes())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HandleCheckFavorites
// ---------------------------------------------------------------------------

func TestHandleCheckFavorites(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		withAuth   bool
		body       any
		setupSvc   func() *MockFavoritesService
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:     "POST success",
			method:   http.MethodPost,
			withAuth: true,
			body: []favorites.FavoriteInput{
				{CardName: "Pikachu", SetName: "Base Set", CardNumber: "58"},
				{CardName: "Charizard", SetName: "Base Set", CardNumber: "4"},
			},
			setupSvc: func() *MockFavoritesService {
				return &MockFavoritesService{
					CheckFavoritesFn: func(_ context.Context, _ int64, cards []favorites.FavoriteInput) ([]favorites.FavoriteCheck, error) {
						checks := make([]favorites.FavoriteCheck, len(cards))
						for i, c := range cards {
							checks[i] = favorites.FavoriteCheck{
								CardName:   c.CardName,
								SetName:    c.SetName,
								CardNumber: c.CardNumber,
								IsFavorite: c.CardName == "Pikachu",
							}
						}
						return checks, nil
					},
				}
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var checks []favorites.FavoriteCheck
				if err := json.Unmarshal(body, &checks); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(checks) != 2 {
					t.Fatalf("expected 2 checks, got %d", len(checks))
				}
				if !checks[0].IsFavorite {
					t.Error("expected Pikachu to be a favorite")
				}
				if checks[1].IsFavorite {
					t.Error("expected Charizard to not be a favorite")
				}
			},
		},
		{
			name:       "POST unauthenticated",
			method:     http.MethodPost,
			withAuth:   false,
			body:       []favorites.FavoriteInput{{CardName: "Pikachu", SetName: "Base Set"}},
			setupSvc:   func() *MockFavoritesService { return &MockFavoritesService{} },
			wantStatus: http.StatusUnauthorized,
			checkBody: func(t *testing.T, body []byte) {
				decodeFavErrorBody(t, body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newFavoritesHandlers(tc.setupSvc())

			bodyBytes, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("failed to marshal body: %v", err)
			}

			req := httptest.NewRequest(tc.method, "/api/favorites/check", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			if tc.withAuth {
				req = req.WithContext(newAuthenticatedContext())
			}

			w := httptest.NewRecorder()
			h.HandleCheckFavorites(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, w.Body.Bytes())
			}
		})
	}
}
