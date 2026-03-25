package analysis

import (
	"sort"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// MinSalesThreshold is the minimum number of sales required for reliable percentile calculations.
const MinSalesThreshold = 10

// CalculatePercentile computes a percentile value using linear interpolation.
// p should be between 0.0 and 1.0 (e.g., 0.25 for p25).
func CalculatePercentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	index := p * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	fraction := index - float64(lower)
	return sorted[lower]*(1-fraction) + sorted[upper]*fraction
}

// CalculateMean calculates the arithmetic mean of a slice of values.
func CalculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// BuildSalesDistribution creates a SalesDistribution from a slice of sale prices.
// Returns nil if there are fewer than minSales values.
func BuildSalesDistribution(values []float64, minSales int, period int, source string) *pricing.SalesDistribution {
	if len(values) < minSales {
		return nil
	}

	return &pricing.SalesDistribution{
		P10:        CalculatePercentile(values, 0.10),
		P25:        CalculatePercentile(values, 0.25),
		P50:        CalculatePercentile(values, 0.50),
		P75:        CalculatePercentile(values, 0.75),
		P90:        CalculatePercentile(values, 0.90),
		Mean:       CalculateMean(values),
		SampleSize: len(values),
		Period:     period,
		Source:     source,
	}
}

// CalculateConservativeExits computes conservative (p25) and optimistic exit prices
// from recent sales data. PSA grades use p25 (conservative), raw NM uses p90 (optimistic).
func CalculateConservativeExits(sales []pricing.SaleRecord, minSalesThreshold int, source string) *pricing.ConservativeExits {
	if len(sales) == 0 {
		return nil
	}

	var psa10Sales, psa9Sales, rawSales []float64

	for _, sale := range sales {
		priceUSD := mathutil.ToDollars(int64(sale.PriceCents))
		grade := pricing.NormalizeGrade(sale.Grade)

		if grade.IsPSA10() {
			psa10Sales = append(psa10Sales, priceUSD)
		} else if grade.IsPSA9() {
			psa9Sales = append(psa9Sales, priceUSD)
		} else if grade.IsRaw() {
			rawSales = append(rawSales, priceUSD)
		}
	}

	result := &pricing.ConservativeExits{}
	hasData := false

	if len(psa10Sales) >= minSalesThreshold {
		result.ConservativePSA10USD = CalculatePercentile(psa10Sales, 0.25)
		result.PSA10Distribution = BuildSalesDistribution(psa10Sales, minSalesThreshold, 30, source)
		hasData = true
	}

	if len(psa9Sales) >= minSalesThreshold {
		result.ConservativePSA9USD = CalculatePercentile(psa9Sales, 0.25)
		result.PSA9Distribution = BuildSalesDistribution(psa9Sales, minSalesThreshold, 30, source)
		hasData = true
	}

	if len(rawSales) >= minSalesThreshold {
		result.OptimisticRawUSD = CalculatePercentile(rawSales, 0.90)
		result.RawDistribution = BuildSalesDistribution(rawSales, minSalesThreshold, 30, source)
		hasData = true
	}

	if !hasData {
		return nil
	}

	return result
}

// CalculateLastSoldByGrade extracts the most recent sale for each grade from sales data.
func CalculateLastSoldByGrade(sales []pricing.SaleRecord) *pricing.LastSoldByGrade {
	if len(sales) == 0 {
		return nil
	}

	type tracker struct {
		sale  *pricing.SaleRecord
		count int
	}

	var psa10, psa9, psa8, raw tracker

	for i := range sales {
		sale := &sales[i]
		grade := pricing.NormalizeGrade(sale.Grade)

		if grade.IsPSA10() {
			psa10.count++
			if psa10.sale == nil || sale.Date > psa10.sale.Date {
				psa10.sale = sale
			}
		} else if grade.IsPSA9() {
			psa9.count++
			if psa9.sale == nil || sale.Date > psa9.sale.Date {
				psa9.sale = sale
			}
		} else if grade.IsPSA8() {
			psa8.count++
			if psa8.sale == nil || sale.Date > psa8.sale.Date {
				psa8.sale = sale
			}
		} else if grade.IsRaw() {
			raw.count++
			if raw.sale == nil || sale.Date > raw.sale.Date {
				raw.sale = sale
			}
		}
	}

	result := &pricing.LastSoldByGrade{}
	hasData := false

	if psa10.sale != nil {
		result.PSA10 = &pricing.GradeSaleInfo{
			LastSoldPrice: mathutil.ToDollars(int64(psa10.sale.PriceCents)),
			LastSoldDate:  psa10.sale.Date,
			SaleCount:     psa10.count,
		}
		hasData = true
	}
	if psa9.sale != nil {
		result.PSA9 = &pricing.GradeSaleInfo{
			LastSoldPrice: mathutil.ToDollars(int64(psa9.sale.PriceCents)),
			LastSoldDate:  psa9.sale.Date,
			SaleCount:     psa9.count,
		}
		hasData = true
	}
	if psa8.sale != nil {
		result.PSA8 = &pricing.GradeSaleInfo{
			LastSoldPrice: mathutil.ToDollars(int64(psa8.sale.PriceCents)),
			LastSoldDate:  psa8.sale.Date,
			SaleCount:     psa8.count,
		}
		hasData = true
	}
	if raw.sale != nil {
		result.Raw = &pricing.GradeSaleInfo{
			LastSoldPrice: mathutil.ToDollars(int64(raw.sale.PriceCents)),
			LastSoldDate:  raw.sale.Date,
			SaleCount:     raw.count,
		}
		hasData = true
	}

	if !hasData {
		return nil
	}

	return result
}
