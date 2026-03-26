package campaigns

import (
	"testing"
)

func TestParsePSAExportRows_HappyPath(t *testing.T) {
	records := [][]string{
		{"Cert Number", "Listing Title", "Grade", "Price Paid", "Category", "Purchase Source"},
		{"192060238", "2021 Pokemon Charizard PSA 9", "9", "$150.00", "Pokemon", "eBay"},
		{"99887766", "2023 Pokemon Pikachu VMAX PSA 10", "10", "$75.50", "Pokemon", "TCGPlayer"},
	}

	rows, errs, err := ParsePSAExportRows(records)
	if err != nil {
		t.Fatalf("ParsePSAExportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParsePSAExportRows: unexpected parse errors: %v", errs)
	}
	if len(rows) != 2 {
		t.Fatalf("ParsePSAExportRows: got %d rows, want 2", len(rows))
	}

	row0 := rows[0]
	if row0.CertNumber != "192060238" {
		t.Errorf("row[0].CertNumber = %q, want %q", row0.CertNumber, "192060238")
	}
	if row0.Grade != 9 {
		t.Errorf("row[0].Grade = %v, want 9", row0.Grade)
	}
	if row0.PricePaid != 150.00 {
		t.Errorf("row[0].PricePaid = %v, want 150.00", row0.PricePaid)
	}
	if row0.ListingTitle != "2021 Pokemon Charizard PSA 9" {
		t.Errorf("row[0].ListingTitle = %q, want %q", row0.ListingTitle, "2021 Pokemon Charizard PSA 9")
	}
	if row0.Category != "Pokemon" {
		t.Errorf("row[0].Category = %q, want %q", row0.Category, "Pokemon")
	}
	if row0.PurchaseSource != "eBay" {
		t.Errorf("row[0].PurchaseSource = %q, want %q", row0.PurchaseSource, "eBay")
	}

	row1 := rows[1]
	if row1.CertNumber != "99887766" {
		t.Errorf("row[1].CertNumber = %q, want %q", row1.CertNumber, "99887766")
	}
	if row1.Grade != 10 {
		t.Errorf("row[1].Grade = %v, want 10", row1.Grade)
	}
}

func TestParsePSAExportRows_OffsetHeader(t *testing.T) {
	// Header on row index 2 (after two junk rows)
	records := [][]string{
		{"Report Title", "PSA Communication Spreadsheet"},
		{"Generated", "2024-01-01"},
		{"Cert Number", "Listing Title", "Grade", "Price Paid", "Category"},
		{"192060238", "Charizard PSA 9", "9", "$150.00", "Pokemon"},
	}

	rows, errs, err := ParsePSAExportRows(records)
	if err != nil {
		t.Fatalf("ParsePSAExportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParsePSAExportRows: unexpected parse errors: %v", errs)
	}
	if len(rows) != 1 {
		t.Fatalf("ParsePSAExportRows: got %d rows, want 1", len(rows))
	}
	if rows[0].CertNumber != "192060238" {
		t.Errorf("CertNumber = %q, want %q", rows[0].CertNumber, "192060238")
	}
}

func TestParsePSAExportRows_NoHeader(t *testing.T) {
	records := [][]string{
		{"Product", "SKU", "Amount"},
		{"Charizard", "CHAR-001", "150"},
		{"Pikachu", "PIK-001", "75"},
	}

	_, _, err := ParsePSAExportRows(records)
	if err == nil {
		t.Fatal("ParsePSAExportRows: expected fatal error for missing header, got nil")
	}
}

func TestParsePSAExportRows_InvalidGrade(t *testing.T) {
	records := [][]string{
		{"Cert Number", "Listing Title", "Grade", "Price Paid", "Category"},
		{"192060238", "Charizard PSA 9", "not-a-grade", "$150.00", "Pokemon"},
		{"99887766", "Pikachu VMAX PSA 10", "10", "$75.00", "Pokemon"},
	}

	rows, errs, err := ParsePSAExportRows(records)
	if err != nil {
		t.Fatalf("ParsePSAExportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 1 {
		t.Errorf("ParsePSAExportRows: got %d parse errors, want 1", len(errs))
	} else {
		if errs[0].Field != "grade" {
			t.Errorf("parse error field = %q, want %q", errs[0].Field, "grade")
		}
	}
	// Valid row should still be returned
	if len(rows) != 1 {
		t.Errorf("ParsePSAExportRows: got %d valid rows, want 1", len(rows))
	}
}

func TestParsePSAExportRows_NoValidRows(t *testing.T) {
	// All data rows have empty cert numbers
	records := [][]string{
		{"Cert Number", "Listing Title", "Grade", "Price Paid", "Category"},
		{"", "No cert card", "9", "$50.00", "Pokemon"},
		{"", "Another no cert", "10", "$75.00", "Pokemon"},
	}

	rows, errs, err := ParsePSAExportRows(records)
	if err != nil {
		t.Fatalf("ParsePSAExportRows: unexpected fatal error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 valid rows, got %d", len(rows))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 parse errors, got %d", len(errs))
	}
}
