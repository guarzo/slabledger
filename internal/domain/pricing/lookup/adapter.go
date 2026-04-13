// Package lookup adapts a PriceProvider to implement inventory.PriceLookup.
// Grade interpolation, fallback logic, and source-price assembly live here
// because they are domain computations on pricing data, not external API concerns.
package lookup

import (
	"context"
	"fmt"
	"math"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

const gradeEpsilon = 1e-9

// validGrade reports whether grade is a known grade value.
// Grade 0 represents raw/ungraded cards.
// Accepts whole grades 6-10 and half-grades (e.g. 6.5, 7.5, 8.5, 9.5).
// Grades below 6 are rejected because the pricing pipeline only tracks
// PSA 6+ and Raw — lower grades would silently fall through to Raw pricing.
func validGrade(grade float64) bool {
	if grade == 0 {
		return true
	}
	if grade < 6 || grade > 10 {
		return false
	}
	// Accept whole grades and .5 half-grades
	frac := grade - math.Floor(grade)
	return math.Abs(frac) < gradeEpsilon || math.Abs(frac-0.5) < gradeEpsilon
}

// isHalfGrade returns true for grades like 8.5, 9.5.
func isHalfGrade(grade float64) bool {
	frac := grade - math.Floor(grade)
	return math.Abs(frac-0.5) < gradeEpsilon
}

// Adapter wraps a PriceProvider to implement inventory.PriceLookup.
type Adapter struct {
	provider pricing.PriceProvider
}

var _ inventory.PriceLookup = (*Adapter)(nil)

// NewAdapter creates a PriceLookup adapter around a PriceProvider.
func NewAdapter(provider pricing.PriceProvider) *Adapter {
	return &Adapter{provider: provider}
}

// GetLastSoldCents returns the last sold price in cents for a card at a given grade.
func (a *Adapter) GetLastSoldCents(ctx context.Context, card inventory.CardIdentity, grade float64) (int, error) {
	if !validGrade(grade) {
		return 0, fmt.Errorf("unsupported grade: %g", grade)
	}
	price, err := a.getPrice(ctx, card)
	if err != nil {
		return 0, err
	}
	if price == nil {
		return 0, nil
	}

	if price.LastSoldByGrade == nil {
		return 0, nil
	}

	// For half-grades, use the floor grade's last sold data
	info := gradeInfo(price.LastSoldByGrade, grade)
	if info == nil || info.LastSoldPrice <= 0 {
		return 0, nil
	}
	return int(info.LastSoldPrice), nil
}

// GetMarketSnapshot returns a comprehensive market snapshot for a card at a given grade.
func (a *Adapter) GetMarketSnapshot(ctx context.Context, card inventory.CardIdentity, grade float64) (*inventory.MarketSnapshot, error) {
	if !validGrade(grade) {
		return nil, fmt.Errorf("unsupported grade: %g", grade)
	}
	price, err := a.getPrice(ctx, card)
	if err != nil {
		return nil, err
	}
	if price == nil {
		return nil, nil
	}

	snap := &inventory.MarketSnapshot{}

	// Last sold data from actual sales records.
	if price.LastSoldByGrade != nil {
		if info := gradeInfo(price.LastSoldByGrade, grade); info != nil {
			snap.LastSoldCents = int(info.LastSoldPrice)
			snap.LastSoldDate = info.LastSoldDate
			snap.SaleCount = info.SaleCount
		}
	}
	// Fallback: eBay smartMarketPrice (median of recent eBay sales)
	if snap.LastSoldCents == 0 && price.GradeDetails != nil {
		key := gradeDetailKey(grade)
		if detail, ok := price.GradeDetails[key]; ok && detail != nil && detail.Ebay != nil && detail.Ebay.PriceCents > 0 {
			snap.LastSoldCents = int(detail.Ebay.PriceCents)
			snap.SaleCount = detail.Ebay.SalesCount
		}
	}
	// 3. Price estimate (stored separately to distinguish from actual sales)
	if price.GradeDetails != nil {
		key := gradeDetailKey(grade)
		if detail, ok := price.GradeDetails[key]; ok && detail != nil && detail.Estimate != nil && detail.Estimate.PriceCents > 0 {
			snap.EstimatedValueCents = int(detail.Estimate.PriceCents)
			snap.EstimateSource = pricing.SourceDH
		}
	}

	// Grade-specific price
	snap.GradePriceCents = gradePrice(price.Grades, grade)

	// Mark half-grade results as estimated since we interpolate
	if isHalfGrade(grade) && snap.GradePriceCents > 0 {
		snap.IsEstimated = true
	}

	// Grade fallback: when a specific grade has no data, estimate from an adjacent grade.
	// PSA 8 → use PSA 9 at 65% discount; PSA 9 → use PSA 10 at 65%.
	if snap.GradePriceCents == 0 {
		fallbackGrade := math.Ceil(grade) + 1
		if isHalfGrade(grade) {
			fallbackGrade = math.Ceil(grade)
		}
		if fallbackGrade <= 10 {
			pUp := gradePrice(price.Grades, fallbackGrade)
			if pUp > 0 {
				snap.GradePriceCents = int(math.Round(float64(pUp) * 0.65))
				snap.IsEstimated = true
			}
		}
	}

	// Market data
	if price.Market != nil {
		snap.LowestListCents = int(price.Market.LowestListing)
		snap.ActiveListings = price.Market.ActiveListings
		snap.SalesLast30d = price.Market.SalesLast30d
		snap.SalesLast90d = price.Market.SalesLast90d
		snap.Volatility = price.Market.Volatility
	}

	// Sales velocity
	if price.Velocity != nil {
		snap.DailyVelocity = price.Velocity.DailyAverage
		snap.WeeklyVelocity = price.Velocity.WeeklyAverage
		snap.MonthlyVelocity = price.Velocity.MonthlyTotal
	}

	// Confidence and source count
	snap.Confidence = price.Confidence
	if len(price.Sources) > 0 {
		snap.Sources = price.Sources
		snap.SourceCount = len(price.Sources)
	}

	// Conservative/distribution data.
	// PSA 8 has no direct data in the provider API; the fallback logic at lines below
	// (conservative = 0.85 × median) applies instead.
	if price.Conservative != nil {
		floorGrade := math.Floor(grade)
		switch floorGrade {
		case 10:
			snap.ConservativeCents = int(mathutil.ToCents(price.Conservative.PSA10USD))
		case 9:
			snap.ConservativeCents = int(mathutil.ToCents(price.Conservative.PSA9USD))
		default:
			if grade == 0 {
				snap.ConservativeCents = int(mathutil.ToCents(price.Conservative.RawUSD))
			}
		}
	}

	// Distributions (percentile data).
	// PSA 8 has no direct distribution data; the fallback at lines below applies.
	if price.Distributions != nil {
		var dist *pricing.SalesDistribution
		floorGrade := math.Floor(grade)
		switch floorGrade {
		case 10:
			dist = price.Distributions.PSA10
		case 9:
			dist = price.Distributions.PSA9
		default:
			if grade == 0 {
				dist = price.Distributions.Raw
			}
		}
		if dist != nil {
			snap.P10Cents = int(mathutil.ToCents(dist.P10))
			snap.MedianCents = int(mathutil.ToCents(dist.P50))
			snap.OptimisticCents = int(mathutil.ToCents(dist.P75))
			snap.P90Cents = int(mathutil.ToCents(dist.P90))
			snap.DistSampleSize = dist.SampleSize
			snap.DistPeriodDays = dist.Period
			if snap.ConservativeCents == 0 {
				snap.ConservativeCents = int(mathutil.ToCents(dist.P25))
			}
		}
	}

	// Fallbacks: if median is 0 but we have grade price, use that
	if snap.MedianCents == 0 && snap.GradePriceCents > 0 {
		snap.MedianCents = snap.GradePriceCents
	}
	if snap.ConservativeCents == 0 && snap.MedianCents > 0 {
		snap.ConservativeCents = int(math.Round(float64(snap.MedianCents) * 0.85))
	}
	if snap.OptimisticCents == 0 && snap.MedianCents > 0 {
		snap.OptimisticCents = int(math.Round(float64(snap.MedianCents) * 1.15))
	}
	if snap.P10Cents == 0 && snap.MedianCents > 0 {
		snap.P10Cents = int(math.Round(float64(snap.MedianCents) * 0.70))
	}
	if snap.P90Cents == 0 && snap.MedianCents > 0 {
		snap.P90Cents = int(math.Round(float64(snap.MedianCents) * 1.30))
	}

	// Log when a non-nil price produces zero median — helps debug silent pricing failures
	if snap.MedianCents == 0 && snap.GradePriceCents == 0 && snap.LastSoldCents == 0 {
		// All core price fields are zero despite having a non-nil price object.
		// This indicates the price data didn't contain usable grades for this grade level.
		snap.PricingGap = true
	}

	// Per-source pricing data (includes 7-day avg)
	snap.SourcePrices = buildSourcePrices(price, grade)

	// Extract 7-day avg from source prices if available
	for _, sp := range snap.SourcePrices {
		if sp.Avg7DayCents > 0 {
			snap.Avg7DayCents = sp.Avg7DayCents
			break
		}
	}

	return snap, nil
}

// getPrice fetches the price for a card from the underlying PriceProvider.
// Strategy: looks up by card name, set, and card number — DH card ID caching is
// handled internally by the DHPriceProvider. Callers that have a DH card ID will
// benefit from cache hits without needing to pass the ID through this adapter layer.
func (a *Adapter) getPrice(ctx context.Context, card inventory.CardIdentity) (*pricing.Price, error) {
	c := pricing.CardLookup{Name: card.CardName, Number: card.CardNumber, PSAListingTitle: card.PSAListingTitle}
	price, err := a.provider.LookupCard(ctx, card.SetName, c)
	if err != nil {
		return nil, fmt.Errorf("price lookup for %q: %w", card.CardName, err)
	}
	return price, nil
}

// gradeDetailKey maps a grade to the GradeDetails map key.
// Half-grades use the floor grade's key (e.g. 8.5 → "psa8").
func gradeDetailKey(grade float64) string {
	return gradeToCanonical(grade).String()
}

// gradeToCanonical maps a numeric grade (float64) to the canonical pricing.Grade.
func gradeToCanonical(grade float64) pricing.Grade {
	switch math.Floor(grade) {
	case 10:
		return pricing.GradePSA10
	case 9:
		return pricing.GradePSA9
	case 8:
		return pricing.GradePSA8
	case 7:
		return pricing.GradePSA7
	case 6:
		return pricing.GradePSA6
	default:
		return pricing.GradeRaw
	}
}

func buildSourcePrices(price *pricing.Price, grade float64) []inventory.SourcePrice {
	var sources []inventory.SourcePrice

	// Per-grade detail data from DH and other sources
	key := gradeDetailKey(grade)
	if price.GradeDetails != nil {
		if detail, ok := price.GradeDetails[key]; ok && detail != nil {
			// eBay sales data
			if detail.Ebay != nil && detail.Ebay.PriceCents > 0 {
				sp := inventory.SourcePrice{
					Source:     "eBay",
					PriceCents: int(detail.Ebay.PriceCents),
					SaleCount:  detail.Ebay.SalesCount,
					Trend:      detail.Ebay.Trend,
					Confidence: detail.Ebay.Confidence,
				}
				if detail.Ebay.MinCents > 0 {
					sp.MinCents = int(detail.Ebay.MinCents)
				}
				if detail.Ebay.MaxCents > 0 {
					sp.MaxCents = int(detail.Ebay.MaxCents)
				}
				if detail.Ebay.Avg7DayCents > 0 {
					sp.Avg7DayCents = int(detail.Ebay.Avg7DayCents)
				}
				if detail.Ebay.Volume7Day > 0 {
					sp.Volume7Day = detail.Ebay.Volume7Day
				}
				sources = append(sources, sp)
			}

			// Price estimate
			if detail.Estimate != nil && detail.Estimate.PriceCents > 0 {
				sp := inventory.SourcePrice{
					Source:     "Estimate",
					PriceCents: int(detail.Estimate.PriceCents),
				}
				if detail.Estimate.LowCents > 0 {
					sp.MinCents = int(detail.Estimate.LowCents)
				}
				if detail.Estimate.HighCents > 0 {
					sp.MaxCents = int(detail.Estimate.HighCents)
				}
				if detail.Estimate.Confidence > 0 {
					if detail.Estimate.Confidence >= 0.8 {
						sp.Confidence = "high"
					} else if detail.Estimate.Confidence >= 0.5 {
						sp.Confidence = "medium"
					} else {
						sp.Confidence = "low"
					}
				}
				sources = append(sources, sp)
			}
		}
	}

	return sources
}

// gradeInfo returns the GradeSaleInfo for the given grade.
// Half-grades use the floor grade (e.g. 8.5 → PSA8).
func gradeInfo(lsbg *pricing.LastSoldByGrade, grade float64) *pricing.GradeSaleInfo {
	floorGrade := math.Floor(grade)
	switch floorGrade {
	case 10:
		return lsbg.PSA10
	case 9:
		return lsbg.PSA9
	case 8:
		return lsbg.PSA8
	case 7:
		return lsbg.PSA7
	case 6:
		return lsbg.PSA6
	default:
		return lsbg.Raw
	}
}

// gradePrice returns the price in cents for the given grade.
// For half-grades, interpolates between the floor and ceiling grade prices
// (average of the two adjacent whole grades).
func gradePrice(grades pricing.GradedPrices, grade float64) int {
	if !isHalfGrade(grade) {
		return wholeGradePrice(grades, grade)
	}
	// Interpolate between floor and ceiling grades
	floor := wholeGradePrice(grades, math.Floor(grade))
	ceil := wholeGradePrice(grades, math.Ceil(grade))
	if floor > 0 && ceil > 0 {
		return (floor + ceil) / 2
	}
	// If only one side has data, use it
	if floor > 0 {
		return floor
	}
	return ceil
}

func wholeGradePrice(grades pricing.GradedPrices, grade float64) int {
	return int(pricing.GetGradePrice(grades, gradeToCanonical(grade)))
}
