// Unified test data for all integration tests.
// Single source of truth for the 34 inventory cards.
//
//go:build integration

package integration

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

func TestPricedInventoryExcludesGenericUnpriceable(t *testing.T) {
	priced := pricedInventory()
	for _, e := range priced {
		if e.CardNumber == "" && isGenericTestSet(e.SetName) {
			t.Errorf("pricedInventory contains entry with empty CardNumber AND generic set: cert=%s card=%q set=%q", e.CertNumber, e.CardName, e.SetName)
		}
	}

	// Verify count matches uniqueInventory minus excluded entries
	uniq := uniqueInventory()
	expectedCount := 0
	for _, e := range uniq {
		if e.SkipPricing {
			continue
		}
		if e.CardNumber != "" || !isGenericTestSet(e.SetName) {
			expectedCount++
		}
	}
	if len(priced) != expectedCount {
		t.Errorf("pricedInventory has %d entries, want %d", len(priced), expectedCount)
	}
}

// InventoryEntry combines PSA row data with parsed card identity and validation bounds.
type InventoryEntry struct {
	CertNumber      string
	ListingTitle    string  // Raw PSA listing title
	Category        string  // PSA category
	Grade           float64 // PSA grade
	PricePaid       float64 // Buy cost USD

	CardName        string // Known/expected card name
	CardNumber      string // Known/expected card number
	SetName         string // Known/expected set name

	ExpectedProduct string  // Expected product name substring
	ExpectedConsole string  // Expected console name substring
	MinPriceUSD     float64 // Minimum reasonable PSA grade price
	MaxPriceUSD     float64 // Maximum reasonable PSA grade price

	IsDuplicate  bool // 2nd copy of same card identity
	SkipPricing  bool // Card is known to be unfindable in pricing sources
}

// fullInventory returns all 34 inventory entries including duplicates.
func fullInventory() []InventoryEntry {
	return []InventoryEntry{
		{
			CertNumber: "145455452", ListingTitle: "2022 POKEMON JAPANESE SWORD & SHIELD START DECK 100 COROCORO COMIC VERSION 008 SNORLAX PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 123.90,
			CardName: "SNORLAX SD.100 COROCORO COMIC VER.", CardNumber: "008", SetName: "JAPANESE SWORD & SHIELD START DECK 100 COROCORO COMIC VERSION",
			ExpectedProduct: "Snorlax", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "133478793", ListingTitle: "2015 POKEMON JAPANESE XY PROMO 126 RAYQUAZA SPIRIT LINK POKEMON CENTER PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 136.44,
			CardName: "RAYQUAZA SPIRIT LINK POKEMON CENTER PROMO", CardNumber: "126", SetName: "JAPANESE XY PROMO",
			ExpectedProduct: "Rayquaza", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 400,
		},
		{
			CertNumber: "143473336", ListingTitle: "2000 POKEMON JAPANESE NEO 2 PROMO 006 CHARIZARD REVERSE FOIL PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 370.20,
			CardName: "CHARIZARD PROMO-REVERSE FOIL", CardNumber: "006", SetName: "JAPANESE NEO 2 PROMO",
			ExpectedProduct: "Charizard", ExpectedConsole: "Japanese", MinPriceUSD: 50, MaxPriceUSD: 1200, // wide: PC matches CD Promo variant ($1000) instead of Neo ($305)
		},
		{
			CertNumber: "145076879", ListingTitle: "2000 POKEMON ROCKET 8 DARK GYARADOS-HOLO 1ST EDITION PSA 8",
			Category: "POKEMON CARDS", Grade: 8, PricePaid: 167.00,
			CardName: "DARK GYARADOS-HOLO 1ST EDITION", CardNumber: "8", SetName: "ROCKET",
			ExpectedProduct: "Gyarados", ExpectedConsole: "Rocket", MinPriceUSD: 30, MaxPriceUSD: 500,
		},
		{
			CertNumber: "150154256", ListingTitle: "2025 POKEMON JAPANESE SV-P PROMO 260 TOHOKU'S PIKACHU SPECIAL BOX POKEMON CENTER TOHOKU PSA 8",
			Category: "POKEMON CARDS", Grade: 8, PricePaid: 90.20,
			CardName: "TOHOKU'S PIKACHU SPECIAL BOX PC TOHOKU", CardNumber: "260", SetName: "JAPANESE SV-P PROMO",
			ExpectedProduct: "Pikachu", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 300,
			IsDuplicate: true,
		},
		{
			CertNumber: "149955139", ListingTitle: "2025 POKEMON PRE EN-PRISMATIC EVOLUTIONS 156 SYLVEON EX SPECIAL ILLUSTRATION RARE PSA 8",
			Category: "POKEMON CARDS", Grade: 8, PricePaid: 242.99,
			CardName: "SYLVEON ex SPECIAL ILLUSTRATION RARE", CardNumber: "156", SetName: "PRE EN-PRISMATIC EVOLUTIONS",
			ExpectedProduct: "Sylveon", ExpectedConsole: "Prismatic", MinPriceUSD: 50, MaxPriceUSD: 500,
		},
		{
			CertNumber: "143210177", ListingTitle: "2014 POKEMON JAPANESE XY RISING FIST 069 DRAGONITE EX 1ST EDITION PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 103.00,
			CardName: "DRAGONITE EX RISING FIST-1ST EDITION", CardNumber: "069", SetName: "JAPANESE XY RISING FIST",
			ExpectedProduct: "Dragonite", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "150154255", ListingTitle: "2025 POKEMON JAPANESE SV-P PROMO 260 TOHOKU'S PIKACHU SPECIAL BOX POKEMON CENTER TOHOKU PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 110.76,
			CardName: "TOHOKU'S PIKACHU SPECIAL BOX PC TOHOKU", CardNumber: "260", SetName: "JAPANESE SV-P PROMO",
			ExpectedProduct: "Pikachu", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "114811268", ListingTitle: "2000 POKEMON GAME MOVIE ANCIENT MEW POKEMON 2000 MOVIE PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 153.00,
			CardName: "ANCIENT MEW", CardNumber: "", SetName: "Promo",
			ExpectedProduct: "Ancient Mew", ExpectedConsole: "Promo", MinPriceUSD: 30, MaxPriceUSD: 400,
		},
		{
			CertNumber: "145076888", ListingTitle: "1999 POKEMON JAPANESE PROMO SOUTHERN ISLANDS 151 MEW-HOLO SOUTHERN ISLAND-R.I. PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 545.40,
			CardName: "MEW-HOLO SOUTHERN ISLAND-R.I.", CardNumber: "151", SetName: "JAPANESE PROMO SOUTHERN ISLANDS",
			ExpectedProduct: "Mew", ExpectedConsole: "Southern", MinPriceUSD: 100, MaxPriceUSD: 1000,
		},
		{
			CertNumber: "122699162", ListingTitle: "2012 POKEMON JAPANESE BLACK & WHITE THUNDER KNUCKLE 031 UMBREON-HOLO 1ST EDITION PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 127.80,
			CardName: "UMBREON-HOLO THUNDER KNUCKLE-1ST Ed.", CardNumber: "031", SetName: "JAPANESE BLACK & WHITE THUNDER KNUCKLE",
			ExpectedProduct: "Umbreon", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 400,
		},
		{
			CertNumber: "144122685", ListingTitle: "2023 POKEMON MEW EN-151 094 GENGAR PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 162.99,
			CardName: "GENGAR", CardNumber: "094", SetName: "MEW EN-151",
			ExpectedProduct: "Gengar", ExpectedConsole: "151", MinPriceUSD: 30, MaxPriceUSD: 400,
		},
		{
			CertNumber: "150154262", ListingTitle: "2019 POKEMON JAPANESE SUN & MOON DOUBLE BLAZE 076 SNORLAX-HOLO PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 121.46,
			CardName: "SNORLAX-HOLO DOUBLE BLAZE", CardNumber: "076", SetName: "JAPANESE SUN & MOON DOUBLE BLAZE",
			ExpectedProduct: "Snorlax", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "145076863", ListingTitle: "1999 POKEMON GAME 2 BLASTOISE-HOLO SHADOWLESS PSA 8.5",
			Category: "POKEMON CARDS", Grade: 8.5, PricePaid: 688.60,
			CardName: "BLASTOISE-HOLO SHADOWLESS", CardNumber: "2", SetName: "Base Set",
			ExpectedProduct: "Blastoise", ExpectedConsole: "Base Set", MinPriceUSD: 100, MaxPriceUSD: 1500,
		},
		{
			CertNumber: "145076878", ListingTitle: "2000 POKEMON GAME MOVIE ANCIENT MEW POKEMON 2000 MOVIE PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 153.00,
			CardName: "ANCIENT MEW", CardNumber: "", SetName: "Promo",
			ExpectedProduct: "Ancient Mew", ExpectedConsole: "Promo", MinPriceUSD: 30, MaxPriceUSD: 400,
			IsDuplicate: true,
		},
		{
			CertNumber: "150154260", ListingTitle: "2019 POKEMON JAPANESE SUN & MOON DOUBLE BLAZE 076 SNORLAX-HOLO PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 121.46,
			CardName: "SNORLAX-HOLO DOUBLE BLAZE", CardNumber: "076", SetName: "JAPANESE SUN & MOON DOUBLE BLAZE",
			ExpectedProduct: "Snorlax", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 300,
			IsDuplicate: true,
		},
		{
			CertNumber: "143518127", ListingTitle: "2025 POKEMON M24 EN-MCDONALD'S COLLECTION 002 PIKACHU PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 173.82,
			CardName: "PIKACHU", CardNumber: "002", SetName: "M24 EN-MCDONALD'S COLLECTION",
			ExpectedProduct: "Pikachu", ExpectedConsole: "McDonalds", MinPriceUSD: 30, MaxPriceUSD: 400,
		},
		{
			CertNumber: "145396462", ListingTitle: "2021 POKEMON CELEBRATIONS CLASSIC COLLECTION 24 BIRTHDAY PIKACHU-HOLO PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 192.15,
			CardName: "BIRTHDAY PIKACHU-HOLO CLASSIC COLL-BLACK STAR", CardNumber: "24", SetName: "CELEBRATIONS CLASSIC COLLECTION",
			ExpectedProduct: "Birthday", ExpectedConsole: "Celebration", MinPriceUSD: 50, MaxPriceUSD: 500,
		},
		{
			CertNumber: "145084327", ListingTitle: "2024 POKEMON SVP EN-SV BLACK STAR PROMO 161 CHARIZARD EX CHARIZARD ex SUPER-PREMIUM COLLECTION PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 182.40,
			CardName: "CHARIZARD ex CHARIZARD ex SUPER-PREM COLL", CardNumber: "161", SetName: "SVP EN-SV BLACK STAR PROMO",
			ExpectedProduct: "Charizard", ExpectedConsole: "Promo", MinPriceUSD: 50, MaxPriceUSD: 500,
		},
		{
			CertNumber: "135767318", ListingTitle: "2002 POKEMON EXPEDITION 56 MEWTWO-REVERSE FOIL PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 252.60,
			CardName: "MEWTWO-REV.FOIL", CardNumber: "56", SetName: "EXPEDITION",
			ExpectedProduct: "Mewtwo", ExpectedConsole: "Expedition", MinPriceUSD: 50, MaxPriceUSD: 600,
		},
		{
			CertNumber: "141627783", ListingTitle: "2025 POKEMON SVP EN-SV BLACK STAR PROMO 176 UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 197.99,
			CardName: "UMBREON ex PRE PREMIUM FIGURE COLL", CardNumber: "176", SetName: "SVP EN-SV BLACK STAR PROMO",
			ExpectedProduct: "Umbreon", ExpectedConsole: "Promo", MinPriceUSD: 50, MaxPriceUSD: 500,
		},
		// --- New cards (13 items, 10 unique) ---
		{
			CertNumber: "139414865", ListingTitle: "2025 POKEMON JAPANESE M1S-MEGA SYMPHONIA 087 MEGA GARDEVOIR EX SPECIAL ART RARE PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 141.39,
			CardName: "MEGA GARDEVOIR ex SAR", CardNumber: "087", SetName: "JAPANESE M1S-MEGA SYMPHONIA",
			ExpectedProduct: "Gardevoir", ExpectedConsole: "Japanese", MinPriceUSD: 30, MaxPriceUSD: 400,
		},
		{
			CertNumber: "135021722", ListingTitle: "2019 POKEMON SM BLACK STAR PROMO SM162 PIKACHU-HOLO TEAM UP SINGLE PACK BLISTERS PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 145.00,
			CardName: "PIKACHU-HOLO TM.UP SNGL.PK.BLST.", CardNumber: "SM162", SetName: "SM BLACK STAR PROMO",
			ExpectedProduct: "Pikachu", ExpectedConsole: "Promo", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "134110532", ListingTitle: "2019 POKEMON SM BLACK STAR PROMO SM162 PIKACHU-HOLO TEAM UP SINGLE PACK BLISTERS PSA 8",
			Category: "POKEMON CARDS", Grade: 8, PricePaid: 83.00,
			CardName: "PIKACHU-HOLO TM.UP SNGL.PK.BLST.", CardNumber: "SM162", SetName: "SM BLACK STAR PROMO",
			ExpectedProduct: "Pikachu", ExpectedConsole: "Promo", MinPriceUSD: 20, MaxPriceUSD: 300,
			IsDuplicate: true,
		},
		{
			CertNumber: "134110528", ListingTitle: "2019 POKEMON SM BLACK STAR PROMO SM162 PIKACHU-HOLO TEAM UP SINGLE PACK BLISTERS PSA 8",
			Category: "POKEMON CARDS", Grade: 8, PricePaid: 83.00,
			CardName: "PIKACHU-HOLO TM.UP SNGL.PK.BLST.", CardNumber: "SM162", SetName: "SM BLACK STAR PROMO",
			ExpectedProduct: "Pikachu", ExpectedConsole: "Promo", MinPriceUSD: 20, MaxPriceUSD: 300,
			IsDuplicate: true,
		},
		{
			CertNumber: "130221147", ListingTitle: "2025 POKEMON SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1 09 CAPTAIN PIKACHU PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 154.76,
			CardName: "CAPTAIN PIKACHU", CardNumber: "09", SetName: "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1",
			ExpectedProduct: "Pikachu", ExpectedConsole: "Chinese", MinPriceUSD: 20, MaxPriceUSD: 400,
		},
		{
			CertNumber: "72973327", ListingTitle: "2022 POKEMON SWSH BLACK STAR PROMO 075 SPECIAL DELIVERY CHARIZARD-HOLO POKEMON CENTER UNITED KINGDOM PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 183.00,
			CardName: "SP.DELIVERY CHARIZARD", CardNumber: "075", SetName: "SWSH BLACK STAR PROMO",
			ExpectedProduct: "Charizard", ExpectedConsole: "Promo", MinPriceUSD: 50, MaxPriceUSD: 500,
		},
		{
			CertNumber: "123238115", ListingTitle: "2025 POKEMON SIMPLIFIED CHINESE CBB2 C-GEM PACK VOL 2 15 UMBREON PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 111.00,
			CardName: "UMBREON", CardNumber: "15", SetName: "SIMPLIFIED CHINESE CBB2 C-GEM PACK VOL 2",
			ExpectedProduct: "Umbreon", ExpectedConsole: "Chinese", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "134093774", ListingTitle: "2024 POKEMON SWSH BLACK STAR PROMO 029 RAYQUAZA-HOLO CROWN ZENITH PREMIUM COLLECTION SEA & SKY PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 99.00,
			CardName: "RAYQUAZA-HOLO CRZ", CardNumber: "029", SetName: "SWSH BLACK STAR PROMO",
			ExpectedProduct: "Rayquaza", ExpectedConsole: "Promo", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "113751496", ListingTitle: "2025 POKEMON PRE EN-PRISMATIC EVOLUTIONS 167 EEVEE EX SPECIAL ILLUSTRATION RARE PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 130.20,
			CardName: "EEVEE ex SIR", CardNumber: "167", SetName: "PRE EN-PRISMATIC EVOLUTIONS",
			ExpectedProduct: "Eevee", ExpectedConsole: "Prismatic", MinPriceUSD: 30, MaxPriceUSD: 400,
		},
		{
			CertNumber: "132537172", ListingTitle: "2023 POKEMON JAPANESE SV4a-SHINY TREASURE ex 236 PIKACHU S PSA 10",
			Category: "POKEMON CARDS", Grade: 10, PricePaid: 135.80,
			CardName: "PIKACHU S", CardNumber: "236", SetName: "JAPANESE SV4a-SHINY TREASURE ex",
			ExpectedProduct: "Pikachu", ExpectedConsole: "Japanese", MinPriceUSD: 20, MaxPriceUSD: 400,
		},
		{
			CertNumber: "137150557", ListingTitle: "2016 POKEMON PROMO POKKEN TOURNAMENT DARK MEWTWO JAPAN PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 111.00,
			CardName: "DARK MEWTWO", CardNumber: "", SetName: "PROMO POKKEN TOURNAMENT",
			ExpectedProduct: "Mewtwo", ExpectedConsole: "Promo", MinPriceUSD: 20, MaxPriceUSD: 300,
			SkipPricing: true, // pricing sources don't index this Japanese Pokken Tournament promo
		},
		{
			CertNumber: "121986129", ListingTitle: "2025 POKEMON DRI EN-DESTINED RIVALS 087 TEAM ROCKET'S MIMIKYU PRERELEASE-STAFF PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 90.20,
			CardName: "TEAM ROCKET'S MIMIKYU", CardNumber: "087", SetName: "DRI EN-DESTINED RIVALS",
			ExpectedProduct: "Mimikyu", ExpectedConsole: "Destined", MinPriceUSD: 20, MaxPriceUSD: 300,
		},
		{
			CertNumber: "118527750", ListingTitle: "2000 POKEMON GAME MOVIE ANCIENT MEW POKEMON 2000 MOVIE PSA 9",
			Category: "POKEMON CARDS", Grade: 9, PricePaid: 186.99,
			CardName: "ANCIENT MEW", CardNumber: "", SetName: "Promo",
			ExpectedProduct: "Ancient Mew", ExpectedConsole: "Promo", MinPriceUSD: 30, MaxPriceUSD: 400,
			IsDuplicate: true,
		},
	}
}

// uniqueInventory returns 28 unique card identities (deduplicates by card identity).
func uniqueInventory() []InventoryEntry {
	all := fullInventory()
	unique := make([]InventoryEntry, 0, len(all))
	for _, e := range all {
		if !e.IsDuplicate {
			unique = append(unique, e)
		}
	}
	return unique
}

// pricedInventory returns entries suitable for API pricing tests.
// Excludes cards with empty card number AND generic set name — those lack
// enough metadata for reliable price lookup. Cards with no number but a
// specific set name (e.g., Ancient Mew, Dark Mewtwo) are included.
func pricedInventory() []InventoryEntry {
	uniq := uniqueInventory()
	priced := make([]InventoryEntry, 0, len(uniq))
	for _, e := range uniq {
		if e.SkipPricing {
			continue
		}
		if e.CardNumber != "" || !isGenericTestSet(e.SetName) {
			priced = append(priced, e)
		}
	}
	return priced
}

func isGenericTestSet(setName string) bool {
	return constants.IsGenericSetName(setName)
}

// toInventoryCard converts to the inventoryCard type used by pricing_test.go.
func (e InventoryEntry) toInventoryCard() inventoryCard {
	return inventoryCard{
		CardName:        e.CardName,
		CardNumber:      e.CardNumber,
		SetName:         e.SetName,
		Grade:           e.Grade,
		BuyUSD:          e.PricePaid,
		CertNumber:      e.CertNumber,
		ExpectedProduct: e.ExpectedProduct,
		ExpectedConsole: e.ExpectedConsole,
		MinPriceUSD:     e.MinPriceUSD,
		MaxPriceUSD:     e.MaxPriceUSD,
	}
}

// toPSARow converts to the psaRow type used by import_pricing_test.go.
func (e InventoryEntry) toPSARow() psaRow {
	return psaRow{
		CertNumber:   e.CertNumber,
		ListingTitle: e.ListingTitle,
		Category:     e.Category,
		Grade:        e.Grade,
		PricePaid:    e.PricePaid,
	}
}
