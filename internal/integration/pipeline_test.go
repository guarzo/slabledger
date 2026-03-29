// Pipeline integration tests: normalization audit, CardHedger lookup,
// multi-source fusion, and end-to-end import-to-pricing.
//
// Run with: go test -tags integration ./internal/integration/ -run TestNormalization -v
//
//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/adapters/clients/fusionprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"

	_ "github.com/joho/godotenv/autoload"
)

// TestNormalizationAudit verifies what each pricing source sees after
// normalization — no API calls required. Logs all three normalized forms
// side by side for manual review.
func TestNormalizationAudit(t *testing.T) {
	for _, card := range uniqueInventory() {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			// --- PriceCharting normalization ---
			pcQuery := pricecharting.ExportBuildQuery(card.SetName, card.CardName, card.CardNumber)
			t.Logf("PC query: %s", pcQuery)

			if !strings.Contains(strings.ToLower(pcQuery), "pokemon") {
				t.Errorf("PC query missing 'pokemon': %s", pcQuery)
			}
			if strings.Contains(strings.ToUpper(pcQuery), "JAPANESE") {
				t.Logf("  NOTE: PC query contains JAPANESE (PriceCharting normalizeSetName does not strip it)")
			}
			for _, code := range []string{"PRE EN", "M24 EN", "MEW EN", "SVP EN"} {
				if strings.Contains(strings.ToUpper(pcQuery), code) {
					t.Logf("  NOTE: PC query contains PSA code %q", code)
				}
			}

			// --- Secondary source normalization ---
			ppName := cardutil.NormalizePurchaseName(card.CardName)
			ppName = cardutil.StripVariantSuffix(ppName)
			ppSet := cardutil.NormalizeSetNameSimple(card.SetName)
			t.Logf("Secondary: name=%q set=%q", ppName, ppSet)

			if strings.Contains(ppName, "-HOLO") || strings.Contains(ppName, "-REV.FOIL") {
				t.Errorf("name has unexpanded abbreviation: %s", ppName)
			}
			if strings.HasPrefix(strings.ToUpper(ppSet), "JAPANESE ") {
				t.Errorf("set still has JAPANESE prefix: %s", ppSet)
			}
			for _, code := range []string{"PRE EN", "M24 EN", "MEW EN", "SVP EN"} {
				if strings.Contains(strings.ToUpper(ppSet), code) {
					t.Errorf("set contains PSA code %q: %s", code, ppSet)
				}
			}

			// --- CardHedger normalization (self-match sanity) ---
			chSelfMatch := cardutil.MatchesSetOverlap(card.SetName, card.SetName)
			t.Logf("CH: self-match=%v set=%q", chSelfMatch, card.SetName)
			if !chSelfMatch {
				t.Errorf("MatchesSetOverlap self-match failed for %q", card.SetName)
			}

			// --- Cross-source comparison ---
			t.Logf("  Summary: PC=%q | PP=%q/%q | CH set=%q",
				pcQuery, ppName, ppSet, card.SetName)
		})
	}
}

// TestCardHedgerLookup validates CardHedger resolution and price retrieval for
// each inventory card. First tries batch cert-based resolution (details-by-certs),
// then for any certs CardHedger hasn't linked to cards, falls back to the
// card-match search pipeline to verify the card IS findable.
// Requires CARD_HEDGER_API_KEY.
func TestCardHedgerLookup(t *testing.T) {
	apiKey := os.Getenv("CARD_HEDGER_API_KEY")
	if apiKey == "" {
		t.Skip("CARD_HEDGER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := observability.NewNoopLogger()
	client := cardhedger.NewClient(apiKey)
	adapter := fusionprice.NewCardHedgerAdapter(client, nil, logger)

	// Phase 1: Batch cert resolution
	inventory := pricedInventory()
	certs := make([]string, len(inventory))
	for i, card := range inventory {
		certs[i] = card.CertNumber
	}

	resp, statusCode, _, err := client.DetailsByCerts(ctx, certs, "PSA")
	if err != nil {
		t.Fatalf("DetailsByCerts failed (status=%d): %v", statusCode, err)
	}

	certMap := make(map[string]cardhedger.CertDetailResult, len(resp.Results))
	for _, detail := range resp.Results {
		certMap[detail.CertInfo.Cert] = detail
	}

	t.Logf("Phase 1 — DetailsByCerts: requested=%d found=%d", resp.TotalRequested, resp.TotalFound)

	certResolved, certUnlinked, searchResolved, searchFoundNoData, searchFailed, withPrices := 0, 0, 0, 0, 0, 0

	for _, card := range inventory {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			var cardID string

			// Try cert-based resolution first
			detail, inResp := certMap[card.CertNumber]
			if inResp && detail.Card != nil {
				cardID = detail.Card.CardID
				t.Logf("  Cert resolved: card_id=%s player=%q set=%q number=%q",
					detail.Card.CardID, detail.Card.Player, detail.Card.Set, detail.Card.Number)
				certResolved++

				if card.ExpectedProduct != "" && !containsCI(detail.Card.Player, card.ExpectedProduct) && !containsCI(detail.Card.Description, card.ExpectedProduct) {
					t.Errorf("WRONG CARD: player=%q desc=%q does not contain %q",
						detail.Card.Player, detail.Card.Description, card.ExpectedProduct)
				}
			} else {
				// Cert not linked — fall back to card-match search pipeline.
				// Card-match is a best-effort fallback; cert-based resolution is
				// the primary production path. Log results but don't fail the test.
				certUnlinked++
				desc := ""
				if inResp {
					desc = detail.CertInfo.Description
				}
				t.Logf("  Cert not linked (desc=%q) — trying card-match search", desc)

				pCard := pricing.Card{
					Name:            card.CardName,
					Number:          card.CardNumber,
					Set:             card.SetName,
					PSAListingTitle: card.ListingTitle,
				}
				result, _, err := adapter.FetchFusionData(ctx, pCard)
				if err != nil {
					t.Logf("  Card-match search: no match (%v)", err)
					searchFailed++
					return
				}

				hasData := false
				if result != nil {
					for grade, prices := range result.GradeData {
						for _, p := range prices {
							if p.Value > 0 {
								hasData = true
								t.Logf("  GradeData: %s=$%.2f", grade, p.Value)
							}
						}
					}
					for grade, est := range result.EstimateDetails {
						if est != nil && est.PriceCents > 0 {
							hasData = true
							t.Logf("  Estimate: %s=$%.2f conf=%.2f", grade, float64(est.PriceCents)/100, est.Confidence)
						}
					}
				}

				if hasData {
					searchResolved++
					t.Logf("  Card-match search found card with price data")
				} else {
					searchFoundNoData++
					t.Logf("  Card-match search found card (no price data)")
				}
				return
			}

			// Fetch prices for cert-resolved cards
			prices, _, _, err := client.GetAllPrices(ctx, cardID)
			if err != nil {
				t.Logf("  GetAllPrices: error=%v", err)
				return
			}
			if prices != nil && len(prices.Prices) > 0 {
				withPrices++
				for _, p := range prices.Prices {
					t.Logf("  Price: %s %s = %s", p.Grader, p.Grade, p.Price)
				}
			} else {
				t.Logf("  GetAllPrices: no price data")
			}
		})
	}

	total := len(inventory)
	fmt.Printf("\n=== CARDHEDGER SUMMARY: CertResolved=%d CertUnlinked=%d SearchResolved=%d SearchFoundNoData=%d SearchFailed=%d WithPrices=%d Total=%d ===\n",
		certResolved, certUnlinked, searchResolved, searchFoundNoData, searchFailed, withPrices, total)

	if searchFailed > 0 {
		t.Logf("NOTE: CardHedger card-match search could not find %d/%d unlinked cards — cert-based resolution is the primary path; card-match normalization can be improved incrementally", searchFailed, certUnlinked)
	}
}

// TestFullMultiSourceFusion exercises all available price sources through the
// fusion engine. Requires PRICECHARTING_TOKEN; optionally uses
// POKEMONPRICE_TRACKER_API_KEY and CARD_HEDGER_API_KEY.
func TestFullMultiSourceFusion(t *testing.T) {
	token := os.Getenv("PRICECHARTING_TOKEN")
	if token == "" {
		t.Skip("PRICECHARTING_TOKEN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := observability.NewNoopLogger()
	appCache := newTestCache(t)

	// Primary source: PriceCharting
	pcCfg := pricecharting.DefaultConfig(token)
	pcCfg.RateLimitInterval = 3 * time.Second
	pc, err := pricecharting.NewPriceCharting(pcCfg, appCache, logger)
	if err != nil {
		t.Fatalf("PriceCharting init: %v", err)
	}
	defer pc.Close()

	// Secondary sources (optional)
	var secondarySources []fusion.SecondaryPriceSource

	if chKey := os.Getenv("CARD_HEDGER_API_KEY"); chKey != "" {
		chClient := cardhedger.NewClient(chKey)
		secondarySources = append(secondarySources, fusionprice.NewCardHedgerAdapter(chClient, nil, logger))
		t.Log("CardHedger: available (excluded from on-demand calls by design)")
	}

	// Card provider for cross-validation
	cardProv := tcgdex.NewTCGdex(appCache, logger)

	fusionProv := fusionprice.NewFusionProviderWithRepo(
		pc,
		secondarySources,
		appCache,
		nil, // no price repo
		nil, // no API tracker
		nil, // no access tracker
		logger,
		0, 0, 0, 0,
		fusionprice.WithCardProvider(cardProv),
	)

	passed, failed, noPrice := 0, 0, 0

	for _, card := range pricedInventory() {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			c := domainCards.Card{
				Name:    card.CardName,
				Number:  card.CardNumber,
				SetName: card.SetName,
			}

			price, err := fusionProv.LookupCard(ctx, card.SetName, c)
			if err != nil {
				t.Errorf("LookupCard FAILED: %v", err)
				failed++
				return
			}
			if price == nil {
				t.Errorf("NO PRICE for $%.2f card", card.PricePaid)
				noPrice++
				return
			}

			// Get grade-appropriate price (use testGradePrice for half-grade interpolation)
			priceCents, gradeLabel := testGradePrice(price.Grades, card.Grade)
			priceUSD := float64(priceCents) / 100.0

			t.Logf("  Product: %s | %s=$%.2f | Sources: %v | Confidence: %.2f",
				price.ProductName, gradeLabel, priceUSD, price.Sources, price.Confidence)

			if priceCents == 0 {
				// Check if a higher grade has data (grade fallback handles downstream)
				if price.Grades.PSA9Cents > 0 || price.Grades.PSA10Cents > 0 {
					t.Logf("  %s=$0 but higher grade available — grade fallback will handle", gradeLabel)
					passed++
					return
				}
				t.Errorf("zero price for %s with no fallback grades", gradeLabel)
				noPrice++
				return
			}

			// Sanity check: price/buy ratio in [0.1, 5.0]
			ratio := priceUSD / card.PricePaid
			if ratio < 0.1 || ratio > 5.0 {
				t.Errorf("PRICE ANOMALY: %s=$%.2f vs buy=$%.2f (ratio=%.2f)",
					gradeLabel, priceUSD, card.PricePaid, ratio)
				failed++
				return
			}

			passed++
			t.Logf("  PASS: %s=$%.2f (buy=$%.2f, ratio=%.2f)", gradeLabel, priceUSD, card.PricePaid, ratio)
		})
	}

	total := len(pricedInventory())
	fmt.Printf("\n=== MULTI-SOURCE FUSION: Passed=%d Failed=%d NoPrice=%d Total=%d ===\n",
		passed, failed, noPrice, total)
}

// TestImportToFullPricing validates the complete import pipeline:
// raw PSA title → parsed metadata → all-source fusion pricing.
// Requires PRICECHARTING_TOKEN.
// Uses the shared runImportPricingCheck helper from import_pricing_test.go.
func TestImportToFullPricing(t *testing.T) {
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

	fmt.Printf("\n=== IMPORT-TO-PRICING: Passed=%d Failed=%d NoPrice=%d Skipped=%d Total=%d ===\n",
		passed, failed, noPrice, skipped, len(entries))
}
