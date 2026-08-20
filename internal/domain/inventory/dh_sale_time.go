package inventory

import "time"

// dhSoldAtDateLayout is the shape both Sale.SaleDate and Purchase.PurchaseDate
// are stored in. Neither ValidateSale nor ValidatePurchase enforces this shape
// (both check only non-emptiness), so malformed values already exist in the
// table and DeriveDHSoldAt must degrade gracefully rather than panic or error.
const dhSoldAtDateLayout = "2006-01-02"

// DeriveDHSoldAt implements the sold_at derivation from design doc §2:
// sold_at = clamp(saleDate, lower = purchaseDate, upper = createdAt).
//
// This exists to keep the DH sale-recording request body a pure function of
// persisted columns -- never the wall clock -- so a retry under the same
// idempotency key resends a byte-identical body. createdAt is the upper
// bound (not "now") for the same reason: it is stored and stable across
// retries, whereas "now" is not.
//
// On a malformed saleDate, the result falls back to createdAt entirely (there
// is no better single instant to offer DH). On a malformed purchaseDate, only
// the lower clamp is omitted -- the parsed saleDate still passes through the
// upper clamp.
//
// The result is always .UTC(): createdAt may arrive in any location, and
// campaign_sales.created_at is a timezone-less column, so failing to normalise
// here would let the same stored instant produce two different sold_at values
// depending on the process's local zone -- a silent idempotency-key collision.
func DeriveDHSoldAt(saleDate, purchaseDate string, createdAt time.Time) time.Time {
	createdAt = createdAt.UTC()

	sale, err := time.Parse(dhSoldAtDateLayout, saleDate)
	if err != nil {
		return createdAt
	}
	sale = sale.UTC()

	if purchase, err := time.Parse(dhSoldAtDateLayout, purchaseDate); err == nil {
		purchase = purchase.UTC()
		if sale.Before(purchase) {
			sale = purchase
		}
	}

	if sale.After(createdAt) {
		sale = createdAt
	}

	return sale.UTC()
}
