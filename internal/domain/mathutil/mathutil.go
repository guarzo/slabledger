package mathutil

import (
	"math"
	"strconv"
)

// Round2 rounds a float to 2 decimal places.
func Round2(f float64) float64 {
	return math.Round(f*100) / 100
}

// ToCents converts a dollar float to int64 cents with proper rounding.
func ToCents(dollars float64) int64 {
	return int64(math.Round(dollars * 100))
}

// ToCentsInt converts a dollar float to int cents with proper rounding.
// Convenience wrapper around ToCents for callers that work with int.
func ToCentsInt(dollars float64) int {
	return int(ToCents(dollars))
}

// ToDollars converts int64 cents to a dollar float.
func ToDollars(cents int64) float64 {
	return float64(cents) / 100
}

// FormatGrade formats a numeric grade for display: whole numbers are printed without
// a decimal point (9 → "9"), fractional grades use the minimal representation (9.5 → "9.5").
func FormatGrade(v float64) string {
	if v == float64(int(v)) {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ConfidenceLabel returns a confidence string ("high", "medium", "low") based on data point count.
// Thresholds: ≥20 → "high", ≥5 → "medium", else → "low".
func ConfidenceLabel(n int) string {
	switch {
	case n >= 20:
		return "high"
	case n >= 5:
		return "medium"
	default:
		return "low"
	}
}

// ConfidenceScore returns a numeric confidence weight derived from data point count.
// Thresholds: ≥20 → 1.0, ≥5 → 0.6, else → 0.3.
func ConfidenceScore(n int) float64 {
	switch {
	case n >= 20:
		return 1.0
	case n >= 5:
		return 0.6
	default:
		return 0.3
	}
}
