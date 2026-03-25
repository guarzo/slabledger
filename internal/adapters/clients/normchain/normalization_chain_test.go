// Package normchain provides end-to-end pinning tests for the full normalization
// chain: PSA listing title → parseCardMetadataFromTitle → PriceCharting query
// and CardHedger query. These tests pin current behavior so that refactoring
// normalization functions in cardutil, pricecharting, or campaigns catches
// regressions immediately.
package normchain

import (
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// normChainCase represents a single PSA listing title flowing through the full
// normalization pipeline. Expected values were captured from the 28 unique
// entries in internal/integration/testdata_test.go.
type normChainCase struct {
	cert         string // PSA cert number (used as test case name)
	listingTitle string // Raw PSA listing title
	category     string // PSA category

	// Outputs from campaigns.ExportParseCardMetadataFromTitle
	wantCardName   string
	wantCardNumber string
	wantSetName    string

	// Outputs from downstream normalization pipelines
	wantPCQuery string // pricecharting.ExportBuildQuery(setName, cardName, cardNumber)
	wantCHQuery string // cardutil.BuildCardMatchQuery(setName, cardName, cardNumber)
}

// normChainCases covers all 28 unique inventory entries.
var normChainCases = []normChainCase{
	{
		cert: "145455452", listingTitle: "2022 POKEMON JAPANESE SWORD & SHIELD START DECK 100 COROCORO COMIC VERSION 008 SNORLAX PSA 10", category: "POKEMON CARDS",
		wantCardName: "SNORLAX", wantCardNumber: "008", wantSetName: "JAPANESE SWORD & SHIELD START DECK 100 COROCORO COMIC VERSION",
		wantPCQuery: "pokemon SWORD and SHIELD START DECK 100 COROCORO COMIC VERSION SNORLAX #8",
		wantCHQuery: "JAPANESE SWORD SHIELD START DECK 100 COROCORO COMIC VERSION SNORLAX 008",
	},
	{
		cert: "133478793", listingTitle: "2015 POKEMON JAPANESE XY PROMO 126 RAYQUAZA SPIRIT LINK POKEMON CENTER PSA 9", category: "POKEMON CARDS",
		wantCardName: "RAYQUAZA SPIRIT LINK", wantCardNumber: "126", wantSetName: "JAPANESE XY PROMO",
		wantPCQuery: "pokemon XY PROMO RAYQUAZA SPIRIT LINK #126",
		wantCHQuery: "JAPANESE XY PROMO RAYQUAZA SPIRIT LINK 126",
	},
	{
		cert: "143473336", listingTitle: "2000 POKEMON JAPANESE NEO 2 PROMO 006 CHARIZARD REVERSE FOIL PSA 9", category: "POKEMON CARDS",
		wantCardName: "CHARIZARD REVERSE FOIL", wantCardNumber: "006", wantSetName: "JAPANESE NEO 2 PROMO",
		wantPCQuery: "pokemon NEO 2 PROMO CHARIZARD REVERSE FOIL #6",
		wantCHQuery: "JAPANESE NEO 2 PROMO CHARIZARD REVERSE FOIL 006",
	},
	{
		cert: "145076879", listingTitle: "2000 POKEMON ROCKET 8 DARK GYARADOS-HOLO 1ST EDITION PSA 8", category: "POKEMON CARDS",
		wantCardName: "DARK GYARADOS-HOLO 1ST EDITION", wantCardNumber: "8", wantSetName: "ROCKET",
		wantPCQuery: "pokemon ROCKET DARK GYARADOS HOLO 1ST EDITION #8",
		wantCHQuery: "ROCKET DARK GYARADOS Holo 8",
	},
	{
		cert: "149955139", listingTitle: "2025 POKEMON PRE EN-PRISMATIC EVOLUTIONS 156 SYLVEON EX SPECIAL ILLUSTRATION RARE PSA 8", category: "POKEMON CARDS",
		wantCardName: "SYLVEON EX", wantCardNumber: "156", wantSetName: "PRE EN-PRISMATIC EVOLUTIONS",
		wantPCQuery: "pokemon PRISMATIC EVOLUTIONS SYLVEON EX #156",
		wantCHQuery: "PRISMATIC EVOLUTIONS SYLVEON EX 156",
	},
	{
		cert: "143210177", listingTitle: "2014 POKEMON JAPANESE XY RISING FIST 069 DRAGONITE EX 1ST EDITION PSA 10", category: "POKEMON CARDS",
		wantCardName: "DRAGONITE EX 1ST EDITION", wantCardNumber: "069", wantSetName: "JAPANESE XY RISING FIST",
		wantPCQuery: "pokemon XY RISING FIST DRAGONITE EX 1ST EDITION #69",
		wantCHQuery: "JAPANESE XY RISING FIST DRAGONITE EX 069",
	},
	{
		cert: "150154255", listingTitle: "2025 POKEMON JAPANESE SV-P PROMO 260 TOHOKU'S PIKACHU SPECIAL BOX POKEMON CENTER TOHOKU PSA 9", category: "POKEMON CARDS",
		wantCardName: "TOHOKU'S PIKACHU", wantCardNumber: "260", wantSetName: "JAPANESE SV-P PROMO",
		wantPCQuery: "pokemon SVP PROMO TOHOKU'S PIKACHU #260",
		wantCHQuery: "JAPANESE SV P PROMO TOHOKU'S PIKACHU 260",
	},
	{
		cert: "114811268", listingTitle: "2000 POKEMON GAME MOVIE ANCIENT MEW POKEMON 2000 MOVIE PSA 9", category: "POKEMON CARDS",
		wantCardName: "ANCIENT MEW", wantCardNumber: "", wantSetName: "Promo",
		wantPCQuery: "pokemon Promo ANCIENT MEW",
		wantCHQuery: "Promo ANCIENT MEW",
	},
	{
		cert: "145076888", listingTitle: "1999 POKEMON JAPANESE PROMO SOUTHERN ISLANDS 151 MEW-HOLO SOUTHERN ISLAND-R.I. PSA 9", category: "POKEMON CARDS",
		wantCardName: "MEW-HOLO SOUTHERN ISLAND-R.I.", wantCardNumber: "151", wantSetName: "JAPANESE PROMO SOUTHERN ISLANDS",
		wantPCQuery: "pokemon PROMO SOUTHERN ISLANDS MEW HOLO SOUTHERN ISLAND R.I. #151",
		wantCHQuery: "JAPANESE PROMO SOUTHERN ISLANDS MEW Holo SOUTHERN ISLAND R.I. 151",
	},
	{
		cert: "122699162", listingTitle: "2012 POKEMON JAPANESE BLACK & WHITE THUNDER KNUCKLE 031 UMBREON-HOLO 1ST EDITION PSA 9", category: "POKEMON CARDS",
		wantCardName: "UMBREON-HOLO 1ST EDITION", wantCardNumber: "031", wantSetName: "JAPANESE BLACK & WHITE THUNDER KNUCKLE",
		wantPCQuery: "pokemon BLACK and WHITE THUNDER KNUCKLE UMBREON HOLO 1ST EDITION #31",
		wantCHQuery: "JAPANESE BLACK WHITE THUNDER KNUCKLE UMBREON Holo 031",
	},
	{
		cert: "144122685", listingTitle: "2023 POKEMON MEW EN-151 094 GENGAR PSA 10", category: "POKEMON CARDS",
		wantCardName: "GENGAR", wantCardNumber: "094", wantSetName: "MEW EN-151",
		wantPCQuery: "pokemon 151 GENGAR #94",
		wantCHQuery: "151 GENGAR 094",
	},
	{
		cert: "150154262", listingTitle: "2019 POKEMON JAPANESE SUN & MOON DOUBLE BLAZE 076 SNORLAX-HOLO PSA 10", category: "POKEMON CARDS",
		wantCardName: "SNORLAX-HOLO", wantCardNumber: "076", wantSetName: "JAPANESE SUN & MOON DOUBLE BLAZE",
		wantPCQuery: "pokemon SUN and MOON DOUBLE BLAZE SNORLAX HOLO #76",
		wantCHQuery: "JAPANESE SUN MOON DOUBLE BLAZE SNORLAX Holo 076",
	},
	{
		cert: "145076863", listingTitle: "1999 POKEMON GAME 2 BLASTOISE-HOLO SHADOWLESS PSA 8.5", category: "POKEMON CARDS",
		wantCardName: "BLASTOISE-HOLO SHADOWLESS", wantCardNumber: "2", wantSetName: "Base Set",
		wantPCQuery: "pokemon Base Set BLASTOISE HOLO SHADOWLESS #2",
		wantCHQuery: "Base Set BLASTOISE Holo 2",
	},
	{
		cert: "143518127", listingTitle: "2025 POKEMON M24 EN-MCDONALD'S COLLECTION 002 PIKACHU PSA 10", category: "POKEMON CARDS",
		wantCardName: "PIKACHU", wantCardNumber: "002", wantSetName: "M24 EN-MCDONALD'S COLLECTION",
		wantPCQuery: "pokemon MCDONALD'S COLLECTION PIKACHU #2",
		wantCHQuery: "MCDONALD'S COLLECTION PIKACHU 002",
	},
	{
		cert: "145396462", listingTitle: "2021 POKEMON CELEBRATIONS CLASSIC COLLECTION 24 BIRTHDAY PIKACHU-HOLO PSA 10", category: "POKEMON CARDS",
		wantCardName: "BIRTHDAY PIKACHU-HOLO", wantCardNumber: "24", wantSetName: "CELEBRATIONS CLASSIC COLLECTION",
		wantPCQuery: "pokemon CELEBRATIONS CLASSIC COLLECTION BIRTHDAY PIKACHU HOLO #24",
		wantCHQuery: "CELEBRATIONS CLASSIC COLLECTION BIRTHDAY PIKACHU Holo 24",
	},
	{
		cert: "145084327", listingTitle: "2024 POKEMON SVP EN-SV BLACK STAR PROMO 161 CHARIZARD EX CHARIZARD ex SUPER-PREMIUM COLLECTION PSA 10", category: "POKEMON CARDS",
		wantCardName: "CHARIZARD EX CHARIZARD ex", wantCardNumber: "161", wantSetName: "SVP EN-SV BLACK STAR PROMO",
		wantPCQuery: "pokemon SVP BLACK STAR PROMOs CHARIZARD EX CHARIZARD ex #161",
		wantCHQuery: "SV BLACK STAR PROMO CHARIZARD ex 161",
	},
	{
		cert: "135767318", listingTitle: "2002 POKEMON EXPEDITION 56 MEWTWO-REVERSE FOIL PSA 9", category: "POKEMON CARDS",
		wantCardName: "MEWTWO-REVERSE FOIL", wantCardNumber: "56", wantSetName: "EXPEDITION",
		wantPCQuery: "pokemon EXPEDITION MEWTWO REVERSE FOIL #56",
		wantCHQuery: "EXPEDITION MEWTWO REVERSE FOIL 56",
	},
	{
		cert: "141627783", listingTitle: "2025 POKEMON SVP EN-SV BLACK STAR PROMO 176 UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION PSA 10", category: "POKEMON CARDS",
		wantCardName: "UMBREON EX", wantCardNumber: "176", wantSetName: "SVP EN-SV BLACK STAR PROMO",
		wantPCQuery: "pokemon SVP BLACK STAR PROMOs UMBREON EX #176",
		wantCHQuery: "SV BLACK STAR PROMO UMBREON EX 176",
	},
	{
		cert: "139414865", listingTitle: "2025 POKEMON JAPANESE M1S-MEGA SYMPHONIA 087 MEGA GARDEVOIR EX SPECIAL ART RARE PSA 10", category: "POKEMON CARDS",
		wantCardName: "MEGA GARDEVOIR EX", wantCardNumber: "087", wantSetName: "JAPANESE M1S-MEGA SYMPHONIA",
		wantPCQuery: "pokemon M1S MEGA SYMPHONIA MEGA GARDEVOIR EX #87",
		wantCHQuery: "JAPANESE M1S MEGA SYMPHONIA MEGA GARDEVOIR EX 087",
	},
	{
		cert: "135021722", listingTitle: "2019 POKEMON SM BLACK STAR PROMO SM162 PIKACHU-HOLO TEAM UP SINGLE PACK BLISTERS PSA 9", category: "POKEMON CARDS",
		wantCardName: "PIKACHU-HOLO", wantCardNumber: "SM162", wantSetName: "SM BLACK STAR PROMO",
		wantPCQuery: "pokemon Promo PIKACHU HOLO #SM162",
		wantCHQuery: "SM BLACK STAR PROMO PIKACHU Holo SM162",
	},
	{
		cert: "130221147", listingTitle: "2025 POKEMON SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1 09 CAPTAIN PIKACHU PSA 9", category: "POKEMON CARDS",
		wantCardName: "CAPTAIN PIKACHU", wantCardNumber: "09", wantSetName: "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1",
		wantPCQuery: "pokemon Chinese GEM PACK VOL 1 CAPTAIN PIKACHU #709",
		wantCHQuery: "Chinese GEM PACK VOL 1 CAPTAIN PIKACHU 09",
	},
	{
		cert: "72973327", listingTitle: "2022 POKEMON SWSH BLACK STAR PROMO 075 SPECIAL DELIVERY CHARIZARD-HOLO POKEMON CENTER UNITED KINGDOM PSA 9", category: "POKEMON CARDS",
		wantCardName: "SPECIAL DELIVERY CHARIZARD-HOLO", wantCardNumber: "075", wantSetName: "SWSH BLACK STAR PROMO",
		wantPCQuery: "pokemon Promo SPECIAL DELIVERY CHARIZARD HOLO #SWSH075",
		wantCHQuery: "SWSH BLACK STAR PROMO SPECIAL DELIVERY CHARIZARD Holo 075",
	},
	{
		cert: "123238115", listingTitle: "2025 POKEMON SIMPLIFIED CHINESE CBB2 C-GEM PACK VOL 2 15 UMBREON PSA 9", category: "POKEMON CARDS",
		wantCardName: "UMBREON", wantCardNumber: "15", wantSetName: "SIMPLIFIED CHINESE CBB2 C-GEM PACK VOL 2",
		wantPCQuery: "pokemon Chinese GEM PACK VOL 2 UMBREON #615",
		wantCHQuery: "Chinese GEM PACK VOL 2 UMBREON 15",
	},
	{
		cert: "134093774", listingTitle: "2024 POKEMON SWSH BLACK STAR PROMO 029 RAYQUAZA-HOLO CROWN ZENITH PREMIUM COLLECTION SEA & SKY PSA 10", category: "POKEMON CARDS",
		wantCardName: "RAYQUAZA-HOLO", wantCardNumber: "029", wantSetName: "SWSH BLACK STAR PROMO",
		wantPCQuery: "pokemon Promo RAYQUAZA HOLO #SWSH029",
		wantCHQuery: "SWSH BLACK STAR PROMO RAYQUAZA Holo 029",
	},
	{
		cert: "113751496", listingTitle: "2025 POKEMON PRE EN-PRISMATIC EVOLUTIONS 167 EEVEE EX SPECIAL ILLUSTRATION RARE PSA 9", category: "POKEMON CARDS",
		wantCardName: "EEVEE EX", wantCardNumber: "167", wantSetName: "PRE EN-PRISMATIC EVOLUTIONS",
		wantPCQuery: "pokemon PRISMATIC EVOLUTIONS EEVEE EX #167",
		wantCHQuery: "PRISMATIC EVOLUTIONS EEVEE EX 167",
	},
	{
		cert: "132537172", listingTitle: "2023 POKEMON JAPANESE SV4a-SHINY TREASURE ex 236 PIKACHU S PSA 10", category: "POKEMON CARDS",
		wantCardName: "PIKACHU S", wantCardNumber: "236", wantSetName: "JAPANESE SV4a-SHINY TREASURE ex",
		wantPCQuery: "pokemon SV4a SHINY TREASURE ex PIKACHU S #236",
		wantCHQuery: "JAPANESE SV4a SHINY TREASURE ex PIKACHU S 236",
	},
	{
		cert: "137150557", listingTitle: "2016 POKEMON PROMO POKKEN TOURNAMENT DARK MEWTWO JAPAN PSA 9", category: "POKEMON CARDS",
		wantCardName: "DARK MEWTWO", wantCardNumber: "", wantSetName: "PROMO POKKEN TOURNAMENT",
		wantPCQuery: "pokemon PROMO POKKEN TOURNAMENT DARK MEWTWO",
		wantCHQuery: "PROMO POKKEN TOURNAMENT DARK MEWTWO",
	},
	{
		cert: "121986129", listingTitle: "2025 POKEMON DRI EN-DESTINED RIVALS 087 TEAM ROCKET'S MIMIKYU PRERELEASE-STAFF PSA 9", category: "POKEMON CARDS",
		wantCardName: "TEAM ROCKET'S MIMIKYU PRERELEASE-STAFF", wantCardNumber: "087", wantSetName: "DRI EN-DESTINED RIVALS",
		wantPCQuery: "pokemon DESTINED RIVALS TEAM ROCKET'S MIMIKYU PRERELEASE STAFF #87",
		wantCHQuery: "DESTINED RIVALS TEAM ROCKET'S MIMIKYU PRERELEASE STAFF 087",
	},
}

// TestNormalizationChainParse verifies that parseCardMetadataFromTitle produces
// the expected card name, card number, and set name for each inventory entry.
func TestNormalizationChainParse(t *testing.T) {
	for _, tc := range normChainCases {
		t.Run(tc.cert, func(t *testing.T) {
			cardName, cardNumber, setName := campaigns.ExportParseCardMetadataFromTitle(tc.listingTitle, tc.category)

			if cardName != tc.wantCardName {
				t.Errorf("cardName: got %q, want %q", cardName, tc.wantCardName)
			}
			if cardNumber != tc.wantCardNumber {
				t.Errorf("cardNumber: got %q, want %q", cardNumber, tc.wantCardNumber)
			}
			if setName != tc.wantSetName {
				t.Errorf("setName: got %q, want %q", setName, tc.wantSetName)
			}
		})
	}
}

// TestNormalizationChainPCQuery verifies the full chain from PSA title through
// parseCardMetadataFromTitle and into pricecharting.ExportBuildQuery.
func TestNormalizationChainPCQuery(t *testing.T) {
	for _, tc := range normChainCases {
		t.Run(tc.cert, func(t *testing.T) {
			cardName, cardNumber, setName := campaigns.ExportParseCardMetadataFromTitle(tc.listingTitle, tc.category)
			pcQuery := pricecharting.ExportBuildQuery(setName, cardName, cardNumber)

			if pcQuery != tc.wantPCQuery {
				t.Errorf("PriceCharting query:\n  got  %q\n  want %q\n  (parsed: name=%q num=%q set=%q)", pcQuery, tc.wantPCQuery, cardName, cardNumber, setName)
			}
		})
	}
}

// TestNormalizationChainCHQuery verifies the full chain from PSA title through
// parseCardMetadataFromTitle and into cardutil.BuildCardMatchQuery.
func TestNormalizationChainCHQuery(t *testing.T) {
	for _, tc := range normChainCases {
		t.Run(tc.cert, func(t *testing.T) {
			cardName, cardNumber, setName := campaigns.ExportParseCardMetadataFromTitle(tc.listingTitle, tc.category)
			chQuery := cardutil.BuildCardMatchQuery(setName, cardName, cardNumber)

			if chQuery != tc.wantCHQuery {
				t.Errorf("CardHedger query:\n  got  %q\n  want %q\n  (parsed: name=%q num=%q set=%q)", chQuery, tc.wantCHQuery, cardName, cardNumber, setName)
			}
		})
	}
}
