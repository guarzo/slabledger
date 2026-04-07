package pricing

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

// Additional grade constants for PSA 1-5 (not in core grades).
const (
	GradePSA1 Grade = "psa1"
	GradePSA2 Grade = "psa2"
	GradePSA3 Grade = "psa3"
	GradePSA4 Grade = "psa4"
	GradePSA5 Grade = "psa5"
)

// CoreGrades are the grades used for DB operations and detail maps.
var CoreGrades = []Grade{GradePSA10, GradePSA9, GradePSA8, GradePSA7, GradePSA6, GradeRaw}

// AllDisplayGrades is the full set of recognized display-format grades (PSA 1-10 + Raw).
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

// DisplayLabel returns the human-readable display label for a grade (e.g., "PSA 10", "Raw").
func (g Grade) DisplayLabel() string {
	if label, ok := displayLabels[g]; ok {
		return label
	}
	return string(g)
}

// SetGradePrice sets the price for a given grade on a GradedPrices struct.
// Supported grades: GradePSA10, GradePSA9, GradePSA8, GradePSA7, GradePSA6,
// GradePSA95, GradeRaw, GradeBGS10.
// Grades outside this set (e.g., PSA 1-5) are silently ignored because GradedPrices
// has no fields for them.
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
