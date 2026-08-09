package inventory_test

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestValidPriceSource(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		valid bool
	}{
		{name: "empty is legacy/unknown", in: "", valid: true},
		{name: "itemized", in: inventory.PriceSourceItemized, valid: true},
		{name: "estimated", in: inventory.PriceSourceEstimated, valid: true},
		{name: "manual", in: inventory.PriceSourceManual, valid: true},
		{name: "bogus", in: "bogus", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inventory.ValidPriceSource(tt.in); got != tt.valid {
				t.Errorf("ValidPriceSource(%q) = %v, want %v", tt.in, got, tt.valid)
			}
		})
	}
}
