package fusionprice

import (
	"context"
	"regexp"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/adapters/clients/pokemonprice"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// warnUnknownPPGrades logs a warning if a PokemonPrice response contains grade
// keys not in psaGradeKeys or extraPPGradeKeys. This signals a new grade format
// or API change that could cause silent data loss in the fusion pipeline.
func warnUnknownPPGrades(ctx context.Context, logger observability.Logger, ppData *pokemonprice.CardPriceData) {
	if logger == nil || ppData == nil || ppData.Ebay == nil {
		return
	}
	for apiGrade := range ppData.Ebay.SalesByGrade {
		if _, ok := psaGradeKeys[apiGrade]; !ok && !extraPPGradeKeys[apiGrade] {
			logger.Warn(ctx, "pokemonprice: unknown grade key in API response",
				observability.String("grade_key", apiGrade))
		}
	}
}

// warnUnknownCHGrades logs a warning if a CardHedger response contains grade
// strings not recognized by pricing.IsCardHedgerGrade (PSA 1-10 + Raw).
// This signals an API format change that could cause silent data loss.
func warnUnknownCHGrades(ctx context.Context, logger observability.Logger, resp *cardhedger.AllPricesByCardResponse) {
	if logger == nil || resp == nil {
		return
	}
	for _, gp := range resp.Prices {
		if !pricing.IsCardHedgerGrade(gp.Grade) {
			logger.Warn(ctx, "cardhedger: unknown grade key in API response",
				observability.String("grade_key", gp.Grade))
		}
	}
}

// warnUnknownPPConfidence logs a warning if a PokemonPrice response contains
// confidence strings not in the expected set (high/medium/low).
// knownPPConfidenceValues is the set of expected confidence strings from PokemonPrice.
var knownPPConfidenceValues = map[string]bool{"high": true, "medium": true, "low": true, "": true}

func warnUnknownPPConfidence(ctx context.Context, logger observability.Logger, ppData *pokemonprice.CardPriceData) {
	if logger == nil || ppData == nil || ppData.Ebay == nil {
		return
	}
	for _, salesData := range ppData.Ebay.SalesByGrade {
		conf := salesData.SmartMarketPrice.Confidence
		if !knownPPConfidenceValues[conf] {
			logger.Warn(ctx, "pokemonprice: unknown confidence value in API response",
				observability.String("confidence", conf))
			return // log once per response, not per grade
		}
	}
}

// eraLikePrefix matches 2-4 uppercase letters optionally followed by a digit,
// used to detect potential new TCG era prefixes not yet in KnownEraTokens.
var eraLikePrefix = regexp.MustCompile(`^[A-Z]{2,4}\d?$`)

// warnUnknownEraPrefix logs a warning if a promo set name contains a token that
// looks like an era prefix but isn't in KnownEraTokens. Only triggers for promo sets.
func warnUnknownEraPrefix(ctx context.Context, logger observability.Logger, setName string) {
	if logger == nil || !strings.Contains(strings.ToLower(setName), "promo") {
		return
	}
	for _, token := range strings.Fields(setName) {
		upper := strings.ToUpper(token)
		if eraLikePrefix.MatchString(upper) && !cardutil.KnownEraTokens[upper] {
			logger.Info(ctx, "potential unknown era prefix in promo set",
				observability.String("token", upper),
				observability.String("set", setName))
			return // log once per set
		}
	}
}
