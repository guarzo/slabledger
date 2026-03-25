package analysis

import (
	"math"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculatePercentile(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		p      float64
		want   float64
	}{
		{"empty", nil, 0.5, 0},
		{"single value", []float64{42.0}, 0.5, 42.0},
		{"p0", []float64{1, 2, 3, 4, 5}, 0, 1.0},
		{"p100", []float64{1, 2, 3, 4, 5}, 1.0, 5.0},
		{"median odd", []float64{1, 2, 3, 4, 5}, 0.5, 3.0},
		{"p25", []float64{1, 2, 3, 4, 5}, 0.25, 2.0},
		{"p75", []float64{1, 2, 3, 4, 5}, 0.75, 4.0},
		{"unsorted input", []float64{5, 1, 3, 2, 4}, 0.5, 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculatePercentile(tt.values, tt.p)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

func TestCalculateMean(t *testing.T) {
	assert.Equal(t, 0.0, CalculateMean(nil))
	assert.Equal(t, 5.0, CalculateMean([]float64{5}))
	assert.Equal(t, 3.0, CalculateMean([]float64{1, 2, 3, 4, 5}))
}

func TestBuildSalesDistribution(t *testing.T) {
	// Below threshold returns nil
	assert.Nil(t, BuildSalesDistribution([]float64{1, 2}, 5, 30, "test"))

	// Above threshold returns distribution
	values := make([]float64, 20)
	for i := range values {
		values[i] = float64(i + 1)
	}
	dist := BuildSalesDistribution(values, 5, 30, "test")
	require.NotNil(t, dist)
	assert.Equal(t, 20, dist.SampleSize)
	assert.Equal(t, 30, dist.Period)
	assert.Equal(t, "test", dist.Source)
	assert.Greater(t, dist.P50, dist.P25)
	assert.Greater(t, dist.P75, dist.P50)
}

func TestCalculateConservativeExits(t *testing.T) {
	// No sales
	assert.Nil(t, CalculateConservativeExits(nil, 10, "test"))

	// Below threshold for all grades
	sales := make([]pricing.SaleRecord, 5)
	for i := range sales {
		sales[i] = pricing.SaleRecord{PriceCents: 1000, Grade: "PSA 10", Date: "2026-01-01"}
	}
	assert.Nil(t, CalculateConservativeExits(sales, 10, "test"))

	// Above threshold for PSA 10
	sales = make([]pricing.SaleRecord, 15)
	for i := range sales {
		sales[i] = pricing.SaleRecord{PriceCents: (i + 1) * 1000, Grade: "PSA 10", Date: "2026-01-01"}
	}
	result := CalculateConservativeExits(sales, 10, "test")
	require.NotNil(t, result)
	assert.Greater(t, result.ConservativePSA10USD, 0.0)
	require.NotNil(t, result.PSA10Distribution)
	assert.Nil(t, result.PSA9Distribution)
	assert.Nil(t, result.RawDistribution)
}

func TestCalculateLastSoldByGrade(t *testing.T) {
	assert.Nil(t, CalculateLastSoldByGrade(nil))

	sales := []pricing.SaleRecord{
		{PriceCents: 5000, Grade: "PSA 10", Date: "2026-01-01"},
		{PriceCents: 6000, Grade: "PSA 10", Date: "2026-01-15"},
		{PriceCents: 3000, Grade: "PSA 9", Date: "2026-01-10"},
		{PriceCents: 1000, Grade: "Ungraded", Date: "2026-01-05"},
	}

	result := CalculateLastSoldByGrade(sales)
	require.NotNil(t, result)

	require.NotNil(t, result.PSA10)
	assert.Equal(t, 60.0, result.PSA10.LastSoldPrice)
	assert.Equal(t, "2026-01-15", result.PSA10.LastSoldDate)
	assert.Equal(t, 2, result.PSA10.SaleCount)

	require.NotNil(t, result.PSA9)
	assert.Equal(t, 30.0, result.PSA9.LastSoldPrice)

	require.NotNil(t, result.Raw)
	assert.InDelta(t, 10.0, result.Raw.LastSoldPrice, 0.01)

	assert.Nil(t, result.PSA8)
}

func TestCalculatePercentileDoesNotMutateInput(t *testing.T) {
	values := []float64{5, 3, 1, 4, 2}
	original := make([]float64, len(values))
	copy(original, values)

	CalculatePercentile(values, 0.5)

	for i := range values {
		if math.Abs(values[i]-original[i]) > 0.001 {
			t.Fatal("CalculatePercentile mutated the input slice")
		}
	}
}
