package inventory

import (
	"testing"
)

func TestParseEbayOrderRows(t *testing.T) {
	records := [][]string{
		{}, // row 0: blank
		{"Sales Record Number", "Order Number", "Buyer Username", "Item Title", "Custom Label", "Sold For", "Sale Date"}, // row 1: header
		{}, // row 2: blank
		{"1938", "02-14554-02089", "buyer1", "Kyogre [Holo] - Pokemon Team Magma & Team Aqua #3 PSA 8.0", "DH-16586", "$137.28", "Apr-22-26"},
		{"1937", "08-14543-89414", "buyer2", "Venusaur - Pokemon Celebrations #15 PSA 9.0", "PSA-68423319", "$28.00", "Apr-22-26"},
	}

	rows, skipped, err := ParseEbayOrderRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d (skipped: %d)", len(rows), len(skipped))
	}

	// DH row
	if rows[0].DHInventoryID != 16586 {
		t.Errorf("expected DHInventoryID=16586, got %d", rows[0].DHInventoryID)
	}
	if rows[0].CertNumber != "" {
		t.Errorf("expected empty CertNumber for DH row, got %q", rows[0].CertNumber)
	}
	if rows[0].SalePriceCents != 13728 {
		t.Errorf("expected 13728 cents, got %d", rows[0].SalePriceCents)
	}
	if rows[0].Date != "2026-04-22" {
		t.Errorf("expected 2026-04-22, got %s", rows[0].Date)
	}
	if rows[0].OrderNumber != "02-14554-02089" {
		t.Errorf("expected order number 02-14554-02089, got %s", rows[0].OrderNumber)
	}

	// PSA cert row
	if rows[1].CertNumber != "68423319" {
		t.Errorf("expected CertNumber=68423319, got %q", rows[1].CertNumber)
	}
	if rows[1].DHInventoryID != 0 {
		t.Errorf("expected DHInventoryID=0 for cert row, got %d", rows[1].DHInventoryID)
	}
	if rows[1].SalePriceCents != 2800 {
		t.Errorf("expected 2800 cents, got %d", rows[1].SalePriceCents)
	}
}

func TestParseEbayOrderRows_SkipsNoIdentifier(t *testing.T) {
	records := [][]string{
		{},
		{"Sales Record Number", "Order Number", "Buyer Username", "Item Title", "Custom Label", "Sold For", "Sale Date"},
		{},
		{"1", "order-1", "buyer", "Some Card PSA 10", "", "$50.00", "Apr-22-26"},
		{"2", "order-2", "buyer", "Another Card", "psa-001-001", "$30.00", "Apr-22-26"},
	}

	rows, skipped, err := ParseEbayOrderRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rows) != 0 {
		t.Errorf("expected 0 valid rows, got %d", len(rows))
	}
	if len(skipped) != 2 {
		t.Errorf("expected 2 skipped, got %d", len(skipped))
	}
	for _, s := range skipped {
		if s.Reason != "no_identifier" {
			t.Errorf("expected reason no_identifier, got %s", s.Reason)
		}
	}
}

func TestParseEbayOrderRows_MissingHeader(t *testing.T) {
	records := [][]string{
		{},
		{"Order Number", "Item Title", "Sold For", "Sale Date"},
	}

	_, _, err := ParseEbayOrderRows(records)
	if err == nil {
		t.Fatal("expected error for missing Custom Label header")
	}
}

func TestParseEbayDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Apr-22-26", "2026-04-22"},
		{"Jan-01-25", "2025-01-01"},
		{"Dec-31-24", "2024-12-31"},
	}
	for _, tc := range tests {
		got, err := parseEbayDate(tc.input)
		if err != nil {
			t.Errorf("parseEbayDate(%q): %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseEbayDate(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestIsEbayCertLabel(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"PSA-68423319", true},
		{"psa-12345", true},
		{"PSA-", false},
		{"DH-123", false},
		{"psa-001-001", false},
		{"PSA-145258548", true},
	}
	for _, tc := range tests {
		got := isEbayCertLabel(tc.input)
		if got != tc.want {
			t.Errorf("isEbayCertLabel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
