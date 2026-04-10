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

// ---------------------------------------------------------------------------
// computeMMSignals
// ---------------------------------------------------------------------------

func TestComputeMMSignals(t *testing.T) {
	cases := []struct {
		name      string
		items     []marketmovers.DailyStatItem
		wantAvg   float64
		wantTrend float64
		wantSales int
	}{
		{
			name:      "empty items",
			items:     nil,
			wantAvg:   0,
			wantTrend: 0,
			wantSales: 0,
		},
		{
			name: "single day",
			items: []marketmovers.DailyStatItem{
				{TotalSalesCount: 5, TotalSalesAmount: 1000, AverageSalePrice: 200},
			},
			wantAvg:   200, // 1000/5
			wantTrend: 0,   // first == last -> 0%
			wantSales: 5,
		},
		{
			name: "multiple days ascending trend",
			items: []marketmovers.DailyStatItem{
				{TotalSalesCount: 2, TotalSalesAmount: 200, AverageSalePrice: 100},
				{TotalSalesCount: 3, TotalSalesAmount: 600, AverageSalePrice: 200},
			},
			wantAvg:   160, // 800/5
			wantTrend: 1.0, // (200-100)/100 = +100%
			wantSales: 5,
		},
		{
			name: "descending trend",
			items: []marketmovers.DailyStatItem{
				{TotalSalesCount: 2, TotalSalesAmount: 400, AverageSalePrice: 200},
				{TotalSalesCount: 2, TotalSalesAmount: 200, AverageSalePrice: 100},
			},
			wantAvg:   150,  // 600/4
			wantTrend: -0.5, // (100-200)/200 = -50%
			wantSales: 4,
		},
		{
			name: "trend capped at +200%",
			items: []marketmovers.DailyStatItem{
				{TotalSalesCount: 1, TotalSalesAmount: 100, AverageSalePrice: 100},
				{TotalSalesCount: 1, TotalSalesAmount: 1000, AverageSalePrice: 1000},
			},
			wantAvg:   550, // 1100/2
			wantTrend: 2.0, // (1000-100)/100 = 900% -> capped to 200%
			wantSales: 2,
		},
		{
			name: "negative trend near theoretical limit",
			items: []marketmovers.DailyStatItem{
				{TotalSalesCount: 1, TotalSalesAmount: 1000, AverageSalePrice: 1000},
				{TotalSalesCount: 1, TotalSalesAmount: 1, AverageSalePrice: 1},
			},
			wantAvg:   500.5,  // 1001/2
			wantTrend: -0.999, // (1-1000)/1000 = -99.9%; can't reach -200% cap with positive prices
			wantSales: 2,
		},
		{
			name: "zero-count days skipped",
			items: []marketmovers.DailyStatItem{
				{TotalSalesCount: 3, TotalSalesAmount: 300, AverageSalePrice: 100},
				{TotalSalesCount: 0, TotalSalesAmount: 0, AverageSalePrice: 0},
				{TotalSalesCount: 2, TotalSalesAmount: 400, AverageSalePrice: 200},
			},
			wantAvg:   140, // 700/5
			wantTrend: 1.0, // (200-100)/100 = +100%
			wantSales: 5,
		},
		{
			name: "all zero-count days",
			items: []marketmovers.DailyStatItem{
				{TotalSalesCount: 0, TotalSalesAmount: 0, AverageSalePrice: 0},
				{TotalSalesCount: 0, TotalSalesAmount: 0, AverageSalePrice: 0},
			},
			wantAvg:   0,
			wantTrend: 0,
			wantSales: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			avg, trend, sales := computeMMSignals(tc.items)
			assert.InDelta(t, tc.wantAvg, avg, 0.01, "avgPrice")
			assert.InDelta(t, tc.wantTrend, trend, 0.01, "trendPct")
			assert.Equal(t, tc.wantSales, sales, "sales30d")
		})
	}
}

// ---------------------------------------------------------------------------
// resolveCollectibleID — fallback logic (continued)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// tokenMatchesTitle — tokenized matching tests
// ---------------------------------------------------------------------------

func TestTokenMatchesTitle(t *testing.T) {
	cases := []struct {
		name        string
		cardName    string
		searchTitle string
		want        bool
	}{
		{
			name:        "exact match short name",
			cardName:    "Charizard",
			searchTitle: "Charizard Base Set PSA 10",
			want:        true,
		},
		{
			name:        "PSA title reordered in MM",
			cardName:    "2022 POKEMON SWORD & SHIELD BRILLIANT STARS CHARIZARD VSTAR",
			searchTitle: "Charizard VSTAR 2022 Sword & Shield Brilliant Stars PSA 10",
			want:        true,
		},
		{
			name:        "completely different card",
			cardName:    "Charizard",
			searchTitle: "Pikachu VMAX PSA 10",
			want:        false,
		},
		{
			name:        "case insensitive",
			cardName:    "Umbreon EX",
			searchTitle: "UMBREON EX Full Art PSA 10",
			want:        true,
		},
		{
			name:        "noise words filtered",
			cardName:    "Pokemon Holo Card Charizard",
			searchTitle: "Charizard PSA 10",
			want:        true,
		},
		{
			name:        "no significant tokens falls back to contains",
			cardName:    "ab cd",
			searchTitle: "ab cd ef",
			want:        true,
		},
		{
			name:        "no significant tokens contains fails",
			cardName:    "ab cd",
			searchTitle: "ef gh",
			want:        false,
		},
		{
			name:        "empty cardName",
			cardName:    "",
			searchTitle: "Charizard PSA 10",
			want:        true,
		},
		{
			name:        "partial token overlap below 60%",
			cardName:    "Alpha Beta Gamma Delta Epsilon",
			searchTitle: "Alpha PSA 10",
			want:        false,
		},
		{
			name:        "60% threshold exactly met",
			cardName:    "Brilliant Stars Charizard VSTAR 2022",
			searchTitle: "Charizard VSTAR Brilliant Stars PSA 10",
			want:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenMatchesTitle(tc.cardName, tc.searchTitle)
			assert.Equal(t, tc.want, got, "tokenMatchesTitle(%q, %q)", tc.cardName, tc.searchTitle)
		})
	}
}

// ---------------------------------------------------------------------------
// searchByCert — tokenized matching integration test
// ---------------------------------------------------------------------------

func TestSearchByCert_TokenizedMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// MM returns title with reordered tokens — would fail with strings.Contains
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(5555), "searchTitle": "Charizard VSTAR 2022 Sword & Shield Brilliant Stars PSA 10", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByCert(context.Background(), &campaigns.Purchase{
		CertNumber: "12345678",
		CardName:   "2022 POKEMON SWORD & SHIELD BRILLIANT STARS CHARIZARD VSTAR",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5555), id, "should match despite token reordering")
}

// ---------------------------------------------------------------------------
// searchByNameGrade — tokenized matching integration test
// ---------------------------------------------------------------------------

func TestSearchByNameGrade_TokenizedMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// MM returns title with reordered tokens — would fail with strings.Contains
		_, _ = w.Write(buildMMSearchResponse(t, []map[string]any{
			{"item": map[string]any{"id": float64(6666), "searchTitle": "Charizard VSTAR 2022 Sword & Shield Brilliant Stars PSA 10", "collectibleType": "sports-card"}},
		}))
	}))
	defer srv.Close()

	s := newMMSchedulerWithServer(srv)
	id, _, _, err := s.searchByNameGrade(context.Background(), &campaigns.Purchase{
		CardName:   "2022 POKEMON SWORD & SHIELD BRILLIANT STARS CHARIZARD VSTAR",
		Grader:     "PSA",
		GradeValue: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(6666), id, "should match despite token reordering")
}
