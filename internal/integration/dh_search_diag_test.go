// DH Search diagnostic tests — probing what search actually returns.
// Run with: go test ./internal/integration/ -tags integration -v -run TestDHSearchDiag -timeout 5m
//
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
)

// TestDHSearchDiag_BasicQueries tests simple search queries to understand what the API returns.
func TestDHSearchDiag_BasicQueries(t *testing.T) {
	c := newDHEnterpriseClient(t)

	queries := []struct {
		name  string
		query string
		set   string
		num   string
		lang  string
	}{
		// Simplest possible queries
		{"just Pikachu", "Pikachu", "", "", ""},
		{"just Charizard", "Charizard", "", "", ""},
		{"Pikachu + en", "Pikachu", "", "", "en"},
		{"Charizard ex", "Charizard ex", "", "", ""},

		// Try set names that match DH's candidate format
		{"Pikachu in 151", "Pikachu", "Pokemon Scarlet & Violet 151", "", ""},
		{"Charizard in SV 151", "Charizard ex", "Pokemon Scarlet & Violet 151", "", ""},
		{"Mew ex in SV 151", "Mew ex", "Pokemon Scarlet & Violet 151", "", ""},
		{"Mew ex #205", "Mew ex", "", "205", ""},

		// Number-only search within a set
		{"SV 151 #173", "", "Pokemon Scarlet & Violet 151", "173", ""},
		{"SV 151 #183", "", "Pokemon Scarlet & Violet 151", "183", ""},

		// Try query-only with set name
		{"query: Pikachu 151", "Pikachu 151", "", "", ""},
		{"query: Charizard ex 151", "Charizard ex 151", "", "", ""},
		{"query: Gengar Breakthrough", "Gengar Breakthrough", "", "", ""},
		{"query: Rayquaza XY Promo", "Rayquaza XY Promo", "", "", ""},

		// Japanese
		{"query: Gengar Japanese", "Gengar", "", "", "ja"},
	}

	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := c.SearchCards(ctx, dh.SearchFilters{
				Query:    tc.query,
				Set:      tc.set,
				Number:   tc.num,
				Language: tc.lang,
				PerPage:  5,
			})
			if err != nil {
				t.Logf("ERROR: %v", err)
				return
			}

			t.Logf("hits=%d filtered=%d pages=%d", resp.Meta.TotalHits, resp.Meta.FilteredHits, resp.Meta.TotalPages)
			for i, r := range resp.Results {
				if i >= 5 {
					break
				}
				t.Logf("  [%d] id=%d name=%q set=%q num=%q lang=%s $%.2f",
					i+1, r.ID, r.Name, r.SetName, r.Number, r.Language, r.MarketPrice)
			}
		})
	}
}

// TestDHSearchDiag_MatchWithDHNames tries /match using DH's own naming from resolve candidates.
func TestDHSearchDiag_MatchWithDHNames(t *testing.T) {
	c := newDHEnterpriseClient(t)

	// Use exact names from the resolve candidates we got in the comparison test
	dhCards := []struct {
		name    string
		pokemon string
		setName string
		number  string
	}{
		{"Pikachu Jungle 60", "Pikachu", "Pokemon Jungle", "60"},
		{"Charizard ex SV 151 199", "Charizard ex", "Pokemon Scarlet & Violet 151", "199"},
		{"Mew ex SV 151 205", "Mew ex", "Pokemon Scarlet & Violet 151", "205"},
		{"Pikachu ex Surging Sparks 238", "Pikachu ex", "Pokemon Surging Sparks", "238"},
	}

	for _, tc := range dhCards {
		t.Run("dh-names/"+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := c.MatchCard(ctx, dh.MatchRequest{
				Metafields: &dh.MatchMetafield{
					Pokemon: tc.pokemon,
					SetName: tc.setName,
					Number:  tc.number,
				},
			})
			if err != nil {
				t.Logf("ERROR: %v", err)
				return
			}

			method := "<nil>"
			if resp.MatchMethod != nil {
				method = *resp.MatchMethod
			}

			if resp.Matched && resp.Card != nil {
				t.Logf("HIT: confidence=%.2f method=%s id=%d name=%q set=%q num=%q",
					resp.Confidence, method, resp.Card.DHCardID, resp.Card.Name, resp.Card.SetName, resp.Card.Number)
			} else {
				t.Logf("MISS: confidence=%.2f method=%s", resp.Confidence, method)
			}
		})
	}
}

// TestDHSearchDiag_CardLookup tries looking up known card IDs from resolve candidates.
func TestDHSearchDiag_CardLookup(t *testing.T) {
	c := newDHEnterpriseClient(t)

	// Card IDs from the ambiguous resolve candidates
	cardIDs := []struct {
		name string
		id   int
	}{
		{"Pikachu Jungle #60 (id=250)", 250},
		{"Charizard ex SV 151 #199 (id=269)", 269},
		{"Mew ex SV 151 #205 (id=302)", 302},
	}

	for _, tc := range cardIDs {
		t.Run("lookup/"+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := c.CardLookup(ctx, tc.id)
			if err != nil {
				t.Logf("ERROR: %v", err)
				return
			}

			t.Logf("FOUND: id=%d name=%q set=%q number=%q rarity=%q lang=%q year=%q",
				resp.Card.ID, resp.Card.Name, resp.Card.SetName, resp.Card.Number,
				resp.Card.Rarity, resp.Card.Language, resp.Card.Year)
		})
	}
}
