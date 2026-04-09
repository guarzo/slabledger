// Market Movers (Sports Card Investor) integration tests.
// Run with: go test ./internal/integration/ -tags integration -v -run TestMarketMovers -timeout 2m
//
//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	_ "github.com/joho/godotenv/autoload"
)

// newMMClient logs in with MM_EMAIL / MM_PASSWORD from .env and returns a ready client.
// Skips the test if credentials are not set.
func newMMClient(t *testing.T) *marketmovers.Client {
	t.Helper()
	email := os.Getenv("MM_EMAIL")
	password := os.Getenv("MM_PASSWORD")
	if email == "" || password == "" {
		t.Skip("MM_EMAIL and MM_PASSWORD required — add to .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	auth := marketmovers.NewAuth()
	resp, err := auth.Login(ctx, email, password)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	t.Logf("Logged in as %q (tier=%s)", resp.User.Email, resp.User.MembershipTier)

	expiry := marketmovers.ParseJWTExpiry(resp.AccessToken)
	t.Logf("Access token expires at %s", expiry.Format(time.RFC3339))

	c := marketmovers.NewClient(
		marketmovers.WithStaticToken(resp.AccessToken),
	)
	return c
}

// TestMarketMovers_Login verifies that auth.Login succeeds and returns valid tokens.
func TestMarketMovers_Login(t *testing.T) {
	email := os.Getenv("MM_EMAIL")
	password := os.Getenv("MM_PASSWORD")
	if email == "" || password == "" {
		t.Skip("MM_EMAIL and MM_PASSWORD required — add to .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	auth := marketmovers.NewAuth()
	resp, err := auth.Login(ctx, email, password)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("expected non-empty AccessToken")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty RefreshToken")
	}
	if resp.User.Email == "" {
		t.Error("expected non-empty User.Email")
	}

	expiry := marketmovers.ParseJWTExpiry(resp.AccessToken)
	if expiry.Before(time.Now()) {
		t.Errorf("access token already expired: expiry=%s", expiry)
	}

	t.Logf("Login OK: email=%s tier=%s token_expires=%s",
		resp.User.Email, resp.User.MembershipTier, expiry.Format(time.RFC3339))
}

// TestMarketMovers_TokenRefresh verifies that a refresh token can obtain a new access token.
func TestMarketMovers_TokenRefresh(t *testing.T) {
	email := os.Getenv("MM_EMAIL")
	password := os.Getenv("MM_PASSWORD")
	if email == "" || password == "" {
		t.Skip("MM_EMAIL and MM_PASSWORD required — add to .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	auth := marketmovers.NewAuth()
	loginResp, err := auth.Login(ctx, email, password)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	refreshResp, err := auth.RefreshToken(ctx, loginResp.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	if refreshResp.AccessToken == "" {
		t.Error("expected non-empty AccessToken from refresh")
	}

	expiry := marketmovers.ParseJWTExpiry(refreshResp.AccessToken)
	t.Logf("Refresh OK: new token expires=%s", expiry.Format(time.RFC3339))
}

// TestMarketMovers_SearchCollectibles verifies text search returns results for a known card.
func TestMarketMovers_SearchCollectibles(t *testing.T) {
	c := newMMClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.SearchCollectibles(ctx, "Charizard PSA 10", 0, 5)
	if err != nil {
		t.Fatalf("SearchCollectibles: %v", err)
	}

	t.Logf("SearchCollectibles('Charizard PSA 10'): %d results", len(resp.Items))
	for i, r := range resp.Items {
		if i >= 5 {
			break
		}
		t.Logf("  [%d] id=%d title=%q last30_avg=$%.2f last30_count=%d",
			i, r.Item.ID, r.Item.SearchTitle,
			r.Item.Stats.Last30.AvgPrice, r.Item.Stats.Last30.TotalSalesCount)
	}

	if len(resp.Items) == 0 {
		t.Error("expected at least one result for 'Charizard PSA 10'")
	}
}

// TestMarketMovers_FetchDailyStats verifies the dailyStatsV2 endpoint returns data
// for a well-known collectible ID obtained from a live search.
func TestMarketMovers_FetchDailyStats(t *testing.T) {
	c := newMMClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First get a real collectible ID via search
	searchResp, err := c.SearchCollectibles(ctx, "Charizard PSA 10", 0, 3)
	if err != nil {
		t.Fatalf("SearchCollectibles: %v", err)
	}
	if len(searchResp.Items) == 0 {
		t.Skip("no search results — cannot test FetchDailyStats")
	}

	id := searchResp.Items[0].Item.ID
	title := searchResp.Items[0].Item.SearchTitle
	dateFrom := time.Now().AddDate(0, 0, -30)

	resp, err := c.FetchDailyStats(ctx, id, dateFrom)
	if err != nil {
		t.Fatalf("FetchDailyStats(id=%d, from=%s): %v", id, dateFrom.Format("2006-01-02"), err)
	}

	t.Logf("FetchDailyStats(%q id=%d, last 30 days): %d days of data", title, id, len(resp.DailyStats))
	for i, day := range resp.DailyStats {
		if i >= 5 {
			t.Logf("  ... and %d more days", len(resp.DailyStats)-5)
			break
		}
		t.Logf("  %s: avg=$%.2f count=%d min=$%.2f max=$%.2f",
			day.FormattedDate, day.AverageSalePrice, day.TotalSalesCount,
			day.MinSalePrice, day.MaxSalePrice)
	}
}

// TestMarketMovers_AvgRecentPrice verifies the end-to-end weighted average calculation.
func TestMarketMovers_AvgRecentPrice(t *testing.T) {
	c := newMMClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	searchResp, err := c.SearchCollectibles(ctx, "Charizard PSA 10", 0, 3)
	if err != nil {
		t.Fatalf("SearchCollectibles: %v", err)
	}
	if len(searchResp.Items) == 0 {
		t.Skip("no search results — cannot test AvgRecentPrice")
	}

	id := searchResp.Items[0].Item.ID
	title := searchResp.Items[0].Item.SearchTitle

	avg, err := c.AvgRecentPrice(ctx, id, 30)
	if err != nil {
		t.Fatalf("AvgRecentPrice(id=%d): %v", id, err)
	}

	t.Logf("AvgRecentPrice(%q id=%d, 30d): $%.2f", title, id, avg)

	if avg < 0 {
		t.Errorf("expected non-negative avg price, got %.2f", avg)
	}
	// If there are sales data, avg should be > 0; just warn if not (card may have no recent sales)
	if avg == 0 {
		t.Logf("WARNING: avg=0 — card may have no sales in last 30 days")
	}
}

// TestMarketMovers_FetchCompletedSummaries verifies the completedSummaries endpoint.
func TestMarketMovers_FetchCompletedSummaries(t *testing.T) {
	c := newMMClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	searchResp, err := c.SearchCollectibles(ctx, "Charizard PSA 10", 0, 3)
	if err != nil {
		t.Fatalf("SearchCollectibles: %v", err)
	}
	if len(searchResp.Items) == 0 {
		t.Skip("no search results — cannot test FetchCompletedSummaries")
	}

	id := searchResp.Items[0].Item.ID
	title := searchResp.Items[0].Item.SearchTitle

	resp, err := c.FetchCompletedSummaries(ctx, id, 30)
	if err != nil {
		t.Fatalf("FetchCompletedSummaries(id=%d): %v", id, err)
	}

	t.Logf("FetchCompletedSummaries(%q id=%d): %d items (total=%d)",
		title, id, len(resp.Items), resp.TotalCount)
	for i, s := range resp.Items {
		if i >= 5 {
			t.Logf("  ... and %d more", len(resp.Items)-5)
			break
		}
		t.Logf("  %s: avg=$%.2f count=%d", s.FormattedDate, s.AverageSalePrice, s.TotalSalesCount)
	}
}

// TestMarketMovers_AddAndRemoveCollectionItem validates the full add→remove lifecycle.
// Searches for a known cheap card, adds it to the collection with purchase details,
// verifies the response, then immediately removes it to keep the collection clean.
func TestMarketMovers_AddAndRemoveCollectionItem(t *testing.T) {
	c := newMMClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Search for a cheap, well-known card to use as the test item.
	searchResp, err := c.SearchCollectibles(ctx, "Pikachu PSA 10", 0, 3)
	if err != nil {
		t.Fatalf("SearchCollectibles: %v", err)
	}
	if len(searchResp.Items) == 0 {
		t.Skip("no search results for 'Pikachu PSA 10' — cannot test AddCollectionItem")
	}

	target := searchResp.Items[0].Item
	t.Logf("Test target: id=%d title=%q", target.ID, target.SearchTitle)

	// 2. Add the item to the collection.
	input := marketmovers.AddCollectionItemInput{
		Collectible: marketmovers.CollectionCollectible{
			CollectibleType: "sports-card",
			CollectibleID:   target.ID,
		},
		PurchaseDetails: marketmovers.CollectionPurchaseDetails{
			Quantity:             1,
			PurchasePricePerItem: 0.01, // penny — clearly a test entry
			ConversionFeePerItem: 0,
			PurchaseDateISO:      "2026-01-01",
			Notes:                "integration-test-cleanup-me",
		},
		CategoryIDs: nil,
	}

	addResp, err := c.AddCollectionItem(ctx, input)
	if err != nil {
		t.Fatalf("AddCollectionItem: %v", err)
	}

	t.Logf("AddCollectionItem OK: success=%v collectibleId=%d collectionItemId=%d isCustom=%v",
		addResp.Success, addResp.CollectibleID, addResp.CollectionItemID, addResp.IsCustomCollectible)

	if !addResp.Success {
		t.Error("expected success=true from AddCollectionItem")
	}
	if addResp.CollectibleID != target.ID {
		t.Errorf("expected collectibleId=%d, got %d", target.ID, addResp.CollectibleID)
	}
	if addResp.CollectionItemID == 0 {
		t.Error("expected non-zero collectionItemId")
	}

	// 3. Clean up: remove the item so we don't pollute the collection.
	err = c.RemoveCollectionItem(ctx, addResp.CollectionItemID, "sports-card")
	if err != nil {
		t.Errorf("RemoveCollectionItem (cleanup): %v — item %d may remain in collection", err, addResp.CollectionItemID)
	} else {
		t.Logf("RemoveCollectionItem OK: cleaned up collectionItemId=%d", addResp.CollectionItemID)
	}
}
