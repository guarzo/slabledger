package inventory

import "testing"

func TestDueDateFromInvoiceDate(t *testing.T) {
	tests := []struct {
		name        string
		invoiceDate string
		want        string
	}{
		{"plus 7 days", "2026-07-01", "2026-07-08"},
		{"crosses month boundary", "2026-07-28", "2026-08-04"},
		{"crosses year boundary", "2026-12-29", "2027-01-05"},
		{"empty input", "", ""},
		{"malformed input", "not-a-date", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dueDateFromInvoiceDate(tt.invoiceDate); got != tt.want {
				t.Errorf("dueDateFromInvoiceDate(%q) = %q, want %q", tt.invoiceDate, got, tt.want)
			}
		})
	}
}
