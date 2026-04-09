// DH Match & Search endpoint integration tests.
// Tests the /enterprise/match and /enterprise/search endpoints against
// real examples that failed cert resolution (ambiguous or not_found).
//
// Run with: go test ./internal/integration/ -tags integration -v -run TestDHMatchSearch -timeout 5m
//
//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
)

// testCard represents a card that failed cert resolution.
type testCard struct {
	name       string // descriptive label
	cardName   string // cleaned card name we sent to resolve
	setName    string // set name we sent
	cardNumber string // card number
	year       string
	variant    string
	language   string // expected language
	certStatus string // what certs/resolve returned
}

// failedExamples are real cards from the 2026-04-09 logs that returned
// ambiguous or not_found from certs/resolve.
var failedExamples = []testCard{
	// --- English cards (ambiguous) ---
	{name: "Pikachu 151 IR", cardName: "Pikachu", setName: "MEW EN-151", cardNumber: "173", year: "2023", language: "en", certStatus: "ambiguous"},
	{name: "Charizard ex 151", cardName: "Charizard Ex", setName: "MEW EN-151", cardNumber: "183", year: "2023", language: "en", certStatus: "ambiguous"},
	{name: "Mew ex 151 UPC", cardName: "Mew Ex", setName: "151 ULTRA-PREMIUM COLLECTION", cardNumber: "205", year: "2023", language: "en", certStatus: "ambiguous"},
	{name: "Rayquaza XY64", cardName: "Rayquaza", setName: "XY BLACK STAR PROMOS", cardNumber: "XY64", variant: "Holo", language: "en", certStatus: "ambiguous"},
	{name: "Snorlax SV Promo", cardName: "Snorlax 151 Pokemon Center Etb", setName: "SVP EN-SV BLACK STAR PROMO 051 SNORLAX", cardNumber: "051", year: "2023", language: "en", certStatus: "ambiguous"},
	{name: "Dedenne GX UNB", cardName: "Dedenne Gx Unb-'20 Yaaa-trnr's.tlkt", setName: "SUN & MOON UNBROKEN BONDS", cardNumber: "195a", year: "2019", language: "en", certStatus: "ambiguous"},
	{name: "Ethan's Typhlosion DRI", cardName: "Ethan's Typhlosion Prerelease-staff", setName: "DRI EN-DESTINED RIVALS", cardNumber: "034", year: "2025", language: "en", certStatus: "ambiguous"},
	{name: "Gengar XY Breakthrough", cardName: "Gengar", setName: "Pokemon Xy Breakthrough", cardNumber: "60", year: "2015", variant: "Holo", language: "en", certStatus: "ambiguous"},

	// --- Japanese cards (ambiguous) ---
	{name: "Gengar JP XY Blue Shock", cardName: "Gengar", setName: "Pokemon Japanese Xy Blue Shock", cardNumber: "024", language: "ja", certStatus: "ambiguous"},
	{name: "Dragonite JP Holon Research", cardName: "Dragonite", setName: "Pokemon Japanese Holon Research Tower", cardNumber: "039", language: "ja", certStatus: "ambiguous"},
	{name: "Ho-Oh EX JP Dragon Blade", cardName: "Ho-oh Ex Dragon Blade", setName: "JAPANESE BLACK & WHITE DRAGON BLADE", cardNumber: "051", language: "ja", certStatus: "ambiguous"},
	{name: "Emolga JP Shiny Collection", cardName: "Fa/emolga", setName: "Pokemon Japanese Black & White Shiny Collection", cardNumber: "023", language: "ja", certStatus: "matched"},

	// --- Simplified Chinese cards (ambiguous) ---
	{name: "Glaceon Vmax CN", cardName: "Glaceon Vmax", setName: "Pokemon Simplified Chinese Cs4a C-Polychromatic Gathering: Friend", cardNumber: "035", language: "zh", certStatus: "ambiguous"},
	{name: "Charizard Vmax CN", cardName: "Charizard Vmax", setName: "Pokemon Simplified Chinese Cs2a C-VIVID Portrayals: Obsidian", cardNumber: "031", language: "zh", certStatus: "ambiguous"},
	{name: "Umbreon Vmax CN", cardName: "Umbreon Vmax", setName: "Pokemon Simplified Chinese Cs4a C-Polychromatic Gathering: Friend", cardNumber: "085", language: "zh", certStatus: "ambiguous"},

	// --- Not found ---
	{name: "Umbreon Gold Star Celebrations", cardName: "Umbreon", setName: "Pokemon Celebrations Classic Collection", cardNumber: "17", year: "2021", variant: "Gold Star", language: "en", certStatus: "not_found"},
	{name: "Luffy One Piece", cardName: "Monkey D. Luffy Black & White Alt Art", setName: "One Piece Starter Deck St21-Ex Gear 5", cardNumber: "001", year: "2025", language: "en", certStatus: "not_found"},
	{name: "Roucarnage FR Jungle", cardName: "Roucarnage", setName: "FRENCH JUNGLE", cardNumber: "8", year: "2000", variant: "Holo", language: "fr", certStatus: "not_found"},
	{name: "Dracaufeu FR Celebrations", cardName: "Dracaufeu", setName: "CELEBRATIONS CLASSIC COLLECTION", cardNumber: "4", year: "2021", variant: "Holo", language: "fr", certStatus: "not_found"},
	{name: "Birthday Pikachu Celebrations", cardName: "Birthday Pikachu", setName: "CELEBRATIONS CLASSIC COLLECTION", cardNumber: "24", year: "2021", variant: "Holo", language: "en", certStatus: "not_found"},
	{name: "Pikachu Expedition", cardName: "Pikachu", setName: "", cardNumber: "", language: "en", certStatus: "not_found"},
}

// TestDHMatchSearch_MatchEndpoint tests /enterprise/match with structured metafields.
func TestDHMatchSearch_MatchEndpoint(t *testing.T) {
	c := newDHEnterpriseClient(t)

	matched, total := 0, 0
	for _, tc := range failedExamples {
		t.Run("match/"+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			total++

			// Try with structured metafields first
			req := dh.MatchRequest{
				Metafields: &dh.MatchMetafield{
					Pokemon: tc.cardName,
					SetName: tc.setName,
					Number:  tc.cardNumber,
				},
			}

			resp, err := c.MatchCard(ctx, req)
			if err != nil {
				t.Logf("MATCH ERROR: %v", err)
				return
			}

			method := "<nil>"
			if resp.MatchMethod != nil {
				method = *resp.MatchMethod
			}

			if resp.Matched && resp.Card != nil {
				matched++
				t.Logf("MATCH HIT: confidence=%.2f method=%s dh_card_id=%d name=%q set=%q number=%q (was: %s)",
					resp.Confidence, method, resp.Card.DHCardID, resp.Card.Name, resp.Card.SetName, resp.Card.Number, tc.certStatus)
			} else {
				t.Logf("MATCH MISS: confidence=%.2f method=%s (was: %s)", resp.Confidence, method, tc.certStatus)
			}
		})
	}

	t.Logf("\n=== MATCH SUMMARY: %d/%d matched (%.0f%%) ===", matched, total, float64(matched)/float64(max(total, 1))*100)
}

// TestDHMatchSearch_MatchTitle tests /enterprise/match with title-only (raw PSA name).
func TestDHMatchSearch_MatchTitle(t *testing.T) {
	c := newDHEnterpriseClient(t)

	// A few examples with the raw PSA title as used for matching
	titleExamples := []struct {
		name  string
		title string
		was   string
	}{
		{"Pikachu 151", "PIKACHU ILLUSTRATION RARE 151", "ambiguous"},
		{"Charizard ex 151", "CHARIZARD ex ULTRA RARE 151", "ambiguous"},
		{"Rayquaza XY Promo", "RAYQUAZA-HOLO BLACK STAR PROMOS XY64", "ambiguous"},
		{"Gengar Breakthrough", "GENGAR-HOLO BRKTHR.-COSMOS-CHMPS.TIN", "ambiguous"},
		{"Mega Gengar ex JP", "MEGA GENGAR ex SPECIAL ART RARE M2a", "matched"},
		{"Birthday Pikachu", "BIRTHDAY PIKACHU-HOLO CLASSIC COLL-BLACK STAR", "not_found"},
		{"Umbreon Gold Star", "UMBREON-GOLD STAR CLASSIC COLL-POP SERIES 5", "not_found"},
	}

	matched := 0
	for _, tc := range titleExamples {
		t.Run("title/"+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := c.MatchCard(ctx, dh.MatchRequest{Title: tc.title})
			if err != nil {
				t.Logf("MATCH ERROR: %v", err)
				return
			}

			method := "<nil>"
			if resp.MatchMethod != nil {
				method = *resp.MatchMethod
			}

			if resp.Matched && resp.Card != nil {
				matched++
				t.Logf("TITLE HIT: confidence=%.2f method=%s dh_card_id=%d name=%q set=%q (was: %s)",
					resp.Confidence, method, resp.Card.DHCardID, resp.Card.Name, resp.Card.SetName, tc.was)
			} else {
				t.Logf("TITLE MISS: confidence=%.2f method=%s (was: %s)", resp.Confidence, method, tc.was)
			}
		})
	}
}

// TestDHMatchSearch_SearchEndpoint tests /enterprise/search as a disambiguation tool.
func TestDHMatchSearch_SearchEndpoint(t *testing.T) {
	c := newDHEnterpriseClient(t)

	// Test search with name + set + number filters
	searchCases := []struct {
		name     string
		query    string
		set      string
		number   string
		language string
		was      string
	}{
		{"Pikachu 151", "Pikachu", "151", "173", "en", "ambiguous"},
		{"Charizard ex 151", "Charizard ex", "151", "183", "en", "ambiguous"},
		{"Mew ex 151", "Mew ex", "151", "205", "en", "ambiguous"},
		{"Rayquaza XY Promo", "Rayquaza", "XY Black Star Promos", "XY64", "en", "ambiguous"},
		{"Gengar Breakthrough", "Gengar", "XY Breakthrough", "60", "en", "ambiguous"},
		{"Gengar JP Blue Shock", "Gengar", "Blue Shock", "024", "ja", "ambiguous"},
		{"Dragonite JP Holon", "Dragonite", "Holon Research Tower", "039", "ja", "ambiguous"},
		{"Glaceon Vmax CN", "Glaceon Vmax", "", "035", "zh", "ambiguous"},
		{"Charizard Vmax CN", "Charizard Vmax", "", "031", "zh", "ambiguous"},
		{"Umbreon Gold Star", "Umbreon", "Celebrations", "17", "en", "not_found"},
		{"Birthday Pikachu", "Birthday Pikachu", "Celebrations", "24", "en", "not_found"},
		{"Roucarnage FR", "Pidgeot", "Jungle", "8", "fr", "not_found"},
		{"Dracaufeu FR", "Charizard", "Celebrations", "4", "fr", "not_found"},
	}

	found := 0
	for _, tc := range searchCases {
		t.Run("search/"+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := c.SearchCards(ctx, dh.SearchFilters{
				Query:    tc.query,
				Set:      tc.set,
				Number:   tc.number,
				Language: tc.language,
				PerPage:  5,
			})
			if err != nil {
				t.Logf("SEARCH ERROR: %v", err)
				return
			}

			t.Logf("SEARCH: total_hits=%d filtered_hits=%d (was: %s)", resp.Meta.TotalHits, resp.Meta.FilteredHits, tc.was)

			for i, r := range resp.Results {
				if i >= 3 {
					t.Logf("  ... and %d more", len(resp.Results)-3)
					break
				}
				t.Logf("  [%d] id=%d name=%q set=%q number=%q lang=%s price=$%.2f",
					i+1, r.ID, r.Name, r.SetName, r.Number, r.Language, r.MarketPrice)
			}

			if len(resp.Results) > 0 {
				found++
			}
		})
	}

	t.Logf("\n=== SEARCH SUMMARY: %d/%d found results ===", found, len(searchCases))
}

// TestDHMatchSearch_SearchThenMatch tries search to find set names, then match with correct names.
func TestDHMatchSearch_SearchThenMatch(t *testing.T) {
	c := newDHEnterpriseClient(t)

	// First search for a card to discover DH's set name, then try match with it
	steps := []struct {
		name        string
		searchQuery string
		searchSet   string
		searchNum   string
	}{
		{"Pikachu 151 IR", "Pikachu", "151", "173"},
		{"Charizard ex 151", "Charizard ex", "151", "183"},
		{"Gengar Breakthrough", "Gengar", "Breakthrough", "60"},
	}

	for _, tc := range steps {
		t.Run("search-then-match/"+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Step 1: Search to find DH's exact card/set names
			searchResp, err := c.SearchCards(ctx, dh.SearchFilters{
				Query:   tc.searchQuery,
				Set:     tc.searchSet,
				Number:  tc.searchNum,
				PerPage: 3,
			})
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(searchResp.Results) == 0 {
				t.Logf("No search results, skipping match step")
				return
			}

			first := searchResp.Results[0]
			t.Logf("Search found: id=%d name=%q set=%q number=%q", first.ID, first.Name, first.SetName, first.Number)

			// Step 2: Try match using DH's own set name
			matchResp, err := c.MatchCard(ctx, dh.MatchRequest{
				Metafields: &dh.MatchMetafield{
					Pokemon: first.Name,
					SetName: first.SetName,
					Number:  first.Number,
				},
			})
			if err != nil {
				t.Fatalf("Match failed: %v", err)
			}

			method := "<nil>"
			if matchResp.MatchMethod != nil {
				method = *matchResp.MatchMethod
			}

			if matchResp.Matched && matchResp.Card != nil {
				t.Logf("MATCH with DH names: confidence=%.2f method=%s dh_card_id=%d name=%q set=%q",
					matchResp.Confidence, method, matchResp.Card.DHCardID, matchResp.Card.Name, matchResp.Card.SetName)

				// Verify IDs match
				if matchResp.Card.DHCardID != first.ID {
					t.Logf("WARNING: search ID=%d != match ID=%d", first.ID, matchResp.Card.DHCardID)
				}
			} else {
				t.Logf("MATCH MISS even with DH's own names: confidence=%.2f method=%s", matchResp.Confidence, method)
			}
		})
	}
}

// TestDHMatchSearch_CompareResolveVsMatch compares cert resolve with match for the same cards.
func TestDHMatchSearch_CompareResolveVsMatch(t *testing.T) {
	c := newDHEnterpriseClient(t)

	// Cards with cert numbers that we know failed resolve
	comparisons := []struct {
		name       string
		certNumber string
		cardName   string
		setName    string
		cardNumber string
	}{
		{"Pikachu 151", "142296435", "Pikachu", "MEW EN-151", "173"},
		{"Charizard ex 151", "144265568", "Charizard Ex", "MEW EN-151", "183"},
		{"Mew ex 151 UPC", "132042223", "Mew Ex", "151 ULTRA-PREMIUM COLLECTION", "205"},
	}

	for _, tc := range comparisons {
		t.Run("compare/"+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Resolve
			resolveResp, err := c.ResolveCert(ctx, dh.CertResolveRequest{
				CertNumber: tc.certNumber,
				CardName:   tc.cardName,
				SetName:    tc.setName,
				CardNumber: tc.cardNumber,
			})
			if err != nil {
				t.Logf("Resolve ERROR: %v", err)
			} else {
				t.Logf("RESOLVE: status=%s dh_card_id=%d candidates=%d",
					resolveResp.Status, resolveResp.DHCardID, len(resolveResp.Candidates))
				for i, cand := range resolveResp.Candidates {
					t.Logf("  candidate[%d]: id=%d name=%q set=%q number=%q", i, cand.DHCardID, cand.CardName, cand.SetName, cand.CardNumber)
				}
			}

			// Match with metafields
			matchResp, err := c.MatchCard(ctx, dh.MatchRequest{
				Metafields: &dh.MatchMetafield{
					Pokemon: tc.cardName,
					SetName: tc.setName,
					Number:  tc.cardNumber,
				},
			})
			if err != nil {
				t.Logf("Match ERROR: %v", err)
			} else {
				method := "<nil>"
				if matchResp.MatchMethod != nil {
					method = *matchResp.MatchMethod
				}
				if matchResp.Matched && matchResp.Card != nil {
					t.Logf("MATCH:   matched=true confidence=%.2f method=%s dh_card_id=%d name=%q set=%q",
						matchResp.Confidence, method, matchResp.Card.DHCardID, matchResp.Card.Name, matchResp.Card.SetName)
				} else {
					t.Logf("MATCH:   matched=false confidence=%.2f method=%s", matchResp.Confidence, method)
				}
			}

			// Also try title-based match with raw PSA name pattern
			rawTitle := fmt.Sprintf("%s %s %s", tc.cardName, tc.setName, tc.cardNumber)
			titleResp, err := c.MatchCard(ctx, dh.MatchRequest{Title: rawTitle})
			if err != nil {
				t.Logf("Title Match ERROR: %v", err)
			} else {
				method := "<nil>"
				if titleResp.MatchMethod != nil {
					method = *titleResp.MatchMethod
				}
				if titleResp.Matched && titleResp.Card != nil {
					t.Logf("TITLE:   matched=true confidence=%.2f method=%s dh_card_id=%d name=%q set=%q",
						titleResp.Confidence, method, titleResp.Card.DHCardID, titleResp.Card.Name, titleResp.Card.SetName)
				} else {
					t.Logf("TITLE:   matched=false confidence=%.2f method=%s", titleResp.Confidence, method)
				}
			}
		})
	}
}
