package inventory

const (
	SaleReasonDiscretionary   = "discretionary"
	SaleReasonInvoicePressure = "invoice_pressure"
	SaleReasonAgingPolicy     = "aging_policy"
	SaleReasonBulkLot         = "bulk_lot"
	SaleReasonShowClearout    = "show_clearout"
)

var validSaleReasons = map[string]bool{
	SaleReasonDiscretionary:   true,
	SaleReasonInvoicePressure: true,
	SaleReasonAgingPolicy:     true,
	SaleReasonBulkLot:         true,
	SaleReasonShowClearout:    true,
}

// ValidSaleReason allows the five reasons plus "" (unknown/legacy).
func ValidSaleReason(s string) bool { return s == "" || validSaleReasons[s] }

// ValidSaleReasonForPatch allows only the five explicit reasons (rejects "").
func ValidSaleReasonForPatch(s string) bool { return validSaleReasons[s] }
