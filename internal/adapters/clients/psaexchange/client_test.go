package psaexchange_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/adapters/clients/psaexchange"
)

// newTestClient builds a psaexchange.Client pointed at srv using a real httpx.Client.
func newTestClient(t *testing.T, srv *httptest.Server) *psaexchange.Client {
	t.Helper()
	cfg := httpx.DefaultConfig("psaexchange-test")
	httpClient := httpx.NewClient(cfg)
	return psaexchange.NewClient(httpClient,
		psaexchange.WithBaseURL(srv.URL),
		psaexchange.WithToken("TESTTOKEN"),
		psaexchange.WithBuyerCID("12345"),
	)
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestClient_FetchCatalog verifies the /api/catalog path and decodes the response.
func TestClient_FetchCatalog(t *testing.T) {
	fixture := mustReadFixture(t, "catalog_pokemon.json")

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	catalog, err := c.FetchCatalog(context.Background())
	if err != nil {
		t.Fatalf("FetchCatalog: %v", err)
	}
	if gotPath != "/api/catalog" {
		t.Errorf("expected path /api/catalog, got %s", gotPath)
	}
	if len(catalog.Cards) == 0 {
		t.Fatal("expected at least one card")
	}
	first := catalog.Cards[0]
	if first.Cert == "" {
		t.Error("Cards[0].Cert is empty")
	}
	if first.Category != "POKEMON CARDS" {
		t.Errorf("Cards[0].Category = %q, want POKEMON CARDS", first.Category)
	}
}

// TestClient_FetchCardLadder verifies /api/cardladder?cert=28660366 path + decode.
func TestClient_FetchCardLadder(t *testing.T) {
	fixture := mustReadFixture(t, "cardladder_28660366.json")

	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	cl, err := c.FetchCardLadder(context.Background(), "28660366")
	if err != nil {
		t.Fatalf("FetchCardLadder: %v", err)
	}
	if gotPath != "/api/cardladder" {
		t.Errorf("expected path /api/cardladder, got %s", gotPath)
	}
	if gotQuery != "cert=28660366" {
		t.Errorf("expected query cert=28660366, got %s", gotQuery)
	}
	if cl.EstimatedValue <= 0 {
		t.Errorf("EstimatedValue = %f, want > 0", cl.EstimatedValue)
	}
	if cl.Description == "" {
		t.Error("Description is empty")
	}
}

// TestClient_FetchCatalog_Non200 verifies that a 503 response returns an error.
func TestClient_FetchCatalog_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.FetchCatalog(context.Background())
	if err == nil {
		t.Fatal("expected error for 503, got nil")
	}
}

// TestClient_CategoryURL verifies the URL shape with token and category.
func TestClient_CategoryURL(t *testing.T) {
	cfg := httpx.DefaultConfig("psaexchange-test")
	httpClient := httpx.NewClient(cfg)
	c := psaexchange.NewClient(httpClient,
		psaexchange.WithBaseURL("https://psa-exchange-catalog.com"),
		psaexchange.WithToken("ABC"),
	)

	got := c.CategoryURL("POKEMON CARDS")
	want := "https://psa-exchange-catalog.com/catalog/ABC?cat=POKEMON+CARDS"
	if got != want {
		t.Errorf("CategoryURL = %q, want %q", got, want)
	}
}

// TestClient_CategoryURL_NoToken verifies empty string when no token configured.
func TestClient_CategoryURL_NoToken(t *testing.T) {
	cfg := httpx.DefaultConfig("psaexchange-test")
	httpClient := httpx.NewClient(cfg)
	c := psaexchange.NewClient(httpClient)

	got := c.CategoryURL("POKEMON CARDS")
	if got != "" {
		t.Errorf("CategoryURL with no token = %q, want empty string", got)
	}
}

// TestClient_Default_BaseURL verifies the default base URL prefix.
func TestClient_Default_BaseURL(t *testing.T) {
	cfg := httpx.DefaultConfig("psaexchange-test")
	httpClient := httpx.NewClient(cfg)
	c := psaexchange.NewClient(httpClient)

	got := c.BaseURL()
	if !strings.HasPrefix(got, "https://psa-exchange-catalog.com/") &&
		got != "https://psa-exchange-catalog.com" {
		t.Errorf("BaseURL = %q, want prefix https://psa-exchange-catalog.com/", got)
	}
}
