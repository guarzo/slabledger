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

// IsForcedReason reports whether a sale reason represents a forced liquidation.
// This is the single Go definition of the rule mirrored by the
// campaign_sales_derive_reason trigger (migration 000022); the SQL copy exists
// only for the rollback window, when no new-image Go code is running.
func IsForcedReason(reason string) bool { return reason == SaleReasonInvoicePressure }
