package inventory

import (
	"testing"
)

func TestParseCLRefreshRows_HappyPath(t *testing.T) {
	records := [][]string{
		{"Slab Serial #", "Card", "Set", "Number", "Current Value", "Population"},
		{"192060238", "Charizard Holo", "Base Set", "4", "450.00", "123"},
		{"99887766", "Pikachu VMAX", "Vivid Voltage", "44", "75.50", "500"},
	}

	rows, errs, err := ParseCLRefreshRows(records)
	if err != nil {
		t.Fatalf("ParseCLRefreshRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParseCLRefreshRows: unexpected parse errors: %v", errs)
	}
	if len(rows) != 2 {
		t.Fatalf("ParseCLRefreshRows: got %d rows, want 2", len(rows))
	}

	if rows[0].SlabSerial != "192060238" {
		t.Errorf("row[0].SlabSerial = %q, want %q", rows[0].SlabSerial, "192060238")
	}
	if rows[0].CurrentValue != 450.00 {
		t.Errorf("row[0].CurrentValue = %v, want 450.00", rows[0].CurrentValue)
	}
	if rows[0].Population != 123 {
		t.Errorf("row[0].Population = %d, want 123", rows[0].Population)
	}

	if rows[1].SlabSerial != "99887766" {
		t.Errorf("row[1].SlabSerial = %q, want %q", rows[1].SlabSerial, "99887766")
	}
	if rows[1].CurrentValue != 75.50 {
		t.Errorf("row[1].CurrentValue = %v, want 75.50", rows[1].CurrentValue)
	}
}

func TestParseCLRefreshRows_MissingColumn(t *testing.T) {
	records := [][]string{
		{"Card", "Set", "Current Value", "Population"},
		{"Charizard", "Base Set", "450.00", "123"},
	}

	_, _, err := ParseCLRefreshRows(records)
	if err == nil {
		t.Fatal("ParseCLRefreshRows: expected fatal error for missing 'slab serial #' column, got nil")
	}
}

func TestParseCLRefreshRows_InvalidValue(t *testing.T) {
	records := [][]string{
		{"Slab Serial #", "Card", "Set", "Number", "Current Value", "Population"},
		{"192060238", "Charizard Holo", "Base Set", "4", "not-a-number", "123"},
		{"99887766", "Pikachu VMAX", "Vivid Voltage", "44", "75.50", "500"},
	}

	rows, errs, err := ParseCLRefreshRows(records)
	if err != nil {
		t.Fatalf("ParseCLRefreshRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 1 {
		t.Errorf("ParseCLRefreshRows: got %d parse errors, want 1", len(errs))
	} else {
		if errs[0].Field != "current value" {
			t.Errorf("parse error field = %q, want %q", errs[0].Field, "current value")
		}
	}
	// Valid row should still be returned
	if len(rows) != 1 {
		t.Errorf("ParseCLRefreshRows: got %d valid rows, want 1", len(rows))
	}
	if len(rows) > 0 && rows[0].SlabSerial != "99887766" {
		t.Errorf("valid row SlabSerial = %q, want %q", rows[0].SlabSerial, "99887766")
	}
}

func TestParseCLImportRows_HappyPath(t *testing.T) {
	records := [][]string{
		{"Slab Serial #", "Card", "Player", "Set", "Number", "Condition", "Investment", "Current Value", "Date Purchased", "Population"},
		{"192060238", "Charizard Holo PSA 9", "Charizard", "Base Set", "4", "PSA 9", "300.00", "450.00", "3/15/2024", "123"},
	}

	rows, errs, err := ParseCLImportRows(records)
	if err != nil {
		t.Fatalf("ParseCLImportRows: unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("ParseCLImportRows: unexpected parse errors: %v", errs)
	}
	if len(rows) != 1 {
		t.Fatalf("ParseCLImportRows: got %d rows, want 1", len(rows))
	}

	row := rows[0]
	if row.SlabSerial != "192060238" {
		t.Errorf("SlabSerial = %q, want %q", row.SlabSerial, "192060238")
	}
	if row.Investment != 300.00 {
		t.Errorf("Investment = %v, want 300.00", row.Investment)
	}
	if row.CurrentValue != 450.00 {
		t.Errorf("CurrentValue = %v, want 450.00", row.CurrentValue)
	}
	if row.DatePurchased != "2024-03-15" {
		t.Errorf("DatePurchased = %q, want %q", row.DatePurchased, "2024-03-15")
	}
	if row.Population != 123 {
		t.Errorf("Population = %d, want 123", row.Population)
	}
}

func TestParseCLImportRows_MissingRequired(t *testing.T) {
	// Missing "investment" column
	records := [][]string{
		{"Slab Serial #", "Card", "Current Value", "Date Purchased"},
		{"192060238", "Charizard Holo", "450.00", "3/15/2024"},
	}

	_, _, err := ParseCLImportRows(records)
	if err == nil {
		t.Fatal("ParseCLImportRows: expected fatal error for missing 'investment' column, got nil")
	}
}
