// Integration test that uses the actual PSA CSV to validate the full
// import → metadata parsing → pricing pipeline.
//
// Run with: go test ./internal/integration/ -tags integration -run TestImportToPricing -v -timeout 5m
//
//go:build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"

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

		cardName, cardNumber, setName := inventory.ExportParseCardMetadataFromTitle(row.ListingTitle, row.Category)

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
			if exp.setNotGeneric && inventory.ExportIsGenericSetName(setName) {
				t.Errorf("set name %q is generic, should be specific", setName)
				failed++
				return
			}
			passed++
		})
	}

	fmt.Printf("\n=== PARSING: %d/%d passed ===\n", passed, passed+failed)
}

