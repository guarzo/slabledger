package campaigns

import (
	"strings"
	"testing"
)

func TestParseMMRefreshRows(t *testing.T) {
	validHeader := []string{
		"Sport", "Grade", "Player Name", "Year", "Set", "Variation",
		"Card Number", "Specific Qualifier", "Quantity", "Date Purchased",
		"Purchase Price Per Card", "Notes", "Category", "Date Sold",
		"Sold Price Per Card", "Last Sale Price", "Last Sale Date",
	}

	tests := []struct {
		name        string
		records     [][]string
		wantRows    []MMRefreshRow
		wantErrMsgs []string // expected ParseError.Message substrings
		wantFatal   bool     // expect a non-nil fatal error
	}{
		{
			name:      "empty CSV returns fatal error",
			records:   [][]string{},
			wantFatal: true,
		},
		{
			name: "wrong first header column returns fatal error",
			records: [][]string{
				{"Name", "Grade", "Player Name", "Year", "Set", "Variation",
					"Card Number", "Specific Qualifier", "Quantity", "Date Purchased",
					"Purchase Price Per Card", "Notes", "Category", "Date Sold",
					"Sold Price Per Card", "Last Sale Price"},
			},
			wantFatal: true,
		},
		{
			name: "header with fewer than 16 columns returns fatal error",
			records: [][]string{
				{"Sport", "Grade", "Player Name"},
			},
			wantFatal: true,
		},
		{
			name: "header only, no data rows returns empty results",
			records: [][]string{
				validHeader,
			},
			wantRows: nil,
		},
		{
			name: "happy path: single valid row",
			records: [][]string{
				validHeader,
				makeMMRow("12345678", "99.50"),
			},
			wantRows: []MMRefreshRow{
				{CertNumber: "12345678", LastSalePrice: 99.50},
			},
		},
		{
			name: "happy path: multiple valid rows",
			records: [][]string{
				validHeader,
				makeMMRow("11111111", "25.00"),
				makeMMRow("22222222", "150.75"),
				makeMMRow("33333333", "0"),
			},
			wantRows: []MMRefreshRow{
				{CertNumber: "11111111", LastSalePrice: 25.00},
				{CertNumber: "22222222", LastSalePrice: 150.75},
				{CertNumber: "33333333", LastSalePrice: 0},
			},
		},
		{
			name: "row with empty cert and empty price is silently skipped",
			records: [][]string{
				validHeader,
				makeMMRow("", ""),
				makeMMRow("55555555", "10.00"),
			},
			wantRows: []MMRefreshRow{
				{CertNumber: "55555555", LastSalePrice: 10.00},
			},
		},
		{
			name: "row with empty cert but non-empty price produces parse error",
			records: [][]string{
				validHeader,
				makeMMRow("", "45.00"),
			},
			wantErrMsgs: []string{"missing cert number"},
		},
		{
			name: "row with invalid price format produces parse error",
			records: [][]string{
				validHeader,
				makeMMRow("12345678", "not-a-number"),
			},
			wantErrMsgs: []string{"invalid Last Sale Price"},
		},
		{
			name: "row with too few columns produces parse error",
			records: [][]string{
				validHeader,
				{"Sport", "Grade", "Player"},
			},
			wantErrMsgs: []string{"too few columns"},
		},
		{
			name: "cert number is whitespace-trimmed",
			records: [][]string{
				validHeader,
				makeMMRow("  99887766  ", "  12.50  "),
			},
			wantRows: []MMRefreshRow{
				{CertNumber: "99887766", LastSalePrice: 12.50},
			},
		},
		{
			name: "mix of valid and invalid rows",
			records: [][]string{
				validHeader,
				makeMMRow("11111111", "10.00"),
				makeMMRow("", "20.00"),       // missing cert → parse error
				makeMMRow("33333333", "bad"), // bad price → parse error
				{"too", "few"},               // too few columns → parse error
				makeMMRow("55555555", "30.00"),
			},
			wantRows: []MMRefreshRow{
				{CertNumber: "11111111", LastSalePrice: 10.00},
				{CertNumber: "55555555", LastSalePrice: 30.00},
			},
			wantErrMsgs: []string{"missing cert number", "invalid Last Sale Price", "too few columns"},
		},
		{
			name: "row with cert but no price is accepted with zero price",
			records: [][]string{
				validHeader,
				makeMMRow("12345678", ""),
			},
			wantRows: []MMRefreshRow{
				{CertNumber: "12345678", LastSalePrice: 0},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, parseErrors, err := ParseMMRefreshRows(tc.records)

			if tc.wantFatal {
				if err == nil {
					t.Fatal("expected fatal error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected fatal error: %v", err)
			}

			// Verify expected rows
			if len(rows) != len(tc.wantRows) {
				t.Fatalf("got %d rows, want %d", len(rows), len(tc.wantRows))
			}
			for i, want := range tc.wantRows {
				got := rows[i]
				if got.CertNumber != want.CertNumber {
					t.Errorf("row[%d].CertNumber = %q, want %q", i, got.CertNumber, want.CertNumber)
				}
				if got.LastSalePrice != want.LastSalePrice {
					t.Errorf("row[%d].LastSalePrice = %v, want %v", i, got.LastSalePrice, want.LastSalePrice)
				}
			}

			// Verify expected parse error messages
			if len(parseErrors) != len(tc.wantErrMsgs) {
				t.Fatalf("got %d parse errors, want %d: %v", len(parseErrors), len(tc.wantErrMsgs), parseErrors)
			}
			for i, wantMsg := range tc.wantErrMsgs {
				if !strings.Contains(parseErrors[i].Message, wantMsg) {
					t.Errorf("parseErrors[%d].Message = %q, want substring %q", i, parseErrors[i].Message, wantMsg)
				}
			}
		})
	}
}

// makeMMRow builds a 17-column MM CSV record with the cert in column 12 (index 11)
// and the price in column 16 (index 15). All other columns are empty.
func makeMMRow(cert, price string) []string {
	row := make([]string, 17)
	row[11] = cert
	row[15] = price
	return row
}
