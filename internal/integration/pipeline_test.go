// Pipeline integration tests: normalization audit, multi-source fusion,
// and end-to-end import-to-pricing.
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

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/adapters/clients/fusionprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"

	_ "github.com/joho/godotenv/autoload"
)

// TestNormalizationAudit verifies what each pricing source sees after
// normalization — no API calls required. Logs normalized forms side by side for
// manual review.
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
			ppSet := cardutil.NormalizeSetNameForSearch(card.SetName)
			t.Logf("Secondary: name=%q set=%q", ppName, ppSet)

			if strings.Contains(ppName, "-HOLO") || strings.Contains(ppName, "-REV.FOIL") {
				t.Errorf("name has unexpanded abbreviation: %s", ppName)
			}
			if strings.HasPrefix(strings.ToUpper(ppSet), "JAPANESE ") {
				t.Logf("  NOTE: set preserves JAPANESE prefix (NormalizeSetNameForSearch retains it by design): %s", ppSet)
			}
			for _, code := range []string{"PRE EN", "M24 EN", "MEW EN", "SVP EN"} {
				if strings.Contains(strings.ToUpper(ppSet), code) {
					t.Errorf("set contains PSA code %q: %s", code, ppSet)
				}
			}

			// --- Set overlap sanity check ---
			selfMatch := cardutil.MatchesSetOverlap(card.SetName, card.SetName)
			t.Logf("SetOverlap: self-match=%v set=%q", selfMatch, card.SetName)
			if !selfMatch {
				t.Errorf("MatchesSetOverlap self-match failed for %q", card.SetName)
			}

			// --- Cross-source comparison ---
			t.Logf("  Summary: PC=%q | secondary=%q/%q | set=%q",
				pcQuery, ppName, ppSet, card.SetName)
		})
	}
}

// TestFullMultiSourceFusion exercises available price sources through the
// fusion engine. Requires PRICECHARTING_TOKEN.
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

	// Card provider for cross-validation
	cardProv := tcgdex.NewTCGdex(appCache, logger)

	fusionProv := fusionprice.NewFusionProviderWithRepo(
		pc,
		nil, // no secondary sources
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
