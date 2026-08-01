package inventory

import "testing"

func TestValidSaleReason(t *testing.T) {
	tests := []struct {
		in         string
		valid      bool
		validPatch bool
	}{
		{"discretionary", true, true},
		{"invoice_pressure", true, true},
		{"aging_policy", true, true},
		{"bulk_lot", true, true},
		{"show_clearout", true, true},
		{"", true, false},
		{"bogus", false, false},
	}
	for _, tt := range tests {
		if got := ValidSaleReason(tt.in); got != tt.valid {
			t.Errorf("ValidSaleReason(%q)=%v want %v", tt.in, got, tt.valid)
		}
		if got := ValidSaleReasonForPatch(tt.in); got != tt.validPatch {
			t.Errorf("ValidSaleReasonForPatch(%q)=%v want %v", tt.in, got, tt.validPatch)
		}
	}
}
