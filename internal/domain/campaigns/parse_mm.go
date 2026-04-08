package campaigns

import (
	"fmt"
	"strconv"
	"strings"
)

// mmExportHeaderCol0 is the expected first column header of the Market Movers 17-column CSV.
const mmExportHeaderCol0 = "Sport"

// ParseMMRefreshRows parses a Market Movers collection export CSV (17 columns)
// and extracts cert number (Notes, col 12) and last sale price (col 16) for each data row.
// Returns rows, any per-row parse warnings, and a fatal error if the file format is wrong.
func ParseMMRefreshRows(records [][]string) ([]MMRefreshRow, []ParseError, error) {
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("empty CSV")
	}

	// Validate header
	header := records[0]
	if len(header) < 16 || strings.TrimSpace(header[0]) != mmExportHeaderCol0 {
		return nil, nil, fmt.Errorf("unrecognised CSV format: expected Market Movers 17-column export (first header column %q)", strings.TrimSpace(header[0]))
	}

	var rows []MMRefreshRow
	var parseErrors []ParseError

	for i, rec := range records[1:] {
		rowNum := i + 2
		if len(rec) < 16 {
			parseErrors = append(parseErrors, ParseError{Row: rowNum, Message: "too few columns"})
			continue
		}

		certNumber := strings.TrimSpace(rec[11]) // col 12 (0-indexed: 11)
		rawPrice := strings.TrimSpace(rec[15])   // col 16 (0-indexed: 15)

		if certNumber == "" && rawPrice == "" {
			continue // blank data row — skip silently
		}

		if certNumber == "" {
			parseErrors = append(parseErrors, ParseError{Row: rowNum, Message: "missing cert number in Notes column"})
			continue
		}

		var lastSalePrice float64
		if rawPrice != "" {
			var err error
			lastSalePrice, err = strconv.ParseFloat(rawPrice, 64)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{Row: rowNum, Message: fmt.Sprintf("invalid Last Sale Price %q: %v", rawPrice, err)})
				continue
			}
		}

		rows = append(rows, MMRefreshRow{
			CertNumber:    certNumber,
			LastSalePrice: lastSalePrice,
		})
	}

	return rows, parseErrors, nil
}
