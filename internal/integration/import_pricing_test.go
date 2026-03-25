// Integration test that uses the actual PSA CSV to validate the full
// import → metadata parsing → pricing pipeline.
//
// Run with: go test ./internal/integration/ -tags integration -run TestImportToPricing -v -timeout 5m
//
//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/fusionprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"

	_ "github.com/joho/godotenv/autoload"
)

// psaRow represents one row from the actual PSA CSV
type psaRow struct {
	CertNumber   string
	ListingTitle string
	Category     string
	Grade        float64
	PricePaid    float64
}

// actualPSARows returns the actual rows from the inventory.
// Delegates to fullInventory() in testdata_test.go for unified test data.
func actualPSARows() []psaRow {
	entries := fullInventory()
	rows := make([]psaRow, len(entries))
	for i, e := range entries {
		rows[i] = e.toPSARow()
	}
	return rows
}

// TestParseCardMetadataFromCSV validates that parseCardMetadataFromTitle
// correctly extracts card name, number, and set name from every PSA listing title.
func TestParseCardMetadataFromCSV(t *testing.T) {
	type expected struct {
		cardNameContains string
		cardNumber       string
		setNotGeneric    bool // set name must NOT be generic
	}

	expectations := map[string]expected{
		"145455452": {"SNORLAX", "008", true},
		"133478793": {"RAYQUAZA", "126", true},
		"143473336": {"CHARIZARD", "006", true},
		"145076879": {"GYARADOS", "8", true},
		"150154256": {"PIKACHU", "260", true},
		"149955139": {"SYLVEON", "156", true},
		"143210177": {"DRAGONITE", "069", true},
		"150154255": {"PIKACHU", "260", true},
		"114811268": {"ANCIENT MEW", "", true}, // no card number, but set is "GAME MOVIE" (non-generic)
		"145076888": {"MEW", "151", true},
		"122699162": {"UMBREON", "031", true},
		"144122685": {"GENGAR", "094", true},
		"150154262": {"SNORLAX", "076", true},
		"145076863": {"BLASTOISE", "2", true},
		"145076878": {"ANCIENT MEW", "", true}, // no card number, but set is "GAME MOVIE" (non-generic)
		"150154260": {"SNORLAX", "076", true},
		"143518127": {"PIKACHU", "002", true},
		"145396462": {"PIKACHU", "24", true},
		"145084327": {"CHARIZARD", "161", true},
		"135767318": {"MEWTWO", "56", true},
		"141627783": {"UMBREON", "176", true},
		// --- New cards ---
		"139414865": {"GARDEVOIR", "087", true},
		"135021722": {"PIKACHU", "SM162", true},
		"134110532": {"PIKACHU", "SM162", true},
		"134110528": {"PIKACHU", "SM162", true},
		"130221147": {"PIKACHU", "09", true},
		"72973327":  {"CHARIZARD", "075", true},
		"123238115": {"UMBREON", "15", true},
		"134093774": {"RAYQUAZA", "029", true},
		"113751496": {"EEVEE", "167", true},
		"132537172": {"PIKACHU", "236", true},
		"137150557": {"MEWTWO", "", true},      // Pokken Tournament promo, no number but set is "PROMO POKKEN TOURNAMENT" (non-generic)
		"121986129": {"MIMIKYU", "087", true},
		"118527750": {"ANCIENT MEW", "", true}, // no card number, but set is "GAME MOVIE" (non-generic, duplicate)
	}

	passed, failed := 0, 0
	for _, row := range actualPSARows() {
		exp, ok := expectations[row.CertNumber]
		if !ok {
			continue
		}

		cardName, cardNumber, setName := campaigns.ExportParseCardMetadataFromTitle(row.ListingTitle, row.Category)

		t.Run(row.CertNumber, func(t *testing.T) {
			t.Logf("Title: %s", row.ListingTitle)
			t.Logf("  → name=%q num=%q set=%q", cardName, cardNumber, setName)

			if !containsCI(cardName, exp.cardNameContains) {
				t.Errorf("card name %q doesn't contain %q", cardName, exp.cardNameContains)
				failed++
				return
			}
			if cardNumber != exp.cardNumber {
				t.Errorf("card number = %q, want %q", cardNumber, exp.cardNumber)
				failed++
				return
			}
			if exp.setNotGeneric && campaigns.ExportIsGenericSetName(setName) {
				t.Errorf("set name %q is generic, should be specific", setName)
				failed++
				return
			}
			passed++
		})
	}

	fmt.Printf("\n=== PARSING: %d/%d passed ===\n", passed, passed+failed)
}

// runImportPricingCheck is the shared import → parse → price lookup → sanity check loop
// used by both TestImportToPricing and TestImportToFullPricing.
func runImportPricingCheck(ctx context.Context, t *testing.T, fusionProv interface {
	LookupCard(context.Context, string, domainCards.Card) (*pricing.Price, error)
}, entries []InventoryEntry) (passed, failed, noPrice, skipped int) {
	t.Helper()

	for _, entry := range entries {
		cardName, cardNumber, setName := campaigns.ExportParseCardMetadataFromTitle(entry.ListingTitle, entry.Category)

		t.Run(entry.CertNumber+"_"+cardName, func(t *testing.T) {
			t.Logf("Title: %s", entry.ListingTitle)
			t.Logf("Parsed: name=%q num=%q set=%q", cardName, cardNumber, setName)

			if entry.SkipPricing {
				t.Logf("  SKIP: card marked as SkipPricing (not indexed by pricing sources)")
				skipped++
				return
			}

			if entry.ExpectedProduct != "" && !containsCI(cardName, entry.ExpectedProduct) {
				t.Logf("  NOTE: parsed name %q doesn't contain %q", cardName, entry.ExpectedProduct)
			}

			c := domainCards.Card{
				Name:    cardName,
				Number:  cardNumber,
				SetName: setName,
			}

			price, err := fusionProv.LookupCard(ctx, setName, c)
			if err != nil {
				t.Errorf("LookupCard FAILED: %v", err)
				failed++
				return
			}
			if price == nil {
				t.Errorf("NO PRICE for $%.2f card", entry.PricePaid)
				noPrice++
				return
			}

			priceCents, gradeLabel := testGradePrice(price.Grades, entry.Grade)
			priceUSD := float64(priceCents) / 100.0

			t.Logf("Result: product=%q %s=$%.2f (buy=$%.2f) sources=%v",
				price.ProductName, gradeLabel, priceUSD, entry.PricePaid, price.Sources)

			if priceCents == 0 {
				if price.Grades.PSA9Cents > 0 || price.Grades.PSA10Cents > 0 {
					t.Logf("  %s=$0 but higher grade available — grade fallback will handle", gradeLabel)
					passed++
					return
				}
				t.Errorf("zero price for %s with no fallback grades", gradeLabel)
				noPrice++
				return
			}

			ratio := priceUSD / entry.PricePaid
			if ratio < 0.1 || ratio > 5.0 {
				t.Errorf("PRICE ANOMALY: %s=$%.2f vs buy=$%.2f (ratio=%.2f)", gradeLabel, priceUSD, entry.PricePaid, ratio)
				failed++
				return
			}

			passed++
		})
	}
	return
}

// TestImportToPricing validates the complete flow: parse CSV → lookup price → get reasonable result
func TestImportToPricing(t *testing.T) {
	token := os.Getenv("PRICECHARTING_TOKEN")
	if token == "" {
		t.Skip("PRICECHARTING_TOKEN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := observability.NewNoopLogger()
	appCache := newTestCache(t)

	pcCfg := pricecharting.DefaultConfig(token)
	pcCfg.RateLimitInterval = 3 * time.Second
	pc, err := pricecharting.NewPriceCharting(pcCfg, appCache, logger)
	if err != nil {
		t.Fatalf("PriceCharting init: %v", err)
	}
	defer pc.Close()

	cardProv := tcgdex.NewTCGdex(appCache, logger)

	fusionProv := fusionprice.NewFusionProviderWithRepo(
		pc, nil, appCache, nil, nil, nil, logger,
		0, 0, 0, 0,
		fusionprice.WithCardProvider(cardProv),
	)

	entries := fullInventory()
	passed, failed, noPrice, skipped := runImportPricingCheck(ctx, t, fusionProv, entries)

	fmt.Printf("\n=== END-TO-END: Passed=%d Failed=%d NoPrice=%d Skipped=%d Total=%d ===\n",
		passed, failed, noPrice, skipped, len(entries))
}
