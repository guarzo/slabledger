package inventory

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// EbayOrderRow represents a single row parsed from an eBay sales report CSV.
type EbayOrderRow struct {
	OrderNumber    string
	Date           string // YYYY-MM-DD
	ProductTitle   string
	SalePriceCents int
	// Exactly one of these will be set based on Custom Label format:
	DHInventoryID int    // From "DH-NNNNN" custom labels
	CertNumber    string // From "PSA-NNNNN" custom labels
}

// ParseEbayOrderRows parses CSV records from an eBay File Exchange sales report.
// Row 0 is blank, row 1 is the header, row 2 is blank, data starts at row 3.
func ParseEbayOrderRows(records [][]string) ([]EbayOrderRow, []OrdersImportSkip, error) {
	if len(records) < 3 {
		return nil, nil, fmt.Errorf("eBay CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[1])
	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	required := []string{"custom label", "item title", "sold for", "sale date", "order number"}
	for _, col := range required {
		if _, ok := headerMap[col]; !ok {
			return nil, nil, fmt.Errorf("eBay CSV is missing required column: %s", col)
		}
	}

	var rows []EbayOrderRow
	var skipped []OrdersImportSkip

	for _, rec := range records[2:] {
		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		orderNumber := getField(colIdx("order number"))
		customLabel := getField(colIdx("custom label"))
		title := getField(colIdx("item title"))
		priceRaw := getField(colIdx("sold for"))
		dateRaw := getField(colIdx("sale date"))

		if orderNumber == "" || title == "" {
			continue
		}

		var dhInvID int
		var certNumber string

		switch {
		case strings.HasPrefix(strings.ToUpper(customLabel), "DH-"):
			id, err := strconv.Atoi(customLabel[3:])
			if err != nil {
				skipped = append(skipped, OrdersImportSkip{
					ProductTitle: title,
					Reason:       fmt.Sprintf("invalid_dh_id: %s", customLabel),
				})
				continue
			}
			dhInvID = id
		case isEbayCertLabel(customLabel):
			certNumber = NormalizePSACert(customLabel[4:]) // strip "PSA-"
			if certNumber == "" {
				skipped = append(skipped, OrdersImportSkip{
					ProductTitle: title,
					Reason:       fmt.Sprintf("invalid_cert: %s", customLabel),
				})
				continue
			}
		default:
			skipped = append(skipped, OrdersImportSkip{
				ProductTitle: title,
				Reason:       "no_identifier",
			})
			continue
		}

		price, err := ParseCurrencyString(priceRaw)
		if err != nil {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   certNumber,
				ProductTitle: title,
				Reason:       fmt.Sprintf("invalid_price: %s", priceRaw),
			})
			continue
		}

		date, err := parseEbayDate(dateRaw)
		if err != nil {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   certNumber,
				ProductTitle: title,
				Reason:       fmt.Sprintf("invalid_date: %s", dateRaw),
			})
			continue
		}

		rows = append(rows, EbayOrderRow{
			OrderNumber:    orderNumber,
			Date:           date,
			ProductTitle:   title,
			SalePriceCents: mathutil.ToCentsInt(price),
			DHInventoryID:  dhInvID,
			CertNumber:     certNumber,
		})
	}

	return rows, skipped, nil
}

// isEbayCertLabel returns true for "PSA-NNNNN" style labels (pure digits after "PSA-").
func isEbayCertLabel(label string) bool {
	upper := strings.ToUpper(label)
	if !strings.HasPrefix(upper, "PSA-") {
		return false
	}
	rest := label[4:]
	if rest == "" {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseEbayDate converts "Jan-02-06" style eBay dates to "2006-01-02".
func parseEbayDate(raw string) (string, error) {
	t, err := time.Parse("Jan-02-06", raw)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}
