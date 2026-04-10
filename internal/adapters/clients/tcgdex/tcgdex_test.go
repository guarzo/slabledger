package tcgdex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/storage"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newTestCache creates a file-based cache for testing.
func newTestCache(t *testing.T) cache.Cache {
	t.Helper()
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	c, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// newTestServer creates a test HTTP server that routes requests by path.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestLangToDisplay(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"en", "English"},
		{"ja", "Japanese"},
		{"fr", "French"},
		{"de", "German"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := langToDisplay(tt.lang)
		if got != tt.want {
			t.Errorf("langToDisplay(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestBuildURL(t *testing.T) {
	adapter := newTCGdexWithClient(nil, nil)
	got := adapter.buildURL("en", "sets/swsh1")
	want := "https://api.tcgdex.net/v2/en/sets/swsh1"
	if got != want {
		t.Errorf("buildURL = %q, want %q", got, want)
	}
}

func TestAvailable(t *testing.T) {
	adapter := newTCGdexWithClient(nil, mocks.NewMockHTTPClient())
	if !adapter.Available() {
		t.Error("TCGdex adapter should always be available")
	}
}

func TestGetCards_MapsCardStubs(t *testing.T) {
	setResp := tcgdexSetDetail{
		ID:   "swsh1",
		Name: "Sword & Shield",
		Serie: struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{ID: "swsh", Name: "Sword & Shield"},
		CardCount: struct {
			Total    int `json:"total"`
			Official int `json:"official"`
		}{Total: 3, Official: 3},
		Cards: []tcgdexCardStub{
			{ID: "swsh1-1", LocalID: "1", Name: "Celebi V", Image: "https://assets.tcgdex.net/en/swsh/swsh1/1"},
			{ID: "swsh1-2", LocalID: "2", Name: "Roselia", Image: "https://assets.tcgdex.net/en/swsh/swsh1/2"},
			{ID: "swsh1-3", LocalID: "3", Name: "Roselia", Image: ""},
		},
	}

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(setResp)
	})
	defer srv.Close()

	config := httpx.DefaultConfig("TCGdexTest")
	config.DefaultTimeout = 5 * time.Second
	client := httpx.NewClient(config)

	ctx := context.Background()
	adapter := NewTCGdexWithClientAndStorage(newTestCache(t), client, WithBaseURL(srv.URL))
	adapter.languages = []string{"en"}

	cards, err := adapter.GetCards(ctx, "swsh1")
	if err != nil {
		t.Fatalf("GetCards error: %v", err)
	}

	if len(cards) != 3 {
		t.Fatalf("expected 3 cards, got %d", len(cards))
	}

	// Verify first card
	c := cards[0]
	if c.ID != "swsh1-1" {
		t.Errorf("card ID = %q, want %q", c.ID, "swsh1-1")
	}
	if c.Name != "Celebi V" {
		t.Errorf("card Name = %q, want %q", c.Name, "Celebi V")
	}
	if c.Number != "1" {
		t.Errorf("card Number = %q, want %q", c.Number, "1")
	}
	if c.Set != "swsh1" {
		t.Errorf("card Set = %q, want %q", c.Set, "swsh1")
	}
	if c.Language != "English" {
		t.Errorf("card Language = %q, want %q", c.Language, "English")
	}
	if c.ImageURL != "https://assets.tcgdex.net/en/swsh/swsh1/1/low.webp" {
		t.Errorf("card ImageURL = %q, want suffix /low.webp", c.ImageURL)
	}

	// Verify empty image handling
	c3 := cards[2]
	if c3.ImageURL != "" {
		t.Errorf("card with empty Image should have empty ImageURL, got %q", c3.ImageURL)
	}
}

func TestSearchCardsFromCache(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore := storage.NewJSONFileStore()

	adapter := NewTCGdexWithClientAndStorage(
		newTestCache(t),
		mocks.NewMockHTTPClient(),
		WithFileStore(fileStore),
		WithCacheDir(tmpDir),
		WithEnablePersist(true),
		WithTCGdexLogger(observability.NewNoopLogger()),
	)

	ctx := context.Background()

	// Save some test cards to persistent storage
	testCards := []domainCards.Card{
		{ID: "swsh1-1", Name: "Celebi V", Number: "1", Set: "swsh1", SetName: "Sword & Shield"},
		{ID: "swsh1-25", Name: "Charizard V", Number: "25", Set: "swsh1", SetName: "Sword & Shield"},
		{ID: "swsh1-100", Name: "Charizard VMAX", Number: "100", Set: "swsh1", SetName: "Sword & Shield"},
	}
	testSet := domainCards.Set{ID: "swsh1", Name: "Sword & Shield", TotalCards: 3}
	if err := adapter.setStore.SaveSet(ctx, "swsh1", testSet, testCards); err != nil {
		t.Fatalf("failed to save test set: %v", err)
	}

	// Mark set as finalized in registry
	if err := adapter.registryMgr.MarkSetDiscovered(ctx, "swsh1", "Sword & Shield", "en", "2020-02-07", 3); err != nil {
		t.Fatalf("failed to mark set discovered: %v", err)
	}
	if err := adapter.registryMgr.MarkSetFinalized(ctx, "swsh1"); err != nil {
		t.Fatalf("failed to mark set finalized: %v", err)
	}

	// Search by card name
	results, total, err := adapter.searchCardsFromCache(ctx, domainCards.SearchCriteria{CardName: "Charizard", Limit: 10})
	if err != nil {
		t.Fatalf("searchCardsFromCache error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 matches for 'Charizard', got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Search by card number
	results, total, err = adapter.searchCardsFromCache(ctx, domainCards.SearchCriteria{CardNumber: "25", Limit: 10})
	if err != nil {
		t.Fatalf("searchCardsFromCache error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 match for number '25', got %d", total)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "Charizard V" {
		t.Errorf("expected Charizard V, got %s", results[0].Name)
	}

	// Search by set name
	results, total, err = adapter.searchCardsFromCache(ctx, domainCards.SearchCriteria{SetName: "sword", Limit: 10})
	if err != nil {
		t.Fatalf("searchCardsFromCache error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 matches for set 'sword', got %d", total)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results for set 'sword', got %d", len(results))
	}

	// Search with limit
	results, _, err = adapter.searchCardsFromCache(ctx, domainCards.SearchCriteria{Query: "Charizard", Limit: 1})
	if err != nil {
		t.Fatalf("searchCardsFromCache error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected limit to 1 result, got %d", len(results))
	}
}

func TestRegistryManager_DiscoverAndFinalize(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore := storage.NewJSONFileStore()
	mgr := NewSetRegistryManager(fileStore, tmpDir, observability.NewNoopLogger())
	ctx := context.Background()

	// Mark discovered
	err := mgr.MarkSetDiscovered(ctx, "swsh1", "Sword & Shield", "en", "2020-02-07", 216)
	if err != nil {
		t.Fatalf("MarkSetDiscovered: %v", err)
	}

	// Verify not finalized yet
	cached, err := mgr.isSetCached(ctx, "swsh1")
	if err != nil {
		t.Fatalf("isSetCached: %v", err)
	}
	if cached {
		t.Error("set should not be cached yet (only discovered)")
	}

	// Finalize
	err = mgr.MarkSetFinalized(ctx, "swsh1")
	if err != nil {
		t.Fatalf("MarkSetFinalized: %v", err)
	}

	// Now it should be cached
	cached, err = mgr.isSetCached(ctx, "swsh1")
	if err != nil {
		t.Fatalf("isSetCached: %v", err)
	}
	if !cached {
		t.Error("set should be cached after finalization")
	}

	// GetNewSetIDs should exclude finalized sets
	newIDs, err := mgr.GetNewSetIDs(ctx, []string{"swsh1", "swsh2"})
	if err != nil {
		t.Fatalf("GetNewSetIDs: %v", err)
	}
	if len(newIDs) != 1 || newIDs[0] != "swsh2" {
		t.Errorf("expected only swsh2 as new, got %v", newIDs)
	}
}

func TestSetStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore := storage.NewJSONFileStore()
	store := NewSetStore(fileStore, tmpDir)
	ctx := context.Background()

	testSet := domainCards.Set{ID: "base1", Name: "Base Set", TotalCards: 102}
	testCards := []domainCards.Card{
		{ID: "base1-1", Name: "Alakazam", Number: "1", Set: "base1"},
	}

	// Save
	err := store.SaveSet(ctx, "base1", testSet, testCards)
	if err != nil {
		t.Fatalf("SaveSet: %v", err)
	}

	// Exists
	if !store.SetExists("base1") {
		t.Error("set should exist after save")
	}

	// Load
	loaded, err := store.LoadSet(ctx, "base1")
	if err != nil {
		t.Fatalf("LoadSet: %v", err)
	}
	if loaded.Set.Name != "Base Set" {
		t.Errorf("loaded set name = %q, want %q", loaded.Set.Name, "Base Set")
	}
	if len(loaded.Cards) != 1 {
		t.Errorf("loaded %d cards, want 1", len(loaded.Cards))
	}

	// Delete
	err = store.DeleteSet(ctx, "base1")
	if err != nil {
		t.Fatalf("DeleteSet: %v", err)
	}
	if store.SetExists("base1") {
		t.Error("set should not exist after delete")
	}
}

func TestGetCacheStats(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore := storage.NewJSONFileStore()

	adapter := NewTCGdexWithClientAndStorage(
		nil,
		mocks.NewMockHTTPClient(),
		WithFileStore(fileStore),
		WithCacheDir(tmpDir),
		WithEnablePersist(true),
		WithTCGdexLogger(observability.NewNoopLogger()),
	)

	ctx := context.Background()

	// Initial stats — empty
	stats, err := adapter.GetCacheStats(ctx)
	if err != nil {
		t.Fatalf("GetCacheStats: %v", err)
	}
	if !stats.Enabled {
		t.Error("stats should be enabled")
	}
	if stats.TotalSets != 0 {
		t.Errorf("expected 0 total sets, got %d", stats.TotalSets)
	}

	// Add a discovered set
	err = adapter.registryMgr.MarkSetDiscovered(ctx, "swsh1", "Sword & Shield", "en", "2020-02-07", 216)
	if err != nil {
		t.Fatalf("MarkSetDiscovered: %v", err)
	}

	stats, err = adapter.GetCacheStats(ctx)
	if err != nil {
		t.Fatalf("GetCacheStats: %v", err)
	}
	if stats.TotalSets != 1 {
		t.Errorf("expected 1 total set, got %d", stats.TotalSets)
	}
	if stats.DiscoveredSets != 1 {
		t.Errorf("expected 1 discovered set, got %d", stats.DiscoveredSets)
	}
	if stats.FinalizedSets != 0 {
		t.Errorf("expected 0 finalized sets, got %d", stats.FinalizedSets)
	}
}

func TestGetCacheStats_Disabled(t *testing.T) {
	adapter := newTCGdexWithClient(nil, mocks.NewMockHTTPClient())

	stats, err := adapter.GetCacheStats(context.Background())
	if err != nil {
		t.Fatalf("GetCacheStats: %v", err)
	}
	if stats.Enabled {
		t.Error("stats should be disabled when persistence is off")
	}
}

func TestGetCards_EncodesSetIDWithPlus(t *testing.T) {
	var requestedRawPath string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// RawPath preserves percent-encoding; falls back to Path if empty
		requestedRawPath = r.URL.RawPath
		if requestedRawPath == "" {
			requestedRawPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "application/json")
		resp := tcgdexSetDetail{
			ID:   "SM1+",
			Name: "Sun & Moon Plus",
			Cards: []tcgdexCardStub{
				{ID: "SM1+-1", LocalID: "1", Name: "Caterpie"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	config := httpx.DefaultConfig("TCGdexTest")
	config.DefaultTimeout = 5 * time.Second
	client := httpx.NewClient(config)

	adapter := NewTCGdexWithClientAndStorage(newTestCache(t), client, WithBaseURL(srv.URL))
	adapter.languages = []string{"en"}

	cards, err := adapter.GetCards(context.Background(), "SM1+")
	if err != nil {
		t.Fatalf("GetCards: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}

	// Verify the "+" in the set ID was percent-encoded as "%2B" in the URL path
	if requestedRawPath != "/en/sets/SM1%2B" {
		t.Errorf("expected raw path /en/sets/SM1%%2B, got %s", requestedRawPath)
	}
}

func TestSearchCards_ValidationError(t *testing.T) {
	adapter := newTCGdexWithClient(nil, mocks.NewMockHTTPClient())

	_, _, err := adapter.SearchCards(context.Background(), domainCards.SearchCriteria{})
	if err == nil {
		t.Error("expected validation error for empty criteria")
	}
}

func TestRateLimiter_BurstAllowed(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := tcgdexSetDetail{
			ID:   "swsh1",
			Name: "Sword & Shield",
			Cards: []tcgdexCardStub{
				{ID: "swsh1-1", LocalID: "1", Name: "Celebi V"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	config := httpx.DefaultConfig("TCGdexTest")
	config.DefaultTimeout = 5 * time.Second
	client := httpx.NewClient(config)

	adapter := NewTCGdexWithClientAndStorage(newTestCache(t), client, WithBaseURL(srv.URL))
	adapter.languages = []string{"en"}

	ctx := context.Background()

	// Two rapid calls should succeed within burst capacity (burst=2)
	for i := 0; i < 2; i++ {
		cards, err := adapter.GetCards(ctx, "swsh1")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if len(cards) != 1 {
			t.Fatalf("call %d: expected 1 card, got %d", i, len(cards))
		}
	}
}

func TestRateLimiter_CancelledContext(t *testing.T) {
	adapter := newTCGdexWithClient(newTestCache(t), mocks.NewMockHTTPClient())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := adapter.get(ctx, "http://example.com/test", &struct{}{})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}
