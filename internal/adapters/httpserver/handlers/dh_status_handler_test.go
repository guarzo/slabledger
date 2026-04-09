package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- local test doubles for DHHandler interfaces ---

// mockDHIntelCounter implements DHIntelligenceCounter.
type mockDHIntelCounter struct {
	CountAllFn        func(ctx context.Context) (int, error)
	LatestFetchedAtFn func(ctx context.Context) (string, error)
}

func (m *mockDHIntelCounter) CountAll(ctx context.Context) (int, error) {
	if m.CountAllFn != nil {
		return m.CountAllFn(ctx)
	}
	return 0, nil
}

func (m *mockDHIntelCounter) LatestFetchedAt(ctx context.Context) (string, error) {
	if m.LatestFetchedAtFn != nil {
		return m.LatestFetchedAtFn(ctx)
	}
	return "", nil
}

// mockDHSuggestCounter implements DHSuggestionsCounter.
type mockDHSuggestCounter struct {
	CountLatestFn     func(ctx context.Context) (int, error)
	LatestFetchedAtFn func(ctx context.Context) (string, error)
}

func (m *mockDHSuggestCounter) CountLatest(ctx context.Context) (int, error) {
	if m.CountLatestFn != nil {
		return m.CountLatestFn(ctx)
	}
	return 0, nil
}

func (m *mockDHSuggestCounter) LatestFetchedAt(ctx context.Context) (string, error) {
	if m.LatestFetchedAtFn != nil {
		return m.LatestFetchedAtFn(ctx)
	}
	return "", nil
}

// mockDHStatusCounter implements DHStatusCounter.
type mockDHStatusCounter struct {
	CountUnsoldByDHPushStatusFn func(ctx context.Context) (map[string]int, error)
}

func (m *mockDHStatusCounter) CountUnsoldByDHPushStatus(ctx context.Context) (map[string]int, error) {
	if m.CountUnsoldByDHPushStatusFn != nil {
		return m.CountUnsoldByDHPushStatusFn(ctx)
	}
	return map[string]int{}, nil
}

// mockDHHealthReporter implements DHHealthReporter.
type mockDHHealthReporter struct {
	tracker *dh.HealthTracker
}

func (m *mockDHHealthReporter) Health() *dh.HealthTracker {
	return m.tracker
}

// mockDHCountsFetcher implements DHCountsFetcher.
type mockDHCountsFetcher struct {
	ListInventoryFn func(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
	GetOrdersFn     func(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
}

func (m *mockDHCountsFetcher) ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	if m.ListInventoryFn != nil {
		return m.ListInventoryFn(ctx, filters)
	}
	return &dh.InventoryListResponse{}, nil
}

func (m *mockDHCountsFetcher) GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error) {
	if m.GetOrdersFn != nil {
		return m.GetOrdersFn(ctx, filters)
	}
	return &dh.OrdersResponse{}, nil
}

// mockDHPurchaseLister implements DHPurchaseLister.
type mockDHPurchaseLister struct {
	ListAllUnsoldPurchasesFn func(ctx context.Context) ([]campaigns.Purchase, error)
	GetPurchaseFn            func(ctx context.Context, id string) (*campaigns.Purchase, error)
}

func (m *mockDHPurchaseLister) ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error) {
	if m.ListAllUnsoldPurchasesFn != nil {
		return m.ListAllUnsoldPurchasesFn(ctx)
	}
	return []campaigns.Purchase{}, nil
}

func (m *mockDHPurchaseLister) GetPurchase(ctx context.Context, id string) (*campaigns.Purchase, error) {
	if m.GetPurchaseFn != nil {
		return m.GetPurchaseFn(ctx, id)
	}
	return nil, nil
}

// --- helpers ---

// newTestDHHandler creates a minimal DHHandler for status/intelligence/suggestions tests.
func newTestDHHandler(
	intelRepo intelligence.Repository,
	suggestionsRepo intelligence.SuggestionsRepository,
	intelCounter DHIntelligenceCounter,
	suggestCounter DHSuggestionsCounter,
	statusCounter DHStatusCounter,
	healthReporter DHHealthReporter,
	countsFetcher DHCountsFetcher,
	purchaseLister DHPurchaseLister,
) *DHHandler {
	return NewDHHandler(
		nil, // certResolver — not needed for these tests
		nil, // cardIDSaver
		purchaseLister,
		nil, // inventoryPusher
		nil, // dhFieldsUpdater
		nil, // pushStatusUpdater
		nil, // candidatesSaver
		statusCounter,
		intelRepo,
		suggestionsRepo,
		intelCounter,
		suggestCounter,
		mocks.NewMockLogger(),
		context.Background(),
		healthReporter,
		countsFetcher,
		nil, // dhApproveService — not needed for these tests
		nil, // matchConfirmer — not needed for these tests
	)
}

// authenticatedRequest wraps a request with a test user in its context.
func authenticatedRequest(req *http.Request) *http.Request {
	user := &auth.User{ID: 1, Username: "testuser", Email: "test@example.com"}
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	return req.WithContext(ctx)
}

// --- HandleGetStatus ---

func TestHandleGetStatus_Unauthenticated(t *testing.T) {
	h := newTestDHHandler(nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/dh/status", nil)
	rec := httptest.NewRecorder()
	h.HandleGetStatus(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleGetStatus_BasicSuccess(t *testing.T) {
	h := newTestDHHandler(nil, nil, nil, nil, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/status", nil))
	rec := httptest.NewRecorder()
	h.HandleGetStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp dhStatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.False(t, resp.BulkMatchRunning)
	assert.Empty(t, resp.BulkMatchError)
}

func TestHandleGetStatus_WithIntelCounters(t *testing.T) {
	intelCounter := &mockDHIntelCounter{
		CountAllFn: func(_ context.Context) (int, error) {
			return 42, nil
		},
		LatestFetchedAtFn: func(_ context.Context) (string, error) {
			return "2026-04-07T10:00:00Z", nil
		},
	}
	suggestCounter := &mockDHSuggestCounter{
		CountLatestFn: func(_ context.Context) (int, error) {
			return 15, nil
		},
		LatestFetchedAtFn: func(_ context.Context) (string, error) {
			return "2026-04-07T09:00:00Z", nil
		},
	}

	h := newTestDHHandler(nil, nil, intelCounter, suggestCounter, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/status", nil))
	rec := httptest.NewRecorder()
	h.HandleGetStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp dhStatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 42, resp.IntelligenceCount)
	assert.Equal(t, "2026-04-07T10:00:00Z", resp.IntelligenceLastFetch)
	assert.Equal(t, 15, resp.SuggestionsCount)
	assert.Equal(t, "2026-04-07T09:00:00Z", resp.SuggestionsLastFetch)
}

func TestHandleGetStatus_WithStatusCounters(t *testing.T) {
	statusCounter := &mockDHStatusCounter{
		CountUnsoldByDHPushStatusFn: func(_ context.Context) (map[string]int, error) {
			return map[string]int{
				campaigns.DHPushStatusUnmatched: 10,
				campaigns.DHPushStatusPending:   5,
				campaigns.DHPushStatusMatched:   3,
				campaigns.DHPushStatusManual:    2,
			}, nil
		},
	}

	h := newTestDHHandler(nil, nil, nil, nil, statusCounter, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/status", nil))
	rec := httptest.NewRecorder()
	h.HandleGetStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp dhStatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 10, resp.UnmatchedCount)
	assert.Equal(t, 5, resp.PendingCount)
	assert.Equal(t, 5, resp.MappedCount) // matched(3) + manual(2)
}

func TestHandleGetStatus_StatusCounterError(t *testing.T) {
	statusCounter := &mockDHStatusCounter{
		CountUnsoldByDHPushStatusFn: func(_ context.Context) (map[string]int, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	h := newTestDHHandler(nil, nil, nil, nil, statusCounter, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/status", nil))
	rec := httptest.NewRecorder()
	h.HandleGetStatus(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}

func TestHandleGetStatus_WithHealthReporter(t *testing.T) {
	tracker := dh.NewHealthTracker()
	tracker.RecordSuccess()
	tracker.RecordSuccess()
	tracker.RecordFailure()

	reporter := &mockDHHealthReporter{tracker: tracker}
	h := newTestDHHandler(nil, nil, nil, nil, nil, reporter, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/status", nil))
	rec := httptest.NewRecorder()
	h.HandleGetStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp dhStatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.NotNil(t, resp.APIHealth)
	assert.Equal(t, 3, resp.APIHealth.TotalCalls)
	assert.Equal(t, 1, resp.APIHealth.Failures)
}

func TestHandleGetStatus_WithCountsFetcher(t *testing.T) {
	fetcher := &mockDHCountsFetcher{
		ListInventoryFn: func(_ context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error) {
			resp := &dh.InventoryListResponse{}
			if filters.Status == "listed" {
				resp.Meta.TotalCount = 7
			} else {
				resp.Meta.TotalCount = 20
			}
			return resp, nil
		},
		GetOrdersFn: func(_ context.Context, _ dh.OrderFilters) (*dh.OrdersResponse, error) {
			return &dh.OrdersResponse{Meta: dh.PaginationMeta{TotalCount: 100}}, nil
		},
	}

	h := newTestDHHandler(nil, nil, nil, nil, nil, nil, fetcher, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/status", nil))
	rec := httptest.NewRecorder()
	h.HandleGetStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp dhStatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 20, resp.DHInventoryCount)
	assert.Equal(t, 7, resp.DHListingsCount)
	assert.Equal(t, 100, resp.DHOrdersCount)
}

// --- HandleGetIntelligence ---

func TestHandleGetIntelligence_Unauthenticated(t *testing.T) {
	h := newTestDHHandler(mocks.NewMockIntelligenceRepository(), nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/dh/intelligence?card_name=Charizard&set_name=Base+Set", nil)
	rec := httptest.NewRecorder()
	h.HandleGetIntelligence(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleGetIntelligence_MissingParams(t *testing.T) {
	h := newTestDHHandler(mocks.NewMockIntelligenceRepository(), nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name  string
		query string
	}{
		{"missing card_name", "?set_name=Base+Set"},
		{"missing set_name", "?card_name=Charizard"},
		{"missing both", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/intelligence"+tt.query, nil))
			rec := httptest.NewRecorder()
			h.HandleGetIntelligence(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			decodeErrorResponse(t, rec)
		})
	}
}

func TestHandleGetIntelligence_Found(t *testing.T) {
	intelRepo := &mocks.MockIntelligenceRepository{
		GetByCardFn: func(_ context.Context, cardName, setName, cardNumber string) (*intelligence.MarketIntelligence, error) {
			return &intelligence.MarketIntelligence{
				CardName: cardName,
				SetName:  setName,
			}, nil
		},
	}

	h := newTestDHHandler(intelRepo, nil, nil, nil, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/intelligence?card_name=Charizard&set_name=Base+Set", nil))
	rec := httptest.NewRecorder()
	h.HandleGetIntelligence(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp intelligence.MarketIntelligence
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "Charizard", resp.CardName)
}

func TestHandleGetIntelligence_NotFound(t *testing.T) {
	intelRepo := &mocks.MockIntelligenceRepository{
		GetByCardFn: func(_ context.Context, _, _, _ string) (*intelligence.MarketIntelligence, error) {
			return nil, nil
		},
	}

	h := newTestDHHandler(intelRepo, nil, nil, nil, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/intelligence?card_name=Unknown&set_name=Base+Set", nil))
	rec := httptest.NewRecorder()
	h.HandleGetIntelligence(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	decodeErrorResponse(t, rec)
}

func TestHandleGetIntelligence_RepoError(t *testing.T) {
	intelRepo := &mocks.MockIntelligenceRepository{
		GetByCardFn: func(_ context.Context, _, _, _ string) (*intelligence.MarketIntelligence, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	h := newTestDHHandler(intelRepo, nil, nil, nil, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/intelligence?card_name=Charizard&set_name=Base+Set", nil))
	rec := httptest.NewRecorder()
	h.HandleGetIntelligence(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}

// --- HandleGetSuggestions ---

func TestHandleGetSuggestions_Unauthenticated(t *testing.T) {
	h := newTestDHHandler(nil, mocks.NewMockSuggestionsRepository(), nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/dh/suggestions", nil)
	rec := httptest.NewRecorder()
	h.HandleGetSuggestions(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleGetSuggestions_Success(t *testing.T) {
	suggestRepo := &mocks.MockSuggestionsRepository{
		GetLatestFn: func(_ context.Context) ([]intelligence.Suggestion, error) {
			return []intelligence.Suggestion{
				{CardName: "Pikachu", SetName: "Base Set"},
				{CardName: "Blastoise", SetName: "Base Set"},
			}, nil
		},
	}

	h := newTestDHHandler(nil, suggestRepo, nil, nil, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/suggestions", nil))
	rec := httptest.NewRecorder()
	h.HandleGetSuggestions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, string(resp["count"]), "2")
}

func TestHandleGetSuggestions_Empty(t *testing.T) {
	suggestRepo := &mocks.MockSuggestionsRepository{
		GetLatestFn: func(_ context.Context) ([]intelligence.Suggestion, error) {
			return nil, nil
		},
	}

	h := newTestDHHandler(nil, suggestRepo, nil, nil, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/suggestions", nil))
	rec := httptest.NewRecorder()
	h.HandleGetSuggestions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	// suggestions key should be a JSON array, not null
	assert.Equal(t, json.RawMessage("[]"), resp["suggestions"])
}

func TestHandleGetSuggestions_RepoError(t *testing.T) {
	suggestRepo := &mocks.MockSuggestionsRepository{
		GetLatestFn: func(_ context.Context) ([]intelligence.Suggestion, error) {
			return nil, fmt.Errorf("storage error")
		},
	}

	h := newTestDHHandler(nil, suggestRepo, nil, nil, nil, nil, nil, nil)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/suggestions", nil))
	rec := httptest.NewRecorder()
	h.HandleGetSuggestions(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}

// --- HandleInventoryAlerts ---

func TestHandleInventoryAlerts_Unauthenticated(t *testing.T) {
	h := newTestDHHandler(nil, mocks.NewMockSuggestionsRepository(), nil, nil, nil, nil, nil, &mockDHPurchaseLister{})
	req := httptest.NewRequest(http.MethodGet, "/api/dh/inventory-alerts", nil)
	rec := httptest.NewRecorder()
	h.HandleInventoryAlerts(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleInventoryAlerts_NoMatches(t *testing.T) {
	suggestRepo := &mocks.MockSuggestionsRepository{
		GetLatestFn: func(_ context.Context) ([]intelligence.Suggestion, error) {
			return []intelligence.Suggestion{
				{CardName: "Charizard", SetName: "Base Set", CardNumber: "004"},
			}, nil
		},
	}
	purchaseLister := &mockDHPurchaseLister{
		ListAllUnsoldPurchasesFn: func(_ context.Context) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{
				{CardName: "Pikachu", SetName: "Base Set", CardNumber: "025"},
			}, nil
		},
	}

	h := newTestDHHandler(nil, suggestRepo, nil, nil, nil, nil, nil, purchaseLister)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/inventory-alerts", nil))
	rec := httptest.NewRecorder()
	h.HandleInventoryAlerts(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, json.RawMessage("[]"), resp["alerts"])
	assert.Equal(t, json.RawMessage("0"), resp["count"])
}

func TestHandleInventoryAlerts_WithMatches(t *testing.T) {
	suggestRepo := &mocks.MockSuggestionsRepository{
		GetLatestFn: func(_ context.Context) ([]intelligence.Suggestion, error) {
			return []intelligence.Suggestion{
				{CardName: "Charizard", SetName: "Base Set", CardNumber: "004"},
				{CardName: "Blastoise", SetName: "Base Set", CardNumber: "002"},
			}, nil
		},
	}
	purchaseLister := &mockDHPurchaseLister{
		ListAllUnsoldPurchasesFn: func(_ context.Context) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{
				{CardName: "Charizard", SetName: "Base Set", CardNumber: "004"},
				{CardName: "Pikachu", SetName: "Base Set", CardNumber: "025"},
			}, nil
		},
	}

	h := newTestDHHandler(nil, suggestRepo, nil, nil, nil, nil, nil, purchaseLister)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/inventory-alerts", nil))
	rec := httptest.NewRecorder()
	h.HandleInventoryAlerts(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, json.RawMessage("1"), resp["count"])
}

func TestHandleInventoryAlerts_SuggestionsError(t *testing.T) {
	suggestRepo := &mocks.MockSuggestionsRepository{
		GetLatestFn: func(_ context.Context) ([]intelligence.Suggestion, error) {
			return nil, fmt.Errorf("suggestions fetch failed")
		},
	}

	h := newTestDHHandler(nil, suggestRepo, nil, nil, nil, nil, nil, &mockDHPurchaseLister{})
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/inventory-alerts", nil))
	rec := httptest.NewRecorder()
	h.HandleInventoryAlerts(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}

func TestHandleInventoryAlerts_PurchasesError(t *testing.T) {
	suggestRepo := &mocks.MockSuggestionsRepository{
		GetLatestFn: func(_ context.Context) ([]intelligence.Suggestion, error) {
			return []intelligence.Suggestion{{CardName: "Charizard", SetName: "Base Set"}}, nil
		},
	}
	purchaseLister := &mockDHPurchaseLister{
		ListAllUnsoldPurchasesFn: func(_ context.Context) ([]campaigns.Purchase, error) {
			return nil, fmt.Errorf("purchase list failed")
		},
	}

	h := newTestDHHandler(nil, suggestRepo, nil, nil, nil, nil, nil, purchaseLister)
	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/inventory-alerts", nil))
	rec := httptest.NewRecorder()
	h.HandleInventoryAlerts(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}
