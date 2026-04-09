package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildMMSearchResponse builds a tRPC-enveloped collectibles search response.
func buildMMSearchResponse(t *testing.T, items []map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"result": map[string]any{
			"data": map[string]any{"items": items},
		},
	})
	require.NoError(t, err)
	return body
}

// newMMSchedulerWithServer creates a MarketMoversRefreshScheduler whose client
// points to the given httptest server.
func newMMSchedulerWithServer(srv *httptest.Server) *MarketMoversRefreshScheduler {
	client := marketmovers.NewClient(
		marketmovers.WithClientBaseURL(srv.URL),
		marketmovers.WithStaticToken("test-token"),
	)
	return &MarketMoversRefreshScheduler{
		StopHandle: NewStopHandle(),
		client:     client,
		logger:     mocks.NewMockLogger(),
		config:     config.MarketMoversConfig{Enabled: true, RefreshHour: 5},
	}
}

// ---------------------------------------------------------------------------
// resolveCollectibleID — empty card name guard
// ---------------------------------------------------------------------------

func TestResolveCollectibleID_EmptyCardName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP call expected for empty card name")
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.resolveCollectibleID(context.Background(), &campaigns.Purchase{
		CertNumber: "12345678",
		CardName:   "",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), id)
}

// ---------------------------------------------------------------------------
// searchByCert
// ---------------------------------------------------------------------------

func TestSearchByCert_MatchingTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(999), "searchTitle": "Charizard Base Set PSA 10", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByCert(context.Background(), &campaigns.Purchase{
		CertNumber: "12345678",
		CardName:   "Charizard",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(999), id, "should return the collectible ID when title matches card name")
}

func TestSearchByCert_TitleMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Search result title does NOT contain card name — irrelevant hit
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(777), "searchTitle": "Pikachu VMAX PSA 10", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByCert(context.Background(), &campaigns.Purchase{
		CertNumber: "12345678",
		CardName:   "Charizard",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), id, "title mismatch should return 0")
}

func TestSearchByCert_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMMSearchResponse(t, nil))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByCert(context.Background(), &campaigns.Purchase{
		CertNumber: "00000000",
		CardName:   "Charizard",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), id)
}

func TestSearchByCert_CaseInsensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(42), "searchTitle": "UMBREON EX Full Art PSA 10", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByCert(context.Background(), &campaigns.Purchase{
		CertNumber: "87654321",
		CardName:   "Umbreon Ex",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(42), id, "title match should be case-insensitive")
}

// ---------------------------------------------------------------------------
// searchByNameGrade
// ---------------------------------------------------------------------------

func TestSearchByNameGrade_MatchingTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(55), "searchTitle": "Mewtwo Base Set PSA 9", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByNameGrade(context.Background(), &campaigns.Purchase{
		CardName:   "Mewtwo",
		Grader:     "PSA",
		GradeValue: 9,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(55), id)
}

func TestSearchByNameGrade_TitleMismatch_ReturnsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Top result is a completely different card — relevance check should reject it
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(88), "searchTitle": "Blastoise Base Set PSA 10", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByNameGrade(context.Background(), &campaigns.Purchase{
		CardName:   "Charizard",
		Grader:     "PSA",
		GradeValue: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), id, "title mismatch should return 0 rather than caching a wrong ID")
}

func TestSearchByNameGrade_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMMSearchResponse(t, nil))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByNameGrade(context.Background(), &campaigns.Purchase{
		CardName:   "Raichu",
		Grader:     "PSA",
		GradeValue: 8,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), id)
}

func TestSearchByNameGrade_EmptyGrader_DefaultsPSA(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("input")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMMSearchResponse(t, nil))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	_, _, _, _ = s.searchByNameGrade(context.Background(), &campaigns.Purchase{
		CardName:   "Bulbasaur",
		Grader:     "", // empty — should default to PSA
		GradeValue: 7,
	})
	assert.Contains(t, gotQuery, "PSA", "empty grader should default to PSA in the query")
}

// ---------------------------------------------------------------------------
// resolveCollectibleID — fallback logic
// ---------------------------------------------------------------------------

// callCount tracks how many times a path is hit across multiple handler calls.
func TestResolveCollectibleID_CertSucceeds_NoNameFallback(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// Always return a matching result
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(100), "searchTitle": "Venusaur Base Set PSA 10", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.resolveCollectibleID(context.Background(), &campaigns.Purchase{
		CertNumber: "11111111",
		CardName:   "Venusaur",
		Grader:     "PSA",
		GradeValue: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(100), id)
	assert.Equal(t, 1, callCount, "cert search succeeded — name search should not have been called")
}

func TestResolveCollectibleID_CertMisses_NameFallbackSucceeds(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// Cert search: return an unrelated result (title mismatch → cert path returns 0)
			_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
				{"item": map[string]any{"id": float64(99), "searchTitle": "Pikachu VMAX PSA 10", "collectibleType": "sports-card"}},
			}))
		} else {
			// Name+grade fallback: return the correct card
			_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
				{"item": map[string]any{"id": float64(200), "searchTitle": "Venusaur Base Set PSA 10", "collectibleType": "sports-card"}},
			}))
		}
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.resolveCollectibleID(context.Background(), &campaigns.Purchase{
		CertNumber: "22222222",
		CardName:   "Venusaur",
		Grader:     "PSA",
		GradeValue: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(200), id, "should return the name-search result after cert miss")
	assert.Equal(t, 2, callCount, "both cert and name searches should have been called")
}

func TestResolveCollectibleID_NoCertNumber_GoesDirectToNameSearch(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(300), "searchTitle": "Charmander Base Set PSA 9", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.resolveCollectibleID(context.Background(), &campaigns.Purchase{
		CertNumber: "", // no cert — skip cert search entirely
		CardName:   "Charmander",
		Grader:     "PSA",
		GradeValue: 9,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(300), id)
	assert.Equal(t, 1, callCount, "no cert number — only one search should be made")
}
