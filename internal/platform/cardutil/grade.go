package cardutil

import (
	"fmt"
	"math"
	"strings"
)

// GradeToCondition converts a numeric PSA grade to the condition format
// stored in cl_sales_comps (e.g. 10 → "g10", 8.5 → "g8_5").
func GradeToCondition(grade float64) string {
	if grade == math.Trunc(grade) {
		return fmt.Sprintf("g%d", int(grade))
	}
	s := fmt.Sprintf("g%g", grade)
	return strings.ReplaceAll(s, ".", "_")
}

// ConditionToAPIFormat converts a stored condition (e.g. "g10", "g8_5")
// back to the CL API format (e.g. "PSA 10", "PSA 8.5").
// Returns empty string if the condition doesn't start with "g".
func ConditionToAPIFormat(condition string) string {
	if !strings.HasPrefix(condition, "g") {
		return ""
	}
	return "PSA " + strings.ReplaceAll(strings.TrimPrefix(condition, "g"), "_", ".")
}
