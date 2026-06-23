package inventory

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

// ParsePSAExportRows parses CSV records from a PSA communication spreadsheet.
// Dynamically finds the header row by scanning for known PSA column names.
// Returns parsed rows, any parse errors, and a fatal error if the header row
// cannot be found. Used by the CSV upload path (HandleGlobalImportPSA).
func ParsePSAExportRows(records [][]string) ([]PSAExportRow, []ParseError, error) {
	headerIdx := FindPSAHeaderRow(records)
	if headerIdx < 0 {
		return nil, nil, fmt.Errorf(
			"could not find PSA header row (expected columns: cert number, listing title, grade)")
	}

	headerMap := BuildHeaderMap(records[headerIdx])
	dataRows := records[headerIdx+1:]

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	var psaRows []PSAExportRow
	var parseErrors []ParseError
	for i, rec := range dataRows {
		rowNum := headerIdx + 2 + i // 1-based row number for error reporting

		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		certNumber := NormalizePSACert(getField(colIdx("cert number")))
		if certNumber == "" {
			continue // Skip empty template rows
		}

		var pricePaid float64
		if pp := getField(colIdx("price paid")); pp != "" {
			var parseErr error
			pricePaid, parseErr = ParseCurrencyString(pp)
			if parseErr != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowNum,
					Field:   "price paid",
					Message: fmt.Sprintf("Row %d: invalid price paid %q: %v", rowNum, pp, parseErr),
				})
				continue
			}
		}

		var grade float64
		if g := getField(colIdx("grade")); g != "" {
			var parseErr error
			grade, parseErr = strconv.ParseFloat(g, 64)
			if parseErr != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowNum,
					Field:   "grade",
					Message: fmt.Sprintf("Row %d: invalid grade %q: %v", rowNum, g, parseErr),
				})
				continue
			}
		}

		dateStr := getField(colIdx("date"))
		purchaseDate := ""
		if dateStr != "" {
			converted, dateErr := ParsePSADate(dateStr)
			if dateErr != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowNum,
					Field:   "date",
					Message: fmt.Sprintf("Row %d: invalid date %q: %v", rowNum, dateStr, dateErr),
				})
				continue
			}
			purchaseDate = converted
		}

		invoiceDateStr := getField(colIdx("invoice date"))
		invoiceDate := ""
		if invoiceDateStr != "" {
			converted, dateErr := ParsePSADate(invoiceDateStr)
			if dateErr != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowNum,
					Field:   "invoice date",
					Message: fmt.Sprintf("Row %d: invalid invoice date %q: %v", rowNum, invoiceDateStr, dateErr),
				})
				continue
			}
			invoiceDate = converted
		}

		shipDateStr := getField(colIdx("ship date"))
		shipDate := ""
		if shipDateStr != "" {
			converted, dateErr := ParsePSADate(shipDateStr)
			if dateErr != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowNum,
					Field:   "ship date",
					Message: fmt.Sprintf("Row %d: invalid ship date %q: %v", rowNum, shipDateStr, dateErr),
				})
				continue
			}
			shipDate = converted
		}

		wasRefunded := false
		refundedStr := strings.ToLower(getField(colIdx("was refunded?")))
		if refundedStr == "yes" || refundedStr == "true" || refundedStr == "1" {
			wasRefunded = true
		}

		psaRows = append(psaRows, PSAExportRow{
			Date:           purchaseDate,
			Category:       getField(colIdx("category")),
			CertNumber:     certNumber,
			ListingTitle:   getField(colIdx("listing title")),
			Grade:          grade,
			PricePaid:      pricePaid,
			PurchaseSource: getField(colIdx("purchase source")),
			ShipDate:       shipDate,
			InvoiceDate:    invoiceDate,
			WasRefunded:    wasRefunded,
			FrontImageURL:  getField(colIdx("front image url")),
			BackImageURL:   getField(colIdx("back image url")),
		})
	}

	return psaRows, parseErrors, nil
}
