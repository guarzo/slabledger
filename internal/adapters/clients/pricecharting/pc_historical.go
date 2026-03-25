package pricecharting

import (
	"context"
	"fmt"
)

// EnrichWithHistoricalData is a no-op. PriceCharting's API docs confirm:
// "Historic prices and historic sales are not supported." The history endpoint
// returns 404 and sales-data is never populated. Kept as a no-op to avoid
// breaking callers; the method will be removed in a future cleanup.
func (p *PriceCharting) EnrichWithHistoricalData(_ context.Context, match *PCMatch) error {
	if match == nil || match.ID == "" {
		return fmt.Errorf("invalid match for historical enrichment")
	}
	return nil
}
