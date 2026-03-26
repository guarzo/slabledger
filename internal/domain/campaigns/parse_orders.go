package campaigns

import (
	"fmt"
	"math"
	"strings"
)

// mapOrdersChannel maps CSV "Sales Channel" values to SaleChannel constants.
// Returns empty string for unknown channels.
func mapOrdersChannel(raw string) SaleChannel {
	switch strings.TrimSpace(raw) {
	case "eBay":
		return SaleChannelEbay
	case "Online Store":
		return SaleChannelWebsite
	default:
		return ""
	}
}

// ParseOrdersExportRows parses CSV records from an orders export.
// The first row must be the header row.
// Returns valid PSA rows, skipped rows (with reasons), and a fatal error
// if the CSV structure is invalid.
func ParseOrdersExportRows(records [][]string) ([]OrdersExportRow, []OrdersImportSkip, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])
	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	// Validate required columns
	required := []string{"date", "sales channel", "product title", "grading company", "cert number", "unit price"}
	for _, col := range required {
		if _, ok := headerMap[col]; !ok {
			return nil, nil, fmt.Errorf("CSV is missing required column: %s", col)
		}
	}

	seen := make(map[string]bool)
	var rows []OrdersExportRow
	var skipped []OrdersImportSkip

	for _, rec := range records[1:] {
		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		orderNumber := getField(colIdx("order"))
		date := getField(colIdx("date"))
		channelRaw := getField(colIdx("sales channel"))
		productTitle := getField(colIdx("product title"))
		grader := getField(colIdx("grading company"))
		certRaw := getField(colIdx("cert number"))
		gradeRaw := getField(colIdx("grade"))
		priceRaw := getField(colIdx("unit price"))

		// Filter: only PSA
		if !strings.EqualFold(grader, "PSA") {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   certRaw,
				ProductTitle: productTitle,
				Reason:       "not_psa",
			})
			continue
		}

		// Filter: must have cert number
		cert := NormalizePSACert(certRaw)
		if cert == "" {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   certRaw,
				ProductTitle: productTitle,
				Reason:       "no_cert",
			})
			continue
		}

		// Map channel
		channel := mapOrdersChannel(channelRaw)
		if channel == "" {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   cert,
				ProductTitle: productTitle,
				Reason:       "unknown_channel",
			})
			continue
		}

		// Deduplicate by cert — first occurrence wins
		if seen[cert] {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   cert,
				ProductTitle: productTitle,
				Reason:       "duplicate",
			})
			continue
		}
		seen[cert] = true

		// Parse price
		price, err := ParseCurrencyString(priceRaw)
		if err != nil {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   cert,
				ProductTitle: productTitle,
				Reason:       fmt.Sprintf("invalid_price: %s", priceRaw),
			})
			continue
		}

		// Parse grade (best-effort, 0 if unparseable)
		var grade float64
		if gradeRaw != "" {
			if v, err := ParseCurrencyString(gradeRaw); err == nil {
				grade = v
			}
		}

		rows = append(rows, OrdersExportRow{
			OrderNumber:  orderNumber,
			Date:         date,
			SalesChannel: channel,
			ProductTitle: productTitle,
			Grader:       "PSA",
			CertNumber:   cert,
			Grade:        grade,
			UnitPrice:    price,
		})
	}

	return rows, skipped, nil
}

// DollarsToCents converts a dollar amount to cents with rounding.
func DollarsToCents(dollars float64) int {
	return int(math.Round(dollars * 100))
}
