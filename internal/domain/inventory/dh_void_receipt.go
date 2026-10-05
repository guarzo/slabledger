package inventory

import "context"

// DHVerifiedSaleVoider acknowledges the captured target, not only a swallowed
// compatibility error. A missing capability leaves the journal open while the
// ordinary generic un-sell may still remove the local sale.
type DHVerifiedSaleVoider interface {
	VoidInventorySaleVerified(context.Context, string, int, string) error
}

func validateRecoveredSale(result *DHSaleResult, target int) bool {
	return result != nil && result.DHSaleID != "" && result.Delisted && (result.SoldInventoryID == nil && !result.Replayed || result.SoldInventoryID != nil && *result.SoldInventoryID == target)
}
