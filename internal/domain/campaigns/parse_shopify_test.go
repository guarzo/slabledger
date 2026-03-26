package campaigns

import (
	"testing"
)

func TestParseShopifyExportRows_HappyPath(t *testing.T) {
	records := [][]string{
		{"Handle", "Title", "Cert Number", "SKU", "Tags", "Image Src", "Variant Price", "Cost Per Item"},
		{"charizard-psa-9", "2021 Pokemon Charizard PSA 9", "192060238", "SKU-001", "Charizard, 4, Base Set, Pokemon", "https://example.com/front.jpg", "150.00", "100.00"},
	}

	rows, errs, err := ParseShopifyExportRows(records)
	if err != nil {
		t.Fatalf("ParseShopifyExportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParseShopifyExportRows: unexpected parse errors: %v", errs)
	}
	if len(rows) != 1 {
		t.Fatalf("ParseShopifyExportRows: got %d rows, want 1", len(rows))
	}

	row := rows[0]
	if row.Handle != "charizard-psa-9" {
		t.Errorf("Handle = %q, want %q", row.Handle, "charizard-psa-9")
	}
	if row.CertNumber != "192060238" {
		t.Errorf("CertNumber = %q, want %q", row.CertNumber, "192060238")
	}
	if row.Title != "2021 Pokemon Charizard PSA 9" {
		t.Errorf("Title = %q, want %q", row.Title, "2021 Pokemon Charizard PSA 9")
	}
	if row.VariantPrice != 150.00 {
		t.Errorf("VariantPrice = %v, want 150.00", row.VariantPrice)
	}
	if row.CostPerItem != 100.00 {
		t.Errorf("CostPerItem = %v, want 100.00", row.CostPerItem)
	}
	if row.FrontImageURL != "https://example.com/front.jpg" {
		t.Errorf("FrontImageURL = %q, want %q", row.FrontImageURL, "https://example.com/front.jpg")
	}
}

func TestParseShopifyExportRows_HandleConsolidation(t *testing.T) {
	// Two rows with the same handle but different cert numbers → 2 distinct products
	records := [][]string{
		{"Handle", "Title", "Cert Number", "SKU", "Tags", "Image Src", "Variant Price", "Cost Per Item"},
		{"bundle-handle", "Charizard PSA 9", "192060238", "SKU-001", "Charizard, 4, Base Set", "https://example.com/front1.jpg", "150.00", "100.00"},
		{"bundle-handle", "Pikachu PSA 10", "99887766", "SKU-002", "Pikachu, 58, Base Set", "https://example.com/front2.jpg", "75.00", "50.00"},
	}

	rows, errs, err := ParseShopifyExportRows(records)
	if err != nil {
		t.Fatalf("ParseShopifyExportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParseShopifyExportRows: unexpected parse errors: %v", errs)
	}
	if len(rows) != 2 {
		t.Fatalf("ParseShopifyExportRows: got %d rows, want 2", len(rows))
	}

	certs := map[string]bool{
		rows[0].CertNumber: true,
		rows[1].CertNumber: true,
	}
	if !certs["192060238"] || !certs["99887766"] {
		t.Errorf("expected cert numbers 192060238 and 99887766, got %v and %v", rows[0].CertNumber, rows[1].CertNumber)
	}
}

func TestParseShopifyExportRows_BackImageMerge(t *testing.T) {
	// First row is the main product; second row is variant-only (empty title) with back image
	records := [][]string{
		{"Handle", "Title", "Cert Number", "SKU", "Tags", "Image Src", "Variant Price", "Cost Per Item"},
		{"charizard-psa-9", "2021 Pokemon Charizard PSA 9", "192060238", "SKU-001", "Charizard, 4, Base Set", "https://example.com/front.jpg", "150.00", "100.00"},
		{"charizard-psa-9", "", "", "", "", "https://example.com/back.jpg", "", ""},
	}

	rows, errs, err := ParseShopifyExportRows(records)
	if err != nil {
		t.Fatalf("ParseShopifyExportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParseShopifyExportRows: unexpected parse errors: %v", errs)
	}
	if len(rows) != 1 {
		t.Fatalf("ParseShopifyExportRows: got %d rows, want 1", len(rows))
	}

	row := rows[0]
	if row.FrontImageURL != "https://example.com/front.jpg" {
		t.Errorf("FrontImageURL = %q, want %q", row.FrontImageURL, "https://example.com/front.jpg")
	}
	if row.BackImageURL != "https://example.com/back.jpg" {
		t.Errorf("BackImageURL = %q, want %q", row.BackImageURL, "https://example.com/back.jpg")
	}
}

func TestParseShopifyExportRows_MissingHandle(t *testing.T) {
	// Missing "handle" column → fatal error
	records := [][]string{
		{"Title", "Cert Number", "SKU", "Tags", "Image Src", "Variant Price"},
		{"Charizard PSA 9", "192060238", "SKU-001", "Charizard, 4, Base Set", "https://example.com/front.jpg", "150.00"},
	}

	_, _, err := ParseShopifyExportRows(records)
	if err == nil {
		t.Fatal("ParseShopifyExportRows: expected fatal error for missing 'handle' column, got nil")
	}
}

func TestParseShopifyExportRows_NoCertSkipped(t *testing.T) {
	// Row with no cert number (CGC, raw, etc.) should be skipped
	records := [][]string{
		{"Handle", "Title", "Cert Number", "SKU", "Tags", "Image Src", "Variant Price", "Cost Per Item"},
		{"raw-card", "Raw Charizard Near Mint", "", "RAW-001", "Charizard, 4, Base Set", "https://example.com/raw.jpg", "30.00", "20.00"},
		{"cgc-card", "Pikachu CGC 9", "", "CGC-001", "Pikachu, 58, Base Set", "https://example.com/cgc.jpg", "60.00", "40.00"},
	}

	rows, errs, err := ParseShopifyExportRows(records)
	if err != nil {
		t.Fatalf("ParseShopifyExportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParseShopifyExportRows: unexpected parse errors: %v", errs)
	}
	// Both rows should be skipped since they have no PSA cert numbers
	if len(rows) != 0 {
		t.Errorf("ParseShopifyExportRows: got %d rows, want 0 (all should be skipped)", len(rows))
	}
}
