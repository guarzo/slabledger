package campaigns

import (
	"testing"
)

func TestParseOrdersExportRows(t *testing.T) {
	records := [][]string{
		{"Order", "Date", "Sales Channel", "Product Title", "Grading Company", "Cert Number", "Grade", "Qty", "Unit Price", "Line Subtotal"},
		{"#1002", "2026-03-10", "eBay", "Dark Gengar Holo - Neo Destiny - #6 PSA 5", "PSA", "194544353", "5", "1", "259.35", "259.35"},
		{"#1001", "2026-03-09", "Online Store", "Ditto - Old Maid CGC 10", "CGC", "", "10", "1", "22.80", "22.80"},
		{"#1004", "2026-03-14", "eBay", "Dragonite Holo PSA 3", "PSA", "191055511", "3", "1", "54.86", "54.86"},
		// Duplicate cert — should be skipped
		{"#1005", "2026-03-15", "eBay", "Dragonite Holo PSA 3", "PSA", "191055511", "3", "1", "54.86", "54.86"},
		// No grading company — should be skipped
		{"#1008", "2026-03-15", "eBay", "Umbreon & Darkrai GX", "", "", "", "1", "37.90", "37.90"},
		// PSA but empty cert — should be skipped
		{"#1012", "2026-03-20", "eBay", "Mewtwo PSA 9", "PSA", "", "9", "1", "442.89", "442.89"},
		// Unknown channel — should be skipped
		{"#1099", "2026-03-20", "Amazon", "Pikachu PSA 10", "PSA", "999999", "10", "1", "100.00", "100.00"},
	}

	rows, skipped, err := ParseOrdersExportRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 valid PSA rows
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// First row: eBay PSA card
	if rows[0].CertNumber != "194544353" {
		t.Errorf("row 0 cert: got %q, want 194544353", rows[0].CertNumber)
	}
	if rows[0].SalesChannel != SaleChannelEbay {
		t.Errorf("row 0 channel: got %q, want ebay", rows[0].SalesChannel)
	}
	if rows[0].UnitPrice != 259.35 {
		t.Errorf("row 0 price: got %f, want 259.35", rows[0].UnitPrice)
	}
	if rows[0].Date != "2026-03-10" {
		t.Errorf("row 0 date: got %q, want 2026-03-10", rows[0].Date)
	}

	// Second row
	if rows[1].CertNumber != "191055511" {
		t.Errorf("row 1 cert: got %q, want 191055511", rows[1].CertNumber)
	}
	if rows[1].SalesChannel != SaleChannelEbay {
		t.Errorf("row 1 channel: got %q, want ebay", rows[1].SalesChannel)
	}

	// Skipped: CGC (1) + duplicate cert (1) + no grader (1) + PSA empty cert (1) + unknown channel (1) = 5
	if len(skipped) != 5 {
		t.Fatalf("got %d skipped, want 5", len(skipped))
	}

	// Check skip reasons
	reasons := map[string]int{}
	for _, s := range skipped {
		reasons[s.Reason]++
	}
	if reasons["not_psa"] != 2 {
		t.Errorf("not_psa skips: got %d, want 2", reasons["not_psa"])
	}
	if reasons["duplicate"] != 1 {
		t.Errorf("duplicate skips: got %d, want 1", reasons["duplicate"])
	}
	if reasons["no_cert"] != 1 {
		t.Errorf("no_cert skips: got %d, want 1", reasons["no_cert"])
	}
	if reasons["unknown_channel"] != 1 {
		t.Errorf("unknown_channel skips: got %d, want 1", reasons["unknown_channel"])
	}
}

func TestParseOrdersExportRows_MissingHeader(t *testing.T) {
	records := [][]string{
		{"Order", "Date", "Product Title"}, // missing required columns
	}
	_, _, err := ParseOrdersExportRows(records)
	if err == nil {
		t.Fatal("expected error for missing columns")
	}
}

func TestParseOrdersExportRows_OnlineStoreChannel(t *testing.T) {
	records := [][]string{
		{"Order", "Date", "Sales Channel", "Product Title", "Grading Company", "Cert Number", "Grade", "Qty", "Unit Price", "Line Subtotal"},
		{"#1055", "2026-03-26", "Online Store", "Karen's Flareon PSA 8", "PSA", "139288937", "8", "1", "71.25", "71.25"},
	}
	rows, _, err := ParseOrdersExportRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].SalesChannel != SaleChannelWebsite {
		t.Errorf("channel: got %q, want website", rows[0].SalesChannel)
	}
}
