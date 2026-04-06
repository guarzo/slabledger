// Package integration contains integration tests that hit real APIs.
// Run with: go test ./internal/integration/ -tags integration -v -timeout 5m
//
//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/platform/cache"
)

// inventoryCard represents a card currently in our inventory.
type inventoryCard struct {
	CardName   string
	CardNumber string
	SetName    string
	Grade      float64
	BuyUSD     float64
	CertNumber string

	// Expected results — what the pricing source SHOULD match
	ExpectedProduct string  // Product name substring
	ExpectedConsole string  // Console name substring
	MinPriceUSD     float64 // minimum reasonable PSA grade price
	MaxPriceUSD     float64 // maximum reasonable PSA grade price
}

// currentInventory returns the actual cards in our inventory.
// Delegates to pricedInventory() in testdata_test.go for unified test data.
func currentInventory() []inventoryCard {
	entries := pricedInventory()
	cards := make([]inventoryCard, len(entries))
	for i, e := range entries {
		cards[i] = e.toInventoryCard()
	}
	return cards
}

func newTestCache(t *testing.T) cache.Cache {
	t.Helper()
	dir := t.TempDir()
	c, err := cache.NewFileCacheBackend(dir+"/test.cache", cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("failed to create test cache: %v", err)
	}
	return c
}

func containsCI(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
