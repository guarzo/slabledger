package fusion

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/pricing/analysis"
)

// Confidence calculation weights for combining different quality factors.
const (
	confidenceSourceWeight        = 0.3
	confidenceVarianceWeight      = 0.3
	confidenceOutlierWeight       = 0.2
	confidenceWeightQualityWeight = 0.2
)

// Freshness thresholds for time-based price data decay.
const (
	freshnessFullValue = 1 * time.Hour      // Data < 1 hour = full value
	freshnessRecent    = 24 * time.Hour     // Data < 24 hours = 0.9
	freshnessWeek      = 7 * 24 * time.Hour // Data < 7 days = 0.7; older = 0.5
)

// FusionEngine combines price data from multiple sources
type FusionEngine struct {
	config FusionConfig
	logger observability.Logger
}

// FusionConfig defines configuration for the fusion engine
type FusionConfig struct {
	MinSources       int                // Minimum sources required for high confidence
	OutlierThreshold float64            // IQR multiplier for outlier detection
	SourceWeights    map[string]float64 // Weight assigned to each source
	DefaultWeight    float64            // Default weight for unknown sources
}

// DefaultFusionConfig returns sensible defaults
func DefaultFusionConfig() FusionConfig {
	return FusionConfig{
		MinSources:       2,   // At least 2 sources for good confidence
		OutlierThreshold: 1.5, // Standard IQR multiplier for outlier detection
		SourceWeights: map[string]float64{
			pricing.SourceCardHedger:  0.85, // Multi-platform price estimates
			pricing.SourceDoubleHolo: 0.90, // DoubleHolo recent sales data
		},
		DefaultWeight: 0.6, // Default for unknown sources
	}
}

// NewFusionEngine creates a new fusion engine
func NewFusionEngine(config FusionConfig, logger observability.Logger) *FusionEngine {
	// Ensure we have a default weight
	if config.DefaultWeight == 0 {
		config.DefaultWeight = 0.6
	}
	// Ensure we have a minimum sources count
	if config.MinSources == 0 {
		config.MinSources = 2
	}
	// Ensure we have an outlier threshold
	if config.OutlierThreshold == 0 {
		config.OutlierThreshold = 1.5
	}

	return &FusionEngine{
		config: config,
		logger: logger,
	}
}

// FusePrices combines multiple price data points into a single fused price
func (fe *FusionEngine) FusePrices(ctx context.Context, prices []PriceData) (*FusedPrice, error) {
	if len(prices) == 0 {
		return nil, fmt.Errorf("no price data provided")
	}

	// Convert to SourceData with weights
	sourceData := fe.convertToSourceData(prices)

	// Remove outliers using IQR method
	filteredPrices, outliers := fe.removeOutliers(ctx, sourceData)

	if len(filteredPrices) == 0 {
		return nil, fmt.Errorf("all prices were outliers")
	}

	// Calculate fused price using weighted median
	fusedValue := fe.calculateWeightedMedian(filteredPrices)

	// Calculate confidence score
	confidence := fe.calculateConfidence(filteredPrices, len(prices))

	// Determine currency (use most common or first)
	currency := fe.determineCurrency(prices)

	return &FusedPrice{
		Value:         fusedValue,
		Confidence:    confidence,
		SourceCount:   len(filteredPrices),
		Sources:       filteredPrices,
		OutliersFound: len(outliers),
		Method:        "weighted_median",
		Currency:      currency,
	}, nil
}

// convertToSourceData converts PriceData to SourceData with weights
func (fe *FusionEngine) convertToSourceData(prices []PriceData) []SourceData {
	result := make([]SourceData, 0, len(prices))

	for _, p := range prices {
		weight := fe.config.SourceWeights[p.Source.Name]
		if weight == 0 {
			weight = fe.config.DefaultWeight
		}

		// Adjust weight based on source confidence
		weight *= p.Source.Confidence

		// Adjust weight based on freshness (newer is better)
		freshnessMultiplier := fe.calculateFreshnessMultiplier(p.Source.Freshness)
		weight *= freshnessMultiplier

		// Adjust weight based on volume (more data is better)
		volumeMultiplier := fe.calculateVolumeMultiplier(p.Source.Volume)
		weight *= volumeMultiplier

		result = append(result, SourceData{
			Source: p.Source.Name,
			Price:  p.Value,
			Weight: weight,
		})
	}

	return result
}

// removeOutliers removes statistical outliers using IQR method
func (fe *FusionEngine) removeOutliers(ctx context.Context, prices []SourceData) (filtered, outliers []SourceData) {
	if len(prices) < 3 {
		return prices, nil // Need at least 3 points for outlier detection
	}

	// Extract values and sort
	values := make([]float64, len(prices))
	for i, p := range prices {
		values[i] = p.Price
	}
	sort.Float64s(values)

	// Calculate quartiles using shared analysis.CalculatePercentile (0.0-1.0 scale)
	q1 := analysis.CalculatePercentile(values, 0.25)
	q3 := analysis.CalculatePercentile(values, 0.75)
	iqr := q3 - q1

	// Calculate bounds
	lowerBound := q1 - (fe.config.OutlierThreshold * iqr)
	upperBound := q3 + (fe.config.OutlierThreshold * iqr)

	// Filter outliers
	for _, p := range prices {
		if p.Price < lowerBound || p.Price > upperBound {
			outliers = append(outliers, p)
			if fe.logger != nil {
				fe.logger.Debug(ctx, "outlier detected and removed",
					observability.String("source", p.Source),
					observability.Float64("price", p.Price),
					observability.Float64("lower_bound", lowerBound),
					observability.Float64("upper_bound", upperBound))
			}
		} else {
			filtered = append(filtered, p)
		}
	}

	return filtered, outliers
}

// calculateWeightedMedian calculates the weighted median of prices
func (fe *FusionEngine) calculateWeightedMedian(prices []SourceData) float64 {
	if len(prices) == 0 {
		return 0
	}

	// Sort by price
	sorted := make([]SourceData, len(prices))
	copy(sorted, prices)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Price < sorted[j].Price
	})

	// Calculate total weight
	totalWeight := 0.0
	for _, p := range sorted {
		totalWeight += p.Weight
	}

	// Find weighted median
	targetWeight := totalWeight / 2.0
	accWeight := 0.0

	for _, p := range sorted {
		accWeight += p.Weight
		if accWeight >= targetWeight {
			return p.Price
		}
	}

	// Fallback to last price (should not happen)
	return sorted[len(sorted)-1].Price
}

// calculateConfidence calculates confidence score based on multiple factors
func (fe *FusionEngine) calculateConfidence(prices []SourceData, originalCount int) float64 {
	if len(prices) == 0 {
		return 0.0
	}

	// Factor 1: Source count confidence (more sources = higher confidence)
	sourceConfidence := float64(len(prices)) / float64(fe.config.MinSources)
	if sourceConfidence > 1.0 {
		sourceConfidence = 1.0
	}

	// Factor 2: Price variance confidence (lower variance = higher confidence)
	variance := fe.calculateVariance(prices)
	meanPrice := fe.calculateMean(prices)

	var varianceConfidence float64
	if meanPrice > 0 {
		// Coefficient of variation (CV): lower is better
		cv := math.Sqrt(variance) / meanPrice
		// Convert CV to confidence (0 CV = 1.0 confidence, high CV = low confidence)
		varianceConfidence = 1.0 / (1.0 + cv)
	} else {
		varianceConfidence = 0.5 // Default if mean is 0
	}

	// Factor 3: Outlier confidence (fewer outliers = higher confidence)
	outliersRemoved := originalCount - len(prices)
	outlierConfidence := 1.0 - (float64(outliersRemoved) / float64(originalCount))

	// Factor 4: Weight quality confidence (higher average weight = better sources)
	avgWeight := 0.0
	for _, p := range prices {
		avgWeight += p.Weight
	}
	avgWeight /= float64(len(prices))
	weightConfidence := avgWeight

	// Weighted combination of factors
	confidence := (sourceConfidence * confidenceSourceWeight) +
		(varianceConfidence * confidenceVarianceWeight) +
		(outlierConfidence * confidenceOutlierWeight) +
		(weightConfidence * confidenceWeightQualityWeight)

	// Ensure between 0 and 1
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

// Helper methods

func (fe *FusionEngine) calculateFreshnessMultiplier(freshness time.Duration) float64 {
	if freshness < freshnessFullValue {
		return 1.0
	}
	if freshness < freshnessRecent {
		return 0.9
	}
	if freshness < freshnessWeek {
		return 0.7
	}
	return 0.5
}

func (fe *FusionEngine) calculateVolumeMultiplier(volume int) float64 {
	// More volume = higher confidence
	// 1-5 items = 0.6
	// 6-20 items = 0.8
	// 21+ items = 1.0
	if volume >= 21 {
		return 1.0
	}
	if volume >= 6 {
		return 0.8
	}
	if volume >= 1 {
		return 0.6
	}
	return 0.5
}

func (fe *FusionEngine) calculateVariance(prices []SourceData) float64 {
	if len(prices) == 0 {
		return 0
	}

	mean := fe.calculateMean(prices)
	variance := 0.0

	for _, p := range prices {
		diff := p.Price - mean
		variance += diff * diff
	}

	return variance / float64(len(prices))
}

func (fe *FusionEngine) calculateMean(prices []SourceData) float64 {
	values := make([]float64, len(prices))
	for i, p := range prices {
		values[i] = p.Price
	}
	return analysis.CalculateMean(values)
}

func (fe *FusionEngine) determineCurrency(prices []PriceData) string {
	if len(prices) == 0 {
		return "USD"
	}

	// Count currencies
	currencyCount := make(map[string]int)
	for _, p := range prices {
		currencyCount[p.Currency]++
	}

	// Find most common
	maxCount := 0
	currency := "USD"
	for curr, count := range currencyCount {
		if count > maxCount {
			maxCount = count
			currency = curr
		}
	}

	return currency
}
