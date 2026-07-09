package psaportal

import "testing"

func TestNormalizeDate(t *testing.T) {
	tests := []struct{ in, want string }{
		{"2026-07-06T00:00:00Z", "2026-07-06"},
		{"2026-07-06", "2026-07-06"},
		{"", ""},
		{"not-a-date", "not-a-date"},
	}
	for _, tt := range tests {
		if got := normalizeDate(tt.in); got != tt.want {
			t.Errorf("normalizeDate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestInvoiceDateFor(t *testing.T) {
	tests := []struct{ in, want string }{
		{"2026-07-06T00:00:00Z", "2026-07-15"}, // mid-month → 15th
		{"2026-07-16", "2026-08-01"},           // after 15th → 1st next month
		{"2026-07-15", "2026-07-15"},           // on the 15th → itself
		{"2026-07-01", "2026-07-01"},           // on the 1st → itself
		{"2026-07-02", "2026-07-15"},           // just after 1st → 15th
		{"2026-12-20", "2027-01-01"},           // year rollover
		{"", ""},                               // empty → empty
		{"garbage", ""},                        // unparseable → empty
	}
	for _, tt := range tests {
		if got := invoiceDateFor(tt.in); got != tt.want {
			t.Errorf("invoiceDateFor(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
