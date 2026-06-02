package mathutil

import (
	"math"
	"strconv"
	"strings"
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

// GradeTitleMatches returns true when searchTitle clearly mentions
// "<grader> <grade>" (e.g. "PSA 10", "BGS 9.5"). Used to validate MM search
// hits whose collectibleId is grade-specific — accepting a wrong-grade title
// silently maps a non-10 purchase to PSA-10 priced rows.
//
// An empty grader defaults to "PSA". When the grader isn't mentioned in the
// title, or is mentioned only with different grade tokens, the result is false:
// unverifiable grade is treated as a mismatch because the empirical failure
// mode is binding to PSA 10.
func GradeTitleMatches(grader string, gradeValue float64, searchTitle string) bool {
	g := strings.ToUpper(strings.TrimSpace(grader))
	if g == "" {
		g = "PSA"
	}
	title := strings.ToUpper(searchTitle)
	wantGrade := strings.ToUpper(FormatGrade(gradeValue))
	idx := 0
	for {
		hit := strings.Index(title[idx:], g+" ")
		if hit < 0 {
			return false
		}
		start := idx + hit + len(g) + 1
		end := start
		for end < len(title) && title[end] != ' ' {
			end++
		}
		if title[start:end] == wantGrade {
			return true
		}
		idx = end
		if idx >= len(title) {
			return false
		}
	}
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
