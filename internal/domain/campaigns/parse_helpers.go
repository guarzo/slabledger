package campaigns

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// psaCertFromSKU extracts a PSA cert number from a SKU like "PSA-192060238".
var psaCertFromSKU = regexp.MustCompile(`(?i)^PSA-(\d+)$`)

// digitsOnly matches a string that is entirely digits.
var digitsOnly = regexp.MustCompile(`^\d+$`)

// BuildHeaderMap creates a lowercase header name -> column index map.
func BuildHeaderMap(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, col := range header {
		col = strings.TrimPrefix(col, "\uFEFF")
		m[strings.TrimSpace(strings.ToLower(col))] = i
	}
	return m
}

// FindPSAHeaderRow scans the first few rows for known PSA column names.
// Returns the header row index, or -1 if not found.
func FindPSAHeaderRow(rows [][]string) int {
	knownColumns := map[string]bool{
		"cert number":   true,
		"listing title": true,
		"grade":         true,
		"price paid":    true,
	}
	for i, row := range rows {
		if i > 5 { // Don't scan more than 6 rows
			break
		}
		headerMap := BuildHeaderMap(row)
		matches := 0
		for col := range knownColumns {
			if _, ok := headerMap[col]; ok {
				matches++
			}
		}
		if matches >= 3 { // At least 3 known columns found
			return i
		}
	}
	return -1
}

// NormalizePSACert returns a digits-only cert number from a raw field value.
// It handles plain digits, "PSA-XXXXX" format, and trims whitespace.
func NormalizePSACert(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if digitsOnly.MatchString(s) {
		return s
	}
	if m := psaCertFromSKU.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return ""
}

// ParseCurrencyString parses a currency string (e.g. "$1,234.56", "1234.56")
// into a float64 value. Handles whitespace trimming, optional "$" prefix,
// and comma removal.
func ParseCurrencyString(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, fmt.Errorf("empty currency string")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid currency value %q: %w", s, err)
	}
	return v, nil
}
