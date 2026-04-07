package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCatalogSearcher struct {
	FetchCardCatalogFn func(ctx context.Context, query string, filters map[string]string, page, limit int) (*cardladder.SearchResponse[cardladder.CatalogCard], error)
}

func (m *mockCatalogSearcher) FetchCardCatalog(ctx context.Context, query string, filters map[string]string, page, limit int) (*cardladder.SearchResponse[cardladder.CatalogCard], error) {
	return m.FetchCardCatalogFn(ctx, query, filters, page, limit)
}

func TestCardCatalogHandler_Success(t *testing.T) {
	mock := &mockCatalogSearcher{
		FetchCardCatalogFn: func(_ context.Context, _ string, _ map[string]string, _, _ int) (*cardladder.SearchResponse[cardladder.CatalogCard], error) {
			return &cardladder.SearchResponse[cardladder.CatalogCard]{
				Hits: []cardladder.CatalogCard{
					{GemRateID: "abc123", Player: "Charizard", Set: "Pokemon Game", Condition: "PSA 10", CurrentValue: 500.0},
				},
				TotalHits: 1,
			}, nil
		},
	}
	h := NewCardCatalogHandler(mock, mocks.NewMockLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/catalog?q=Charizard&limit=10", nil)
	h.HandleSearch(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp cardladder.SearchResponse[cardladder.CatalogCard]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 1, resp.TotalHits)
	assert.Equal(t, "abc123", resp.Hits[0].GemRateID)
}

func TestCardCatalogHandler_MissingQuery(t *testing.T) {
	mock := &mockCatalogSearcher{
		FetchCardCatalogFn: func(_ context.Context, _ string, _ map[string]string, _, _ int) (*cardladder.SearchResponse[cardladder.CatalogCard], error) {
			return nil, nil
		},
	}
	h := NewCardCatalogHandler(mock, mocks.NewMockLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/catalog", nil)
	h.HandleSearch(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCardCatalogHandler_SearchError(t *testing.T) {
	mock := &mockCatalogSearcher{
		FetchCardCatalogFn: func(_ context.Context, _ string, _ map[string]string, _, _ int) (*cardladder.SearchResponse[cardladder.CatalogCard], error) {
			return nil, errors.New("api error")
		},
	}
	h := NewCardCatalogHandler(mock, mocks.NewMockLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/catalog?q=Charizard", nil)
	h.HandleSearch(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCardCatalogHandler_FiltersPassedThrough(t *testing.T) {
	var capturedQuery string
	var capturedFilters map[string]string
	mock := &mockCatalogSearcher{
		FetchCardCatalogFn: func(_ context.Context, query string, filters map[string]string, _, _ int) (*cardladder.SearchResponse[cardladder.CatalogCard], error) {
			capturedQuery = query
			capturedFilters = filters
			return &cardladder.SearchResponse[cardladder.CatalogCard]{}, nil
		},
	}
	h := NewCardCatalogHandler(mock, mocks.NewMockLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/catalog?q=Charizard&condition=PSA+10&set=Pokemon+Game", nil)
	h.HandleSearch(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Charizard", capturedQuery)
	assert.Equal(t, "PSA 10", capturedFilters["condition"])
	assert.Equal(t, "Pokemon Game", capturedFilters["set"])
}
