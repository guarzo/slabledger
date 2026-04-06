// Package integration contains integration tests that hit real APIs.
// Run with: go test ./internal/integration/ -tags integration -v -timeout 5m
//
//go:build integration

package integration

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/cache"

	_ "github.com/joho/godotenv/autoload"
)

// inventoryCard represents a card currently in our inventory.
type inventoryCard struct {
	CardName   string
	CardNumber string
	SetName    string
	Grade      float64
	BuyUSD     float64
	CertNumber string

	// Expected results — what PriceCharting SHOULD match
	ExpectedProduct string  // PriceCharting product name substring
	ExpectedConsole string  // PriceCharting console name substring
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

func TestPriceChartingLookup(t *testing.T) {
	token := os.Getenv("PRICECHARTING_TOKEN")
	if token == "" {
		t.Skip("PRICECHARTING_TOKEN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := observability.NewNoopLogger()
	appCache := newTestCache(t)

	cfg := pricecharting.DefaultConfig(token)
	cfg.RateLimitInterval = 3 * time.Second // be gentle with API
	pc, err := pricecharting.NewPriceCharting(cfg, appCache, logger)
	if err != nil {
		t.Fatalf("failed to create PriceCharting client: %v", err)
	}
	defer pc.Close()

	for _, card := range currentInventory() {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			c := domainCards.Card{
				Name:    card.CardName,
				Number:  card.CardNumber,
				SetName: card.SetName,
			}

			price, err := pc.LookupCard(ctx, card.SetName, c)
			if err != nil {
				t.Errorf("LookupCard failed: %v", err)
				return
			}
			if price == nil {
				t.Errorf("LookupCard returned nil (no match)")
				return
			}

			// Check product name contains expected substring
			if card.ExpectedProduct != "" {
				if !containsCI(price.ProductName, card.ExpectedProduct) {
					t.Errorf("WRONG CARD: product=%q does not contain %q", price.ProductName, card.ExpectedProduct)
				}
			}

			// Check we have graded price data
			hasGrade := price.Grades.PSA10Cents > 0 || price.Grades.PSA9Cents > 0 || price.Grades.PSA8Cents > 0
			if !hasGrade {
				t.Errorf("no graded prices: PSA10=%d PSA9=%d PSA8=%d", price.Grades.PSA10Cents, price.Grades.PSA9Cents, price.Grades.PSA8Cents)
			}

			// Check price is in reasonable range
			priceCents, _ := testGradePrice(price.Grades, card.Grade)
			priceUSD := float64(priceCents) / 100.0

			t.Logf("  Product: %s", price.ProductName)
			t.Logf("  PSA10=$%.2f PSA9=$%.2f PSA8=$%.2f Raw=$%.2f",
				float64(price.Grades.PSA10Cents)/100, float64(price.Grades.PSA9Cents)/100,
				float64(price.Grades.PSA8Cents)/100, float64(price.Grades.RawCents)/100)

			if priceUSD > 0 && card.MinPriceUSD > 0 {
				if priceUSD < card.MinPriceUSD || priceUSD > card.MaxPriceUSD {
					t.Errorf("PRICE OUT OF RANGE: $%.2f not in [$%.2f, $%.2f] for grade %.0f (buy=$%.2f)",
						priceUSD, card.MinPriceUSD, card.MaxPriceUSD, card.Grade, card.BuyUSD)
				}
			}
		})
	}
}

// testGradePrice mirrors the production gradePrice logic from pricelookup/adapter.go.
// For half-grades, interpolates between the floor and ceiling grade prices.
func testGradePrice(grades pricing.GradedPrices, grade float64) (int64, string) {
	floor := math.Floor(grade)
	ceil := math.Ceil(grade)
	isHalf := math.Abs(grade-floor-0.5) < 1e-9

	wholePrice := func(g float64) int64 {
		switch g {
		case 10:
			return grades.PSA10Cents
		case 9:
			return grades.PSA9Cents
		case 8:
			return grades.PSA8Cents
		default:
			return grades.RawCents
		}
	}

	label := fmt.Sprintf("PSA%.0f", grade)
	if isHalf {
		label = fmt.Sprintf("PSA%.1f", grade)
	}

	if !isHalf {
		return wholePrice(grade), label
	}

	floorP := wholePrice(floor)
	ceilP := wholePrice(ceil)
	if floorP > 0 && ceilP > 0 {
		return (floorP + ceilP) / 2, label
	}
	if floorP > 0 {
		return floorP, label
	}
	return ceilP, label
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
