package mathutil

import "math"

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
