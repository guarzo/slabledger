package psaportal

import (
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Lightdash "Itemized Purchases" field keys (confirmed via live spike — see
// docs/private/2026-06-22-psa-portal-spike-findings.md).
const (
	colCert       = "fct_instantoffers_offers_cert_number"
	colTitle      = "marketplace_listings_listing_title"
	colGrade      = "fct_instantoffers_offers_grade_value"
	colPricePaid  = "marketplace_listings_total_listing_final_price_metric"
	colSource     = "fct_instantoffers_offers_origination_source"
	colDate       = "marketplace_listings_buyer_payment_date_pst_day"
	colShipDate   = "vault_withdrawal_items_shipment_date_day"
	colRefunded   = "fct_instantoffers_offers_is_offer_refunded"
	colCategory   = "dim_ims_inventory_set_sport_detailed"
	colFrontImage = "dim_ims_inventory_front_image_url"
	colBackImage  = "dim_ims_inventory_back_image_url"
)

// mapRow converts one flattened Lightdash row into a PSAExportRow.
//
// The tile has no invoice-date dimension. PSA invoices on a fixed cadence (the 1st
// and 15th of each month), so InvoiceDate is derived from the buyer payment date as
// the next 1st-or-15th on or after it (see invoiceDateFor).
func mapRow(r map[string]string) (inventory.PSAExportRow, error) {
	row := inventory.PSAExportRow{
		CertNumber:     inventory.NormalizePSACert(r[colCert]),
		ListingTitle:   r[colTitle],
		PurchaseSource: r[colSource],
		Category:       r[colCategory],
		Date:           normalizeDate(r[colDate]),
		ShipDate:       normalizeDate(r[colShipDate]),
		InvoiceDate:    invoiceDateFor(r[colDate]),
		FrontImageURL:  r[colFrontImage],
		BackImageURL:   r[colBackImage],
		WasRefunded:    isTruthy(r[colRefunded]),
	}
	if g := r[colGrade]; g != "" {
		v, err := strconv.ParseFloat(g, 64)
		if err != nil {
			return row, err
		}
		row.Grade = v
	}
	if p := r[colPricePaid]; p != "" {
		v, err := inventory.ParseCurrencyString(p)
		if err != nil {
			return row, err
		}
		row.PricePaid = v
	}
	return row, nil
}

// normalizeDate keeps only the calendar date (YYYY-MM-DD). Lightdash day-granularity
// dimensions arrive as RFC3339 (e.g. "2026-07-06T00:00:00Z"); the old sheet used
// YYYY-MM-DD, so downstream import expects that. Unknown formats pass through.
func normalizeDate(s string) string {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

// invoiceDateFor derives the PSA invoice date from a payment date: the next 1st or
// 15th of the month on or after the payment date. Empty/unparseable input → "".
func invoiceDateFor(paymentDate string) string {
	d := normalizeDate(paymentDate)
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return ""
	}
	var inv time.Time
	switch day := t.Day(); {
	case day == 1:
		inv = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	case day <= 15:
		inv = time.Date(t.Year(), t.Month(), 15, 0, 0, 0, 0, time.UTC)
	default:
		inv = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	}
	return inv.Format("2006-01-02")
}

func isTruthy(s string) bool {
	switch s {
	case "true", "True", "TRUE", "1", "yes", "Yes":
		return true
	}
	return false
}
