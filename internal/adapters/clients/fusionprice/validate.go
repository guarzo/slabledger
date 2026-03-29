package fusionprice

import (
	"context"
	"regexp"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

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
