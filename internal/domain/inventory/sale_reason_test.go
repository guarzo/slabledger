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

func TestIsForcedReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   bool
	}{
		{name: "invoice pressure is forced", reason: SaleReasonInvoicePressure, want: true},
		{name: "discretionary is not forced", reason: SaleReasonDiscretionary, want: false},
		{name: "aging policy is not forced", reason: SaleReasonAgingPolicy, want: false},
		{name: "bulk lot is not forced", reason: SaleReasonBulkLot, want: false},
		{name: "show clearout is not forced", reason: SaleReasonShowClearout, want: false},
		{name: "empty is not forced", reason: "", want: false},
		{name: "unknown is not forced", reason: "nonsense", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsForcedReason(tt.reason); got != tt.want {
				t.Errorf("IsForcedReason(%q) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}
