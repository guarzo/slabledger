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

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/adapters/clients/fusionprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/pokemonprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
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

func TestCardValidation(t *testing.T) {
	logger := observability.NewNoopLogger()
	appCache := newTestCache(t)
	cardProv := tcgdex.NewTCGdex(appCache, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, card := range currentInventory() {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			cleanName := cardutil.NormalizeCardName(card.CardName)
			cleanName = cardutil.StripVariantSuffix(cleanName)

			// Test ResolveCardIdentity
			canonical := fusionprice.ResolveCardIdentity(ctx, cardProv, cleanName, card.CardNumber, card.SetName)
			if canonical != nil {
				t.Logf("  Canonical: %s #%s [%s]", canonical.Name, canonical.Number, canonical.SetName)
				// Verify canonical card name is reasonable
				if card.ExpectedProduct != "" && !containsCI(canonical.Name, card.ExpectedProduct) {
					t.Errorf("WRONG CANONICAL: %q does not contain %q", canonical.Name, card.ExpectedProduct)
				}
			} else {
				t.Logf("  Canonical: nil (no match in TCGdex — expected for Japanese/promo cards)")
			}

			// Test ValidateCardResolution - simulates what happens when PriceCharting returns a product
			validation := fusionprice.ValidateCardResolution(ctx, cardProv, card.CardName, card.CardNumber, card.SetName)
			t.Logf("  Validation: valid=%v reason=%q", validation.Valid, validation.Reason)

			// Validation should NOT reject cards that have no match (absence != invalid)
			if !validation.Valid {
				t.Errorf("VALIDATION REJECTED: reason=%q — this prevents pricing from working", validation.Reason)
			}
		})
	}
}

func TestPokemonPriceLookup(t *testing.T) {
	apiKey := os.Getenv("POKEMONPRICE_TRACKER_API_KEY")
	if apiKey == "" {
		t.Skip("POKEMONPRICE_TRACKER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := pokemonprice.NewClient(apiKey)

	for _, card := range currentInventory() {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			cleanName := cardutil.NormalizePurchaseName(card.CardName)
			cleanName = cardutil.StripVariantSuffix(cleanName)
			normalizedSet := cardutil.NormalizeSetNameSimple(card.SetName)

			data, statusCode, _, err := client.GetPriceWithGraded(ctx, card.SetName, card.CardName, card.CardNumber)
			if err != nil {
				t.Logf("  PokemonPrice: no result (status=%d, err=%v)", statusCode, err)
				t.Logf("  Search: name=%q set=%q number=%q", cleanName, normalizedSet, card.CardNumber)
				return // Many Japanese cards won't be found — that's OK
			}

			t.Logf("  PokemonPrice: %s #%s [%s] market=$%.2f",
				data.Name, data.CardNumber, data.SetName, data.Prices.Market)

			// Verify it found the right card
			if card.ExpectedProduct != "" && !containsCI(data.Name, card.ExpectedProduct) {
				t.Errorf("WRONG CARD: %q does not contain %q", data.Name, card.ExpectedProduct)
			}

			// Rate limit
			time.Sleep(1100 * time.Millisecond)
		})
	}
}

func TestFullLookupPipeline(t *testing.T) {
	token := os.Getenv("PRICECHARTING_TOKEN")
	if token == "" {
		t.Skip("PRICECHARTING_TOKEN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := observability.NewNoopLogger()
	appCache := newTestCache(t)

	// Create PriceCharting provider
	pcCfg := pricecharting.DefaultConfig(token)
	pcCfg.RateLimitInterval = 3 * time.Second
	pc, err := pricecharting.NewPriceCharting(pcCfg, appCache, logger)
	if err != nil {
		t.Fatalf("failed to create PriceCharting: %v", err)
	}
	defer pc.Close()

	// Create TCGdex provider
	cardProv := tcgdex.NewTCGdex(appCache, logger)

	// Create FusionPriceProvider (without PokemonPrice — simulates rate-limited scenario)
	fusionProv := fusionprice.NewFusionProviderWithRepo(
		pc,
		nil, // no secondary sources (PokemonPrice rate limited)
		appCache,
		nil, // no price repo
		nil, // no API tracker
		nil, // no access tracker
		logger,
		0, 0, 0, 0,
		fusionprice.WithCardProvider(cardProv),
	)

	passed := 0
	failed := 0
	noPrice := 0

	for _, card := range currentInventory() {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			c := domainCards.Card{
				Name:    card.CardName,
				Number:  card.CardNumber,
				SetName: card.SetName,
			}

			price, err := fusionProv.LookupCard(ctx, card.SetName, c)
			if err != nil {
				t.Errorf("LookupCard FAILED: %v", err)

				// Try direct PriceCharting to see if the issue is validation/fusion
				directPrice, directErr := pc.LookupCard(ctx, card.SetName, c)
				if directErr != nil {
					t.Logf("  Direct PC also failed: %v", directErr)
				} else if directPrice != nil {
					t.Logf("  Direct PC works: product=%s PSA10=$%.2f — PIPELINE IS BLOCKING IT",
						directPrice.ProductName, float64(directPrice.Grades.PSA10Cents)/100)
				}

				failed++
				return
			}
			if price == nil {
				t.Errorf("LookupCard returned nil — NO PRICE for $%.2f card", card.BuyUSD)

				// Try direct PriceCharting
				directPrice, directErr := pc.LookupCard(ctx, card.SetName, c)
				if directErr != nil {
					t.Logf("  Direct PC also failed: %v", directErr)
				} else if directPrice != nil {
					t.Logf("  Direct PC works: product=%s PSA10=$%.2f — PIPELINE IS BLOCKING IT",
						directPrice.ProductName, float64(directPrice.Grades.PSA10Cents)/100)
				}

				noPrice++
				return
			}

			// Get grade-appropriate price
			priceCents, gradeLabel := testGradePrice(price.Grades, card.Grade)
			priceUSD := float64(priceCents) / 100.0

			t.Logf("  Product: %s | %s=$%.2f | Sources: %v | Confidence: %.2f",
				price.ProductName, gradeLabel, priceUSD, price.Sources, price.Confidence)

			if priceCents == 0 {
				t.Errorf("zero price for grade %s (buy=$%.2f)", gradeLabel, card.BuyUSD)
				noPrice++
				return
			}

			// Check price sanity against buy price
			ratio := priceUSD / card.BuyUSD
			if ratio < 0.1 || ratio > 5.0 {
				t.Errorf("PRICE ANOMALY: %s=$%.2f vs buy=$%.2f (ratio=%.2f)", gradeLabel, priceUSD, card.BuyUSD, ratio)
				failed++
				return
			}

			passed++
			t.Logf("  PASS: %s=$%.2f (buy=$%.2f, ratio=%.2f)", gradeLabel, priceUSD, card.BuyUSD, ratio)
		})
	}

	fmt.Printf("\n=== PRICING SUMMARY ===\n")
	fmt.Printf("Passed: %d/%d\n", passed, len(currentInventory()))
	fmt.Printf("Failed: %d\n", failed)
	fmt.Printf("No price: %d\n", noPrice)
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
