package cardladder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_FetchCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("index") != "collectioncards" {
			t.Fatalf("unexpected index: %s", r.URL.Query().Get("index"))
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Fatalf("unexpected auth: %s", auth)
		}
		json.NewEncoder(w).Encode(SearchResponse[CollectionCard]{ //nolint:errcheck
			Hits: []CollectionCard{
				{CollectionCardID: "card1", Player: "Charizard", CurrentValue: 500, Image: "https://cdn/cert/123456/img.jpg"},
				{CollectionCardID: "card2", Player: "Pikachu", CurrentValue: 100, Image: "https://cdn/cert/789012/img.jpg"},
			},
			TotalHits: 2,
		})
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL+"/search"),
		WithStaticToken("test-token"),
	)
	cards, err := client.FetchCollectionPage(context.Background(), "coll-123", 0, 100)
	if err != nil {
		t.Fatalf("FetchCollectionPage failed: %v", err)
	}
	if len(cards.Hits) != 2 {
		t.Errorf("got %d hits, want 2", len(cards.Hits))
	}
	if cards.Hits[0].Player != "Charizard" {
		t.Errorf("first card player = %q, want %q", cards.Hits[0].Player, "Charizard")
	}
}

func TestClient_FetchSalesComps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("index") != "salesarchive" {
			t.Fatalf("unexpected index: %s", r.URL.Query().Get("index"))
		}
		json.NewEncoder(w).Encode(SearchResponse[SaleComp]{ //nolint:errcheck
			Hits: []SaleComp{
				{ItemID: "ebay-123", Price: 135, Platform: "eBay", ListingType: "Auction"},
			},
			TotalHits: 1,
		})
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL+"/search"),
		WithStaticToken("test-token"),
	)
	comps, err := client.FetchSalesComps(context.Background(), "gemrate-abc", "g9", "psa", 0, 100)
	if err != nil {
		t.Fatalf("FetchSalesComps failed: %v", err)
	}
	if len(comps.Hits) != 1 {
		t.Errorf("got %d hits, want 1", len(comps.Hits))
	}
	if comps.Hits[0].Platform != "eBay" {
		t.Errorf("platform = %q, want %q", comps.Hits[0].Platform, "eBay")
	}
}

func TestClient_TokenRefreshOnExpiry(t *testing.T) {
	callCount := 0
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(FirebaseRefreshResponse{ //nolint:errcheck
			IDToken:      "refreshed-token",
			RefreshToken: "new-refresh",
			ExpiresIn:    "3600",
		})
	}))
	defer authServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer refreshed-token" {
			t.Fatalf("expected refreshed token, got: %s", auth)
		}
		json.NewEncoder(w).Encode(SearchResponse[CollectionCard]{TotalHits: 0}) //nolint:errcheck
	}))
	defer searchServer.Close()

	auth := NewFirebaseAuth("test-key", WithTokenBaseURL(authServer.URL))
	client := NewClient(
		WithBaseURL(searchServer.URL+"/search"),
		WithTokenManager(auth, "old-refresh-token", time.Now().Add(-1*time.Hour)),
	)
	_, err := client.FetchCollectionPage(context.Background(), "coll-123", 0, 100)
	if err != nil {
		t.Fatalf("FetchCollectionPage failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 refresh call, got %d", callCount)
	}
}

func TestClient_ConcurrentTokenRefresh(t *testing.T) {
	var refreshCalls atomic.Int64
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalls.Add(1)
		json.NewEncoder(w).Encode(FirebaseRefreshResponse{ //nolint:errcheck
			IDToken:      "refreshed-token",
			RefreshToken: "new-refresh",
			ExpiresIn:    "3600",
		})
	}))
	defer authServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SearchResponse[CollectionCard]{TotalHits: 0}) //nolint:errcheck
	}))
	defer searchServer.Close()

	auth := NewFirebaseAuth("test-key", WithTokenBaseURL(authServer.URL))
	client := NewClient(
		WithBaseURL(searchServer.URL+"/search"),
		WithTokenManager(auth, "old-refresh", time.Now().Add(-1*time.Hour)),
	)

	// Launch 5 concurrent requests that all need a token refresh
	const goroutines = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = client.FetchCollectionPage(context.Background(), "coll-123", 0, 100)
		}()
	}
	wg.Wait()

	calls := refreshCalls.Load()
	if calls > int64(goroutines) {
		t.Errorf("expected at most %d refresh calls, got %d", goroutines, calls)
	}
}
