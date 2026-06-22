package psaportal

import (
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Lightdash "Itemized Purchases" field keys (confirmed via live spike — see SPIKE_FINDINGS.md).
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
// InvoiceDate is intentionally left empty: the Itemized Purchases tile has no
// invoice-date dimension (see SPIKE_FINDINGS.md "GAP"); revisit once real rows exist.
func mapRow(r map[string]string) (inventory.PSAExportRow, error) {
	row := inventory.PSAExportRow{
		CertNumber:     inventory.NormalizePSACert(r[colCert]),
		ListingTitle:   r[colTitle],
		PurchaseSource: r[colSource],
		Category:       r[colCategory],
		Date:           normalizeDate(r[colDate]),
		ShipDate:       normalizeDate(r[colShipDate]),
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

// normalizeDate converts the portal's date string to YYYY-MM-DD.
// Spike saw 0 rows, so the raw format is unconfirmed; the "_day" granularity
// dimensions are expected to already be YYYY-MM-DD, so this is a passthrough for
// now. TODO: confirm + reformat once real rows exist.
func normalizeDate(s string) string { return s }

func isTruthy(s string) bool {
	switch s {
	case "true", "True", "TRUE", "1", "yes", "Yes":
		return true
	}
	return false
}
