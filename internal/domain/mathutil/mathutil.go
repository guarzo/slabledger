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
