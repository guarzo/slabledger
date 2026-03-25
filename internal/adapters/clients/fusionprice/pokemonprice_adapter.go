package fusionprice

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/pokemonprice"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PokemonPriceAdapter wraps a pokemonprice.Client and implements SecondaryPriceSource.
type PokemonPriceAdapter struct {
	client *pokemonprice.Client
	logger observability.Logger
}

// PPOption configures a PokemonPriceAdapter.
type PPOption func(*PokemonPriceAdapter)

// WithPPLogger sets the logger for API response validation warnings.
func WithPPLogger(logger observability.Logger) PPOption {
	return func(a *PokemonPriceAdapter) { a.logger = logger }
}

// NewPokemonPriceAdapter creates a new adapter wrapping a pokemonprice.Client.
func NewPokemonPriceAdapter(client *pokemonprice.Client, opts ...PPOption) *PokemonPriceAdapter {
	a := &PokemonPriceAdapter{client: client}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// FetchFusionData fetches price data from PokemonPriceTracker and converts to fusion format.
// Uses includeEbay=true to get per-grade eBay sales data (costs 2 API credits).
// All detail data (eBay grades, velocity) is returned in the FetchResult, avoiding shared mutable state.
func (a *PokemonPriceAdapter) FetchFusionData(ctx context.Context, card pricing.Card) (*fusion.FetchResult, *fusion.ResponseMeta, error) {
	if a.client == nil {
		return nil, buildResponseMeta(0, nil), fmt.Errorf("pokemonprice: client not configured")
	}

	ppData, statusCode, headers, err := a.client.GetPriceWithGraded(ctx, card.Set, card.Name, card.Number)
	if err != nil {
		return nil, buildResponseMeta(statusCode, headers), err
	}

	warnUnknownPPGrades(ctx, a.logger, ppData)
	warnUnknownPPConfidence(ctx, a.logger, ppData)
	fusionData, ebayDetails, velocity := convertPokemonPriceWithDetails(ppData)

	return &fusion.FetchResult{
		GradeData:   fusionData,
		EbayDetails: ebayDetails,
		Velocity:    velocity,
	}, buildResponseMeta(statusCode, headers), nil
}

// Available returns true if the underlying client is configured.
func (a *PokemonPriceAdapter) Available() bool {
	return a.client != nil && a.client.Available()
}

// Name returns the source identifier.
func (a *PokemonPriceAdapter) Name() string {
	return "pokemonprice"
}

// psaGradeKeys maps PokemonPrice API grade keys (e.g. "psa10", "ungraded")
// to domain Grade values. Includes PSA grades + cross-grading keys that have
// a direct Grade equivalent (e.g., "bgs10" → GradeBGS10).
var psaGradeKeys = func() map[string]pricing.Grade {
	m := make(map[string]pricing.Grade, len(pricing.AllDisplayGrades)+4)
	for _, g := range pricing.AllDisplayGrades {
		if g == pricing.GradeRaw {
			continue // handled separately as "ungraded"
		}
		m[g.String()] = g
	}
	m["ungraded"] = pricing.GradeRaw
	// Cross-grading keys with direct Grade equivalents
	m["bgs10"] = pricing.GradeBGS10
	m["bgs95"] = pricing.GradePSA95
	m["cgc95"] = pricing.GradePSA95
	return m
}()

// extraPPGradeKeys lists non-PSA grade keys that PokemonPrice returns for
// other grading companies. These are recognized as valid API responses but
// have no fusion Grade mapping (except where psaGradeKeys already covers them).
var extraPPGradeKeys = map[string]bool{
	"cgc10": true, "cgc9": true, "cgc8": true, "cgc7": true, "cgc6": true,
	"bgs9": true, "bgs8": true, "bgs7": true, "bgs6": true,
	"ags10": true, "ags9": true, "ags8": true,
	"tag10": true, "tag9": true, "tag8": true,
}

// confidenceToScore maps PokemonPriceTracker confidence strings to numeric scores.
func confidenceToScore(confidence string) float64 {
	switch confidence {
	case "high":
		return 0.95
	case "medium":
		return 0.80
	case "low":
		return 0.60
	default:
		return 0.50
	}
}

// convertPokemonPriceWithDetails converts PokemonPriceTracker data to fusion.PriceData
// and also extracts per-grade eBay detail and sales velocity.
func convertPokemonPriceWithDetails(ppData *pokemonprice.CardPriceData) (map[string][]fusion.PriceData, map[string]*pricing.EbayGradeDetail, *pricing.SalesVelocity) {
	if ppData == nil {
		return make(map[string][]fusion.PriceData), make(map[string]*pricing.EbayGradeDetail), nil
	}

	result := make(map[string][]fusion.PriceData)
	ebayDetails := make(map[string]*pricing.EbayGradeDetail)
	var velocity *pricing.SalesVelocity

	// Raw card price from TCGPlayer market data (always available)
	if ppData.Prices.Market > 0 {
		result["raw"] = []fusion.PriceData{
			{
				Value:    ppData.Prices.Market,
				Currency: "USD",
				Source: fusion.DataSource{
					Name:       "pokemonprice",
					Freshness:  time.Since(ppData.UpdatedAt),
					Volume:     ppData.Prices.Sellers,
					Confidence: 0.87,
				},
			},
		}
	}

	// eBay graded data (available when includeEbay=true was used)
	if ppData.Ebay == nil {
		return result, ebayDetails, velocity
	}

	// Extract sales velocity
	sv := ppData.Ebay.SalesVelocity
	if sv.DailyAverage > 0 || sv.MonthlyTotal > 0 {
		velocity = &pricing.SalesVelocity{
			DailyAverage:  sv.DailyAverage,
			WeeklyAverage: sv.WeeklyAverage,
			MonthlyTotal:  sv.MonthlyTotal,
		}
	}

	for apiGrade, salesData := range ppData.Ebay.SalesByGrade {
		grade, ok := psaGradeKeys[apiGrade]
		if !ok {
			continue
		}

		if salesData.SmartMarketPrice.Price <= 0 {
			continue
		}

		fusionKey := grade.String()

		// Build eBay grade detail (prices converted to cents)
		detail := &pricing.EbayGradeDetail{
			PriceCents:  mathutil.ToCents(salesData.SmartMarketPrice.Price),
			Confidence:  salesData.SmartMarketPrice.Confidence,
			SalesCount:  salesData.Count,
			MedianCents: mathutil.ToCents(salesData.MedianPrice),
			MinCents:    mathutil.ToCents(salesData.MinPrice),
			MaxCents:    mathutil.ToCents(salesData.MaxPrice),
		}
		if salesData.MarketTrend != nil {
			detail.Trend = *salesData.MarketTrend
		}
		if salesData.MarketPrice7Day != nil {
			detail.Avg7DayCents = mathutil.ToCents(*salesData.MarketPrice7Day)
		}
		if salesData.DailyVolume7Day != nil {
			detail.Volume7Day = *salesData.DailyVolume7Day
		}
		ebayDetails[fusionKey] = detail

		// Build fusion price data
		pd := fusion.PriceData{
			Value:    salesData.SmartMarketPrice.Price,
			Currency: "USD",
			Source: fusion.DataSource{
				Name:       "pokemonprice",
				Freshness:  time.Since(ppData.Ebay.UpdatedAt),
				Volume:     salesData.Count,
				Confidence: confidenceToScore(salesData.SmartMarketPrice.Confidence),
			},
		}
		result[fusionKey] = []fusion.PriceData{pd}
	}

	return result, ebayDetails, velocity
}
