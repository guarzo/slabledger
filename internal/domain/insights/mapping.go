package insights

import "strings"

// parameterColumn maps inventory.TuningRecommendation.Parameter values to the
// four Insights table columns. Values not in the map are dropped from the
// table in v1.
var parameterColumn = map[string]string{
	"buyTermsCLPct": "buyPct",
	"phase":         "buyPct", // campaign-retire recommendations surface under buyPct
	"gradeRange":    "characters",
	"dailySpendCap": "spendCap",
}

// MapParameterToColumn returns the v1 column key for a tuning parameter, or "" if
// the parameter does not map to a v1 column.
func MapParameterToColumn(parameter string) string {
	return parameterColumn[parameter]
}

// confidenceActThreshold is the minimum TuningRecommendation.Confidence
// (which the existing code stores as "data point count") at which a cell is
// treated as an act-now signal rather than a tune suggestion.
const confidenceActThreshold = 15

// DeriveCellSeverity returns the cell severity for a given confidence value.
func DeriveCellSeverity(confidence int) Severity {
	if confidence >= confidenceActThreshold {
		return SeverityAct
	}
	return SeverityTune
}

// DeriveRowStatus returns the row status from its cell map.
// Kill wins if any recommendation starts with "Retire" or "Close";
// otherwise the most severe cell wins.
func DeriveRowStatus(cells map[string]TuningCell) Status {
	for _, c := range cells {
		r := strings.ToLower(strings.TrimSpace(c.Recommendation))
		if strings.HasPrefix(r, "retire") || strings.HasPrefix(r, "close") {
			return StatusKill
		}
	}
	hasAct := false
	hasTune := false
	for _, c := range cells {
		switch c.Severity {
		case SeverityAct:
			hasAct = true
		case SeverityTune:
			hasTune = true
		}
	}
	switch {
	case hasAct:
		return StatusAct
	case hasTune:
		return StatusTune
	default:
		return StatusOK
	}
}
