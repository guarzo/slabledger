package inventory

import (
	"context"
	"errors"
	"fmt"
)

// SaleStatusByCert pairs a PSA purchase with its sale, if any. Sale is nil
// when the card is unsold.
type SaleStatusByCert struct {
	Purchase *Purchase
	Sale     *Sale
}

// GetSaleStatusByCert looks up the PSA purchase for a cert and its sale.
// Returns ErrPurchaseNotFound when no purchase has that cert.
func (s *service) GetSaleStatusByCert(ctx context.Context, certNumber string) (*SaleStatusByCert, error) {
	p, err := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", certNumber)
	if err != nil {
		return nil, err
	}
	sale, err := s.sales.GetSaleByPurchaseID(ctx, p.ID)
	if err != nil {
		if errors.Is(err, ErrSaleNotFound) {
			return &SaleStatusByCert{Purchase: p}, nil
		}
		return nil, fmt.Errorf("sale lookup: %w", err)
	}
	return &SaleStatusByCert{Purchase: p, Sale: sale}, nil
}
