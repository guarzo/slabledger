package pricing

import (
	"regexp"
	"strings"
)

// Grade represents a normalized card grade.
type Grade string

const (
	GradeRaw     Grade = "raw"
	GradePSA6    Grade = "psa6"
	GradePSA7    Grade = "psa7"
	GradePSA8    Grade = "psa8"
	GradePSA9    Grade = "psa9"
	GradePSA95   Grade = "psa95" // CGC/BGS 9.5
	GradePSA10   Grade = "psa10"
	GradeBGS10   Grade = "bgs10" // BGS 10 Black Label
	GradeUnknown Grade = ""      // Unrecognized grade
)

// nonAlphanumericRegex matches any non-alphanumeric characters for grade normalization.
var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9.]+`)

// NormalizeGrade converts a raw grade string to a canonical Grade value.
// It lowercases the input, strips non-alphanumeric characters (preserving dots
// for values like "9.5"), then matches against known patterns.
//
// Examples:
//   - "PSA 10", "psa10", "10" -> GradePSA10
//   - "PSA 9", "psa9", "9"   -> GradePSA9
//   - "PSA 8", "psa8", "8"   -> GradePSA8
//   - "BGS 9.5", "CGC 9.5"   -> GradePSA95
//   - "BGS 10"                -> GradeBGS10
//   - "PSA 7", "7"            -> GradePSA7
//   - "PSA 6", "6"            -> GradePSA6
//   - "PSA 5", "5" .. "PSA 1", "1" -> GradePSA5..GradePSA1
//   - "", "ungraded", "raw", "nm", "Near Mint" -> GradeRaw
func NormalizeGrade(raw string) Grade {
	// Convert to lowercase and trim whitespace
	normalized := strings.ToLower(strings.TrimSpace(raw))

	// Remove all non-alphanumeric characters (spaces, hyphens, etc.)
	// but preserve dots for grades like "9.5"
	normalized = nonAlphanumericRegex.ReplaceAllString(normalized, "")

	// Check for raw/ungraded first
	switch normalized {
	case "", "ungraded", "raw", "nm", "nearmint":
		return GradeRaw
	}

	// BGS 10 (Black Label) — check before generic "10" match
	if normalized == "bgs10" || normalized == "bgs10blacklabel" {
		return GradeBGS10
	}

	// 9.5 grades (CGC/BGS)
	if normalized == "9.5" || normalized == "psa95" ||
		normalized == "cgc9.5" || normalized == "bgs9.5" ||
		normalized == "cgc95" || normalized == "bgs95" {
		return GradePSA95
	}

	// PSA 10
	if normalized == "psa10" || normalized == "10" || normalized == "gemmint10" {
		return GradePSA10
	}

	// PSA 9
	if normalized == "psa9" || normalized == "9" {
		return GradePSA9
	}

	// PSA 8
	if normalized == "psa8" || normalized == "8" {
		return GradePSA8
	}

	// PSA 7
	if normalized == "psa7" || normalized == "7" {
		return GradePSA7
	}

	// PSA 6
	if normalized == "psa6" || normalized == "6" {
		return GradePSA6
	}

	// PSA 5
	if normalized == "psa5" || normalized == "5" {
		return GradePSA5
	}

	// PSA 4
	if normalized == "psa4" || normalized == "4" {
		return GradePSA4
	}

	// PSA 3
	if normalized == "psa3" || normalized == "3" {
		return GradePSA3
	}

	// PSA 2
	if normalized == "psa2" || normalized == "2" {
		return GradePSA2
	}

	// PSA 1
	if normalized == "psa1" || normalized == "1" {
		return GradePSA1
	}

	return GradeUnknown
}

// Additional grade constants for PSA 1-5 (used by CardHedger but not in core fusion).
const (
	GradePSA1 Grade = "psa1"
	GradePSA2 Grade = "psa2"
	GradePSA3 Grade = "psa3"
	GradePSA4 Grade = "psa4"
	GradePSA5 Grade = "psa5"
)

// CoreGrades are the grades used for DB operations, fusion output, and detail maps.
var CoreGrades = []Grade{GradePSA10, GradePSA9, GradePSA8, GradePSA7, GradePSA6, GradeRaw}

// AllDisplayGrades is the full set of recognized display-format grades (PSA 1-10 + Raw).
// Used for CardHedger grade validation.
var AllDisplayGrades = []Grade{
	GradePSA10, GradePSA9, GradePSA8, GradePSA7, GradePSA6,
	GradePSA5, GradePSA4, GradePSA3, GradePSA2, GradePSA1,
	GradeRaw,
}

// displayLabels maps each Grade to its human-readable display label.
var displayLabels = map[Grade]string{
	GradeRaw:   "Raw",
	GradePSA1:  "PSA 1",
	GradePSA2:  "PSA 2",
	GradePSA3:  "PSA 3",
	GradePSA4:  "PSA 4",
	GradePSA5:  "PSA 5",
	GradePSA6:  "PSA 6",
	GradePSA7:  "PSA 7",
	GradePSA8:  "PSA 8",
	GradePSA9:  "PSA 9",
	GradePSA95: "CGC 9.5",
	GradePSA10: "PSA 10",
	GradeBGS10: "BGS 10",
}

// displayToGrade is the reverse lookup from display label to Grade.
// Includes aliases (e.g., "BGS 9.5" → GradePSA95) so GradeFromDisplay and
// IsKnownDisplayGrade recognize common alternative labels.
var displayToGrade = func() map[string]Grade {
	m := make(map[string]Grade, len(displayLabels)+1)
	for g, label := range displayLabels {
		m[label] = g
	}
	// "BGS 9.5" is a common alias for the 9.5 grade (canonical label is "CGC 9.5").
	m["BGS 9.5"] = GradePSA95
	return m
}()

// DisplayLabel returns the human-readable display label for a grade (e.g., "PSA 10", "Raw").
func (g Grade) DisplayLabel() string {
	if label, ok := displayLabels[g]; ok {
		return label
	}
	return string(g)
}

// GradeFromDisplay parses a display-format string (e.g., "PSA 10", "Raw") to a Grade.
// Returns GradeUnknown if the display string is not recognized.
func GradeFromDisplay(display string) Grade {
	if g, ok := displayToGrade[display]; ok {
		return g
	}
	return GradeUnknown
}

// IsKnownDisplayGrade reports whether the display string is a recognized grade label
// (any grading system: PSA, BGS, CGC).
func IsKnownDisplayGrade(display string) bool {
	_, ok := displayToGrade[display]
	return ok
}

// extraCardHedgerGrades lists non-PSA grade strings that CardHedger returns
// for other grading companies. These are recognized as valid API responses
// but only a subset maps to fusion Grade values (via displayToGrade).
var extraCardHedgerGrades = func() []string {
	companies := []string{"CGC", "BGS", "AGS", "TAG", "SGC", "HGA"}
	numericGrades := []string{
		"10", "9.5", "9", "8.5", "8", "7.5", "7", "6.5", "6",
		"5.5", "5", "4.5", "4", "3.5", "3", "2.5", "2", "1.5", "1",
	}
	out := make([]string, 0, len(companies)*len(numericGrades)+1)
	out = append(out, "CGC 10 PRISTINE")
	for _, co := range companies {
		for _, g := range numericGrades {
			out = append(out, co+" "+g)
		}
	}
	return out
}()

// knownCardHedgerGrades is the set of display labels that CardHedger returns.
// Includes PSA grades + Raw plus non-PSA grading companies (CGC, BGS, AGS, TAG).
var knownCardHedgerGrades = func() map[string]bool {
	m := make(map[string]bool, len(AllDisplayGrades)+len(extraCardHedgerGrades))
	for _, g := range AllDisplayGrades {
		m[g.DisplayLabel()] = true
	}
	for _, s := range extraCardHedgerGrades {
		m[s] = true
	}
	return m
}()

// IsCardHedgerGrade reports whether the display string is a recognized
// CardHedger grade: PSA 1-10, Raw, and non-PSA grading company labels
// (CGC, BGS, AGS, TAG with various numeric grades).
func IsCardHedgerGrade(display string) bool {
	return knownCardHedgerGrades[display]
}

// SetGradePrice sets the price for a given grade on a GradedPrices struct.
// Supported grades: GradePSA10, GradePSA9, GradePSA8, GradePSA7, GradePSA6,
// GradePSA95, GradeRaw, GradeBGS10.
// Grades outside this set (e.g., PSA 1-5) are silently ignored because GradedPrices
// has no fields for them — they are only used for CardHedger API validation, not storage.
func SetGradePrice(grades *GradedPrices, g Grade, cents int64) {
	switch g {
	case GradePSA10:
		grades.PSA10Cents = cents
	case GradePSA9:
		grades.PSA9Cents = cents
	case GradePSA8:
		grades.PSA8Cents = cents
	case GradePSA7:
		grades.PSA7Cents = cents
	case GradePSA6:
		grades.PSA6Cents = cents
	case GradePSA95:
		grades.Grade95Cents = cents
	case GradeRaw:
		grades.RawCents = cents
	case GradeBGS10:
		grades.BGS10Cents = cents
	}
}

// GetGradePrice returns the price for a given grade from a GradedPrices struct.
// Supported grades: GradePSA10, GradePSA9, GradePSA8, GradePSA7, GradePSA6,
// GradePSA95, GradeRaw, GradeBGS10.
// Returns 0 for unsupported grades (e.g., PSA 1-5) which have no GradedPrices field.
func GetGradePrice(grades GradedPrices, g Grade) int64 {
	switch g {
	case GradePSA10:
		return grades.PSA10Cents
	case GradePSA9:
		return grades.PSA9Cents
	case GradePSA8:
		return grades.PSA8Cents
	case GradePSA7:
		return grades.PSA7Cents
	case GradePSA6:
		return grades.PSA6Cents
	case GradePSA95:
		return grades.Grade95Cents
	case GradeRaw:
		return grades.RawCents
	case GradeBGS10:
		return grades.BGS10Cents
	default:
		return 0
	}
}

func (g Grade) IsPSA10() bool { return g == GradePSA10 }
func (g Grade) IsPSA9() bool  { return g == GradePSA9 }
func (g Grade) IsPSA8() bool  { return g == GradePSA8 }
func (g Grade) IsRaw() bool   { return g == GradeRaw }

// String returns the string representation of the grade.
func (g Grade) String() string { return string(g) }
