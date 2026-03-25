package fusionprice

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// CardHedgerAdapter wraps a cardhedger.Client and implements SecondaryPriceSource.
// It resolves card names to CardHedger IDs via a cached mapping, then fetches
// price estimates for each grade.
type CardHedgerAdapter struct {
	client       *cardhedger.Client
	resolver     fusion.CardIDResolver
	hintResolver fusion.PriceHintResolver
	logger       observability.Logger
}

// NewCardHedgerAdapter creates a new adapter.
// The resolver caches card_name+set_name → CardHedger card_id mappings.
func NewCardHedgerAdapter(client *cardhedger.Client, resolver fusion.CardIDResolver, logger observability.Logger, opts ...CardHedgerAdapterOption) *CardHedgerAdapter {
	a := &CardHedgerAdapter{client: client, resolver: resolver, logger: logger}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// CardHedgerAdapterOption is a functional option for CardHedgerAdapter.
type CardHedgerAdapterOption func(*CardHedgerAdapter)

// WithCardHedgerHintResolver sets a PriceHintResolver for user-provided hints.
func WithCardHedgerHintResolver(r fusion.PriceHintResolver) CardHedgerAdapterOption {
	return func(a *CardHedgerAdapter) {
		a.hintResolver = r
	}
}

// FetchFusionData fetches price estimates from CardHedger and converts to fusion format.
// All detail data (estimate grades) is returned in the FetchResult, avoiding shared mutable state.
func (a *CardHedgerAdapter) FetchFusionData(ctx context.Context, card pricing.Card) (*fusion.FetchResult, *fusion.ResponseMeta, error) {
	if a.client == nil {
		return nil, buildResponseMeta(0, nil), fmt.Errorf("cardhedger: client not configured")
	}

	cardName, setName, cardNumber := card.Name, card.Set, card.Number

	// Inject normalization trace for debugging the query construction chain.
	// Reuse existing trace if the caller already set one up on the context.
	if cardutil.TraceFromContext(ctx) == nil {
		ctx = cardutil.ContextWithTrace(ctx)
	}
	defer a.logTrace(ctx, cardName, setName)

	cardID, statusCode, headers, err := a.resolveCardID(ctx, cardName, setName, cardNumber, card.PSAListingTitle)
	if err != nil {
		return nil, buildResponseMeta(statusCode, headers), err
	}

	resp, statusCode, headers, err := a.client.GetAllPrices(ctx, cardID)
	if err != nil {
		return nil, buildResponseMeta(statusCode, headers), err
	}

	warnUnknownCHGrades(ctx, a.logger, resp)
	fusionData, estimateDetails := convertCardHedgerWithDetails(resp)

	// Fetch batch estimates for UI grades (1 API call for core grades)
	items := make([]cardhedger.PriceEstimateItem, 0, len(pricing.CoreGrades))
	for _, g := range pricing.CoreGrades {
		items = append(items, cardhedger.PriceEstimateItem{CardID: cardID, Grade: g.DisplayLabel()})
	}
	batchResp, _, _, batchErr := a.client.BatchPriceEstimate(ctx, items)
	if batchErr != nil {
		if a.logger != nil {
			a.logger.Warn(ctx, "cardhedger batch estimate failed",
				observability.String("card_id", cardID),
				observability.Err(batchErr))
		}
	} else if batchResp != nil {
		for _, r := range batchResp.Results {
			if r.Error != nil || r.Price == nil {
				continue
			}
			fusionKey, ok := cardHedgerGradeToFusion(r.Grade)
			if !ok {
				if a.logger != nil {
					a.logger.Warn(ctx, "cardhedger: unknown grade key in batch estimate response",
						observability.String("grade_key", r.Grade),
						observability.String("card_id", cardID))
				}
				continue
			}
			detail := &pricing.EstimateGradeDetail{
				PriceCents: mathutil.ToCents(*r.Price),
				Confidence: 0.85,
			}
			if r.PriceLow != nil {
				detail.LowCents = mathutil.ToCents(*r.PriceLow)
			}
			if r.PriceHigh != nil {
				detail.HighCents = mathutil.ToCents(*r.PriceHigh)
			}
			if r.Confidence != nil {
				detail.Confidence = *r.Confidence
			}
			estimateDetails[fusionKey] = detail
		}
	}

	return &fusion.FetchResult{
		GradeData:       fusionData,
		EstimateDetails: estimateDetails,
	}, buildResponseMeta(statusCode, headers), nil
}

// Available returns true if the underlying client is configured.
func (a *CardHedgerAdapter) Available() bool {
	return a.client != nil && a.client.Available()
}

// Name returns the source identifier.
func (a *CardHedgerAdapter) Name() string {
	return "cardhedger"
}

// cardHedgerMappingMaxAge is the maximum age for cached CardHedger ID mappings.
// Mappings older than this are re-resolved via card-match to avoid stale IDs.
const cardHedgerMappingMaxAge = 30 * 24 * time.Hour // 30 days

// resolveCardID looks up the CardHedger card_id from the mapping cache,
// falling back to the card-match API endpoint if not found or stale.
// Uses confidence thresholds to decide whether to cache the mapping.
//
// Query fallback chain (up to 3 API calls):
//  1. Full normalized query: NormalizeSetNameForSearch(set) + SimplifyForSearch(name) + number
//  2. Minimal query: truncateAtVariant(name) + eraPrefix + number
//  3. Raw PSA listing title (stripped of grade suffix) — lets CardHedger's LLM parse it
func (a *CardHedgerAdapter) resolveCardID(ctx context.Context, cardName, setName, cardNumber, psaTitle string) (string, int, http.Header, error) {
	trace := cardutil.TraceFromContext(ctx)
	normalizedNumber := cardutil.NormalizeCardNumber(cardNumber)

	// Fast path: check hint and cache before making API calls.
	if id, found := a.resolveFromHintOrCache(ctx, cardName, setName, normalizedNumber); found {
		return id, 0, nil, nil
	}

	// Step 1: Full normalized query (set + simplified name + number)
	query := cardutil.BuildCardMatchQuery(setName, cardName, cardNumber)
	trace.AddStep("BuildCardMatchQuery", fmt.Sprintf("set=%q name=%q num=%q", setName, cardName, cardNumber), query)

	lastQuery := query
	resp, statusCode, headers, err := a.client.CardMatch(ctx, lastQuery, "Pokemon", 10)
	if err != nil {
		return "", statusCode, headers, fmt.Errorf("cardhedger card-match failed for %q / %q (query=%q): %w", cardName, setName, lastQuery, err)
	}
	if resp == nil {
		return "", statusCode, headers, fmt.Errorf("cardhedger: nil response for %q / %q", cardName, setName)
	}

	// Step 2: Minimal query — truncateAtVariant(name) + optional eraPrefix + number
	// Runs for numberless cards too (e.g., Ancient Mew) where the core name alone
	// can produce a better match than the full normalized query.
	// Also retries when the initial match is low-confidence, since a simpler query
	// may produce a higher-confidence result from CardHedger's matching engine.
	noMatch := resp.Match == nil || resp.Match.CardID == ""
	lowConfidence := !noMatch && cardhedger.ShouldRejectMatch(resp.Match.Confidence)
	if noMatch || lowConfidence {
		simplifiedName := cardutil.SimplifyForSearch(cardutil.NormalizePurchaseName(cardName))
		coreName := truncateAtVariant(simplifiedName)
		fbNumber := normalizedNumber
		if fbNumber != "" && !hasLetterPrefix(fbNumber) {
			if prefix := cardutil.ExtractEraPrefix(setName); prefix != "" {
				fbNumber = prefix + fbNumber
			} else if strings.Contains(strings.ToLower(setName), "promo") {
				warnUnknownEraPrefix(ctx, a.logger, setName)
			}
		}
		if fbNumber != "" {
			lastQuery = coreName + " " + fbNumber
		} else {
			lastQuery = coreName
		}
		trace.AddStep("resolveCardID:fallback", query, lastQuery)
		if a.logger != nil {
			a.logger.Debug(ctx, "cardhedger: retrying with minimal query",
				observability.String("original_query", query),
				observability.String("fallback_query", lastQuery))
		}
		resp, statusCode, headers, err = a.client.CardMatch(ctx, lastQuery, "Pokemon", 10)
		if err != nil {
			return "", statusCode, headers, fmt.Errorf("cardhedger card-match fallback failed for %q / %q (query=%q): %w", cardName, setName, lastQuery, err)
		}
		if resp == nil {
			return "", statusCode, headers, fmt.Errorf("cardhedger: nil fallback response for %q / %q", cardName, setName)
		}
	}

	// Step 3: Raw PSA listing title — lets CardHedger's LLM parse natural language
	noMatch = resp.Match == nil || resp.Match.CardID == ""
	lowConfidence = !noMatch && cardhedger.ShouldRejectMatch(resp.Match.Confidence)
	if (noMatch || lowConfidence) && psaTitle != "" {
		cleanTitle := cardutil.PSAGradeSuffixRegex.ReplaceAllString(psaTitle, "")
		cleanTitle = strings.TrimSpace(cleanTitle)
		trace.AddStep("resolveCardID:psaTitle", psaTitle, cleanTitle)
		if cleanTitle != "" {
			lastQuery = cleanTitle
			if a.logger != nil {
				a.logger.Debug(ctx, "cardhedger: retrying with raw PSA title",
					observability.String("psa_title", lastQuery))
			}
			resp, statusCode, headers, err = a.client.CardMatch(ctx, lastQuery, "Pokemon", 10)
			if err != nil {
				return "", statusCode, headers, fmt.Errorf("cardhedger card-match PSA title failed for %q / %q (query=%q): %w", cardName, setName, lastQuery, err)
			}
			if resp == nil {
				return "", statusCode, headers, fmt.Errorf("cardhedger: nil PSA title response for %q / %q", cardName, setName)
			}
		}
	}

	return a.evaluateAndCacheResult(ctx, resp, cardName, setName, normalizedNumber, lastQuery, statusCode, headers)
}

// resolveFromHintOrCache checks user-provided hints and the mapping cache
// before making any API calls. Returns (id, true) if found, ("", false) otherwise.
func (a *CardHedgerAdapter) resolveFromHintOrCache(ctx context.Context, cardName, setName, normalizedNumber string) (string, bool) {
	if a.hintResolver != nil {
		hint, err := a.hintResolver.GetHint(ctx, cardName, setName, normalizedNumber, "cardhedger")
		if err != nil {
			if a.logger != nil {
				a.logger.Debug(ctx, "hint resolution failed",
					observability.String("card", cardName),
					observability.String("set", setName),
					observability.Err(err))
			}
		} else if hint != "" {
			return hint, true
		}
	}

	if a.resolver != nil {
		cached, err := a.resolver.GetExternalIDFresh(ctx, cardName, setName, normalizedNumber, "cardhedger", cardHedgerMappingMaxAge)
		if err != nil {
			if a.logger != nil {
				a.logger.Warn(ctx, "card ID mapping lookup failed",
					observability.String("card", cardName),
					observability.String("set", setName),
					observability.Err(err))
			}
		} else if cached != "" {
			return cached, true
		}
	}

	return "", false
}

// evaluateAndCacheResult checks the final card-match response, applies
// confidence thresholds, and caches high-confidence matches.
func (a *CardHedgerAdapter) evaluateAndCacheResult(
	ctx context.Context,
	resp *cardhedger.CardMatchResponse,
	cardName, setName, normalizedNumber, lastQuery string,
	statusCode int, headers http.Header,
) (string, int, http.Header, error) {
	trace := cardutil.TraceFromContext(ctx)

	if resp.Match == nil || resp.Match.CardID == "" {
		trace.AddStep("resolveCardID:result", lastQuery, "no match")
		return "", statusCode, headers, fmt.Errorf("cardhedger: no match for %q / %q (query=%q)", cardName, setName, lastQuery)
	}

	confidence := resp.Match.Confidence
	trace.AddStep("resolveCardID:result", lastQuery, fmt.Sprintf("id=%s conf=%.2f", resp.Match.CardID, confidence))
	if a.logger != nil {
		a.logger.Info(ctx, "cardhedger card-match result",
			observability.String("card", cardName),
			observability.String("set", setName),
			observability.String("card_id", resp.Match.CardID),
			observability.Float64("confidence", confidence))
	}

	if cardhedger.ShouldRejectMatch(confidence) {
		return "", statusCode, headers, fmt.Errorf("cardhedger: match confidence too low (%.2f) for %q / %q", confidence, cardName, setName)
	}

	externalID := resp.Match.CardID

	if cardhedger.ShouldCacheMatch(confidence) && a.resolver != nil {
		if err := a.resolver.SaveExternalID(ctx, cardName, setName, normalizedNumber, "cardhedger", externalID); err != nil {
			if a.logger != nil {
				a.logger.Warn(ctx, "failed to cache card ID mapping",
					observability.String("card", cardName),
					observability.String("external_id", externalID),
					observability.Err(err))
			}
		}
	}

	return externalID, statusCode, headers, nil
}

// logTrace emits the normalization trace at Debug level if one exists on the context.
func (a *CardHedgerAdapter) logTrace(ctx context.Context, cardName, setName string) {
	cardutil.LogNormalizationTrace(ctx, a.logger, cardName, setName)
}

// hasLetterPrefix returns true if the string starts with a letter (e.g., "SM162", "SWSH029").
func hasLetterPrefix(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// variantWords are keywords that mark the boundary between a Pokémon's name
// and variant descriptors (Holo, Reverse, Foil). Used by truncateAtVariant.
var variantWords = map[string]bool{
	"holo": true, "reverse": true, "foil": true,
}

// truncateAtVariant extracts the core Pokémon name by truncating at variant
// keywords like "Holo", "Reverse", "Foil". Keeps multi-word names intact.
// e.g., "PIKACHU Holo TM.UP SNGL.PK.BLST." → "PIKACHU"
//
//	"DARK GYARADOS Holo 1ST EDITION" → "DARK GYARADOS"
//	"SYLVEON ex" → "SYLVEON ex" (type suffixes preserved)
func truncateAtVariant(name string) string {
	words := strings.Fields(name)
	for i, w := range words {
		if variantWords[strings.ToLower(w)] && i > 0 {
			return strings.Join(words[:i], " ")
		}
	}
	return name
}

// cardHedgerGradeToFusion converts a CardHedger display-format grade string
// (e.g., "PSA 10", "Raw") to the fusion key string (e.g., "psa10", "raw").
// Returns the fusion key and true if the grade maps to a known Grade value.
// Non-PSA grades that CardHedger returns (CGC, AGS, TAG, etc.) are silently
// skipped when they have no mapping in displayToGrade.
func cardHedgerGradeToFusion(displayGrade string) (string, bool) {
	g := pricing.GradeFromDisplay(displayGrade)
	if g == pricing.GradeUnknown {
		return "", false
	}
	return g.String(), true
}

// convertCardHedgerWithDetails converts CardHedger all-prices response to fusion format
// and also extracts per-grade estimate details.
func convertCardHedgerWithDetails(resp *cardhedger.AllPricesByCardResponse) (map[string][]fusion.PriceData, map[string]*pricing.EstimateGradeDetail) {
	result := make(map[string][]fusion.PriceData)
	estimates := make(map[string]*pricing.EstimateGradeDetail)

	for _, gp := range resp.Prices {
		fusionKey, ok := cardHedgerGradeToFusion(gp.Grade)
		if !ok {
			continue
		}

		price, err := strconv.ParseFloat(gp.Price, 64)
		if err != nil || price <= 0 {
			continue
		}

		result[fusionKey] = []fusion.PriceData{
			{
				Value:    price,
				Currency: "USD",
				Source: fusion.DataSource{
					Name:       "cardhedger",
					Freshness:  0,
					Volume:     0,
					Confidence: 0.85,
				},
			},
		}

		// Store as estimate detail (confidence range filled by batch estimate if available)
		estimates[fusionKey] = &pricing.EstimateGradeDetail{
			PriceCents: mathutil.ToCents(price),
			Confidence: 0.85,
		}
	}

	return result, estimates
}
