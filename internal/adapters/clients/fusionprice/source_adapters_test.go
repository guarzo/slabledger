package fusionprice

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// requireEstimate is a test helper that fetches an estimate by grade key and fails the test if nil.
func requireEstimate(t *testing.T, estimates map[string]*pricing.EstimateGradeDetail, key string) *pricing.EstimateGradeDetail {
	t.Helper()
	d, ok := estimates[key]
	if !ok || d == nil {
		t.Fatalf("expected estimates[%q] to be non-nil", key)
	}
	return d
}
