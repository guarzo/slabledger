package scheduler

import (
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
)

// tokenMatchesTitle checks whether a card name and an MM SearchTitle refer to the same card
// using tokenized matching. Instead of requiring the full card name as a substring of the
// search title (which fails when PSA titles like "2022 POKEMON SWORD & SHIELD BRILLIANT STARS
// CHARIZARD VSTAR" don't match MM's normalized "Charizard VSTAR 2022 Brilliant Stars PSA 10"),
// this splits both strings into tokens and checks that a sufficient proportion of significant
// card-name tokens appear in the search title.
//
// Tokens shorter than 3 characters and common noise words are ignored. The match threshold
// is 60% of significant tokens (minimum 2 matches).
func tokenMatchesTitle(cardName, searchTitle string) bool {
	titleLower := strings.ToLower(searchTitle)
	titleTokens := strings.Fields(titleLower)
	titleSet := make(map[string]bool, len(titleTokens))
	for _, t := range titleTokens {
		titleSet[t] = true
	}

	cardTokens := strings.Fields(strings.ToLower(cardName))

	var significant, matched int
	for _, tok := range cardTokens {
		if len(tok) < 3 || noiseWords[tok] {
			continue
		}
		significant++
		if titleSet[tok] {
			matched++
		}
	}

	if significant == 0 {
		// No significant tokens — fall back to plain contains.
		return strings.Contains(titleLower, strings.ToLower(cardName))
	}

	// For 1-2 significant tokens, require all to match (exact match threshold).
	// For 3+ tokens, require at least 60% to match (fuzzy matching for long PSA titles).
	if significant <= 2 {
		return matched == significant
	}
	return matched >= 2 && float64(matched)/float64(significant) >= 0.6
}

// noiseWords are common tokens in PSA listing titles that are often absent, reformatted,
// or abbreviated in MM search titles — excluded from token matching.
var noiseWords = map[string]bool{
	"pokemon": true, "pokémon": true,
	"the": true, "and": true, "for": true, "with": true,
	"holo": true, "card": true, "cards": true,
}

// computeMMSignals derives count-weighted average price, 30-day trend %, and total
// sales volume from a slice of daily stats items.
// trendPct is (lastDayWithSales.AvgPrice - firstDayWithSales.AvgPrice) / firstDayWithSales.AvgPrice,
// capped to ±200% to resist single-sale outlier days.
func computeMMSignals(items []marketmovers.DailyStatItem) (avgPrice float64, trendPct float64, sales30d int) {
	if len(items) == 0 {
		return 0, 0, 0
	}

	var totalAmount float64
	var totalCount int
	var firstPrice, lastPrice float64

	for _, item := range items {
		if item.TotalSalesCount <= 0 {
			continue
		}
		totalAmount += item.TotalSalesAmount
		totalCount += item.TotalSalesCount
		if firstPrice == 0 {
			firstPrice = item.AverageSalePrice
		}
		lastPrice = item.AverageSalePrice
	}

	if totalCount == 0 {
		return 0, 0, 0
	}

	avgPrice = totalAmount / float64(totalCount)
	sales30d = totalCount

	if firstPrice > 0 {
		raw := (lastPrice - firstPrice) / firstPrice
		// Cap to ±200% to avoid outlier distortion from single-sale days
		trendPct = max(-2.0, min(2.0, raw))
	}

	return avgPrice, trendPct, sales30d
}
