package inventory

import (
	"testing"
)

func TestBuildHeaderMap(t *testing.T) {
	header := []string{"Cert Number", "  Listing Title  ", "GRADE", "Price Paid", "Category"}
	m := BuildHeaderMap(header)

	expected := map[string]int{
		"cert number":   0,
		"listing title": 1,
		"grade":         2,
		"price paid":    3,
		"category":      4,
	}

	for key, wantIdx := range expected {
		if gotIdx, ok := m[key]; !ok {
			t.Errorf("BuildHeaderMap: expected key %q not found in map", key)
		} else if gotIdx != wantIdx {
			t.Errorf("BuildHeaderMap: key %q got index %d, want %d", key, gotIdx, wantIdx)
		}
	}

	if len(m) != len(expected) {
		t.Errorf("BuildHeaderMap: map length %d, want %d", len(m), len(expected))
	}
}

func TestFindPSAHeaderRow_Found(t *testing.T) {
	rows := [][]string{
		{"junk data", "more junk", "ignored"},
		{"Cert Number", "Listing Title", "Grade", "Price Paid", "Category"},
		{"12345678", "Pikachu PSA 10", "10", "$50.00", "Pokemon"},
	}

	got := FindPSAHeaderRow(rows)
	if got != 1 {
		t.Errorf("FindPSAHeaderRow: got %d, want 1", got)
	}
}

func TestFindPSAHeaderRow_NotFound(t *testing.T) {
	rows := [][]string{
		{"Name", "Price", "Status"},
		{"Card A", "100", "Active"},
		{"Card B", "200", "Sold"},
	}

	got := FindPSAHeaderRow(rows)
	if got != -1 {
		t.Errorf("FindPSAHeaderRow: got %d, want -1", got)
	}
}

func TestNormalizePSACert(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain digits", "192060238", "192060238"},
		{"PSA prefix", "PSA-192060238", "192060238"},
		{"PSA prefix lowercase", "psa-192060238", "192060238"},
		{"whitespace around digits", "  99887766  ", "99887766"},
		{"whitespace around PSA prefix", "  PSA-12345678  ", "12345678"},
		{"empty string", "", ""},
		{"non-numeric no PSA prefix", "CGC-12345", ""},
		{"non-numeric text only", "some-card-name", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePSACert(tc.input)
			if got != tc.want {
				t.Errorf("NormalizePSACert(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseCurrencyString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"dollar sign with commas", "$1,234.56", 1234.56, false},
		{"plain decimal", "100.00", 100.00, false},
		{"dollar sign small", "$0.99", 0.99, false},
		{"no dollar sign no comma", "42.5", 42.5, false},
		{"empty string", "", 0, true},
		{"whitespace only", "   ", 0, true},
		{"non-numeric", "abc", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCurrencyString(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseCurrencyString(%q): expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseCurrencyString(%q): unexpected error: %v", tc.input, err)
				return
			}
			if got != tc.want {
				t.Errorf("ParseCurrencyString(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestParsePSAExportRows_MultipleBadFieldsPerRow verifies a row with more than
// one invalid field surfaces every field error (not just the first) before the
// row is skipped, so users don't fix-and-reupload one error at a time.
func TestParsePSAExportRows_MultipleBadFieldsPerRow(t *testing.T) {
	records := [][]string{
		{"cert number", "listing title", "grade", "price paid", "date"},
		// Both grade and price paid are invalid on this one row.
		{"12345678", "Charizard", "notagrade", "notaprice", ""},
	}

	rows, parseErrors, err := ParsePSAExportRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected the malformed row to be skipped, got %d rows", len(rows))
	}

	gotFields := map[string]bool{}
	for _, pe := range parseErrors {
		gotFields[pe.Field] = true
	}
	for _, want := range []string{"price paid", "grade"} {
		if !gotFields[want] {
			t.Errorf("expected an error for field %q, got errors: %+v", want, parseErrors)
		}
	}
}

// TestParsePSAExportRows_ValidRow is a sanity check that a well-formed row maps
// through cleanly with no parse errors.
func TestParsePSAExportRows_ValidRow(t *testing.T) {
	records := [][]string{
		{"cert number", "listing title", "grade", "price paid"},
		{"12345678", "Charizard", "10", "$125.00"},
	}

	rows, parseErrors, err := ParsePSAExportRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrors) != 0 {
		t.Fatalf("expected no parse errors, got %+v", parseErrors)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CertNumber != "12345678" || rows[0].Grade != 10 || rows[0].PricePaid != 125.00 {
		t.Errorf("unexpected row mapping: %+v", rows[0])
	}
}
