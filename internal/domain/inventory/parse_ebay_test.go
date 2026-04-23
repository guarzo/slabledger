package inventory

import (
	"testing"
)

func TestParseEbayOrderRows(t *testing.T) {
	baseHeader := [][]string{
		{},
		{"Sales Record Number", "Order Number", "Buyer Username", "Item Title", "Custom Label", "Sold For", "Sale Date"},
		{},
	}

	tests := []struct {
		name         string
		dataRows     [][]string
		wantRows     int
		wantSkipped  int
		wantErr      bool
		checkRows    func(t *testing.T, rows []EbayOrderRow)
		checkSkipped func(t *testing.T, skipped []OrdersImportSkip)
	}{
		{
			name: "DH and PSA cert rows parsed",
			dataRows: [][]string{
				{"1938", "02-14554-02089", "buyer1", "Kyogre PSA 8.0", "DH-16586", "$137.28", "Apr-22-26"},
				{"1937", "08-14543-89414", "buyer2", "Venusaur PSA 9.0", "PSA-68423319", "$28.00", "Apr-22-26"},
			},
			wantRows:    2,
			wantSkipped: 0,
			checkRows: func(t *testing.T, rows []EbayOrderRow) {
				if rows[0].DHInventoryID != 16586 {
					t.Errorf("row 0: DHInventoryID = %d, want 16586", rows[0].DHInventoryID)
				}
				if rows[0].CertNumber != "" {
					t.Errorf("row 0: CertNumber = %q, want empty", rows[0].CertNumber)
				}
				if rows[0].SalePriceCents != 13728 {
					t.Errorf("row 0: SalePriceCents = %d, want 13728", rows[0].SalePriceCents)
				}
				if rows[0].Date != "2026-04-22" {
					t.Errorf("row 0: Date = %q, want 2026-04-22", rows[0].Date)
				}
				if rows[0].OrderNumber != "02-14554-02089" {
					t.Errorf("row 0: OrderNumber = %q, want 02-14554-02089", rows[0].OrderNumber)
				}
				if rows[1].CertNumber != "68423319" {
					t.Errorf("row 1: CertNumber = %q, want 68423319", rows[1].CertNumber)
				}
				if rows[1].DHInventoryID != 0 {
					t.Errorf("row 1: DHInventoryID = %d, want 0", rows[1].DHInventoryID)
				}
				if rows[1].SalePriceCents != 2800 {
					t.Errorf("row 1: SalePriceCents = %d, want 2800", rows[1].SalePriceCents)
				}
			},
		},
		{
			name: "no_identifier skipped",
			dataRows: [][]string{
				{"1", "order-1", "buyer", "Some Card", "", "$50.00", "Apr-22-26"},
				{"2", "order-2", "buyer", "Another Card", "psa-001-001", "$30.00", "Apr-22-26"},
			},
			wantRows:    0,
			wantSkipped: 2,
			checkSkipped: func(t *testing.T, skipped []OrdersImportSkip) {
				for _, s := range skipped {
					if s.Reason != "no_identifier" {
						t.Errorf("reason = %q, want no_identifier", s.Reason)
					}
				}
			},
		},
		{
			name: "invalid_dh_id skipped",
			dataRows: [][]string{
				{"1", "order-1", "buyer", "Card Title", "DH-xyz", "$50.00", "Apr-22-26"},
			},
			wantRows:    0,
			wantSkipped: 1,
			checkSkipped: func(t *testing.T, skipped []OrdersImportSkip) {
				if skipped[0].Reason != "invalid_dh_id: DH-xyz" {
					t.Errorf("reason = %q, want invalid_dh_id: DH-xyz", skipped[0].Reason)
				}
			},
		},
		{
			name: "zero and negative dh_id skipped",
			dataRows: [][]string{
				{"1", "order-1", "buyer", "Card A", "DH-0", "$50.00", "Apr-22-26"},
				{"2", "order-2", "buyer", "Card B", "DH--5", "$50.00", "Apr-22-26"},
			},
			wantRows:    0,
			wantSkipped: 2,
		},
		{
			name: "invalid_price skipped",
			dataRows: [][]string{
				{"1", "order-1", "buyer", "Card Title", "DH-123", "$xx", "Apr-22-26"},
			},
			wantRows:    0,
			wantSkipped: 1,
			checkSkipped: func(t *testing.T, skipped []OrdersImportSkip) {
				if skipped[0].Reason != "invalid_price: $xx" {
					t.Errorf("reason = %q, want invalid_price: $xx", skipped[0].Reason)
				}
			},
		},
		{
			name: "invalid_date skipped",
			dataRows: [][]string{
				{"1", "order-1", "buyer", "Card Title", "DH-123", "$50.00", "BadDate"},
			},
			wantRows:    0,
			wantSkipped: 1,
			checkSkipped: func(t *testing.T, skipped []OrdersImportSkip) {
				if skipped[0].Reason != "invalid_date: BadDate" {
					t.Errorf("reason = %q, want invalid_date: BadDate", skipped[0].Reason)
				}
			},
		},
		{
			name:    "missing required header",
			wantErr: true,
		},
		{
			name: "blank rows skipped silently",
			dataRows: [][]string{
				{"", "", "", "", "", "", ""},
			},
			wantRows:    0,
			wantSkipped: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var records [][]string
			if tc.name == "missing required header" {
				records = [][]string{
					{},
					{"Sales Record Number", "Order Number", "Buyer Username", "Custom Label", "Sold For", "Sale Date"},
					{},
					{"1", "order-1", "buyer", "DH-123", "$50.00", "Apr-22-26"},
				}
			} else {
				records = append(records, baseHeader...)
				records = append(records, tc.dataRows...)
			}

			rows, skipped, err := ParseEbayOrderRows(records)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rows) != tc.wantRows {
				t.Errorf("rows = %d, want %d", len(rows), tc.wantRows)
			}
			if len(skipped) != tc.wantSkipped {
				t.Errorf("skipped = %d, want %d", len(skipped), tc.wantSkipped)
			}
			if tc.checkRows != nil {
				tc.checkRows(t, rows)
			}
			if tc.checkSkipped != nil {
				tc.checkSkipped(t, skipped)
			}
		})
	}
}

func TestParseEbayDate(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"Apr-22-26", "2026-04-22", false},
		{"Jan-01-25", "2025-01-01", false},
		{"Dec-31-24", "2024-12-31", false},
		{"BadDate", "", true},
		{"", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseEbayDate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsEbayCertLabel(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"PSA-68423319", true},
		{"psa-12345", true},
		{"PSA-145258548", true},
		{"PSA-", false},
		{"DH-123", false},
		{"psa-001-001", false},
		{"", false},
		{"PSA-abc", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := isEbayCertLabel(tc.input); got != tc.want {
				t.Errorf("isEbayCertLabel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
