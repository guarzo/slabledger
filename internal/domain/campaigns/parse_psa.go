package campaigns

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsePSAExportRows parses CSV records from a PSA communication spreadsheet.
// Dynamically finds the header row by scanning for known PSA column names.
// Returns parsed rows, any parse errors, and a fatal error if the header row
// cannot be found.
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

		certNumber := getField(colIdx("cert number"))
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
			VaultStatus:    getField(colIdx("vault status")),
			InvoiceDate:    invoiceDate,
			WasRefunded:    wasRefunded,
			FrontImageURL:  getField(colIdx("front image url")),
			BackImageURL:   getField(colIdx("back image url")),
		})
	}

	return psaRows, parseErrors, nil
}
