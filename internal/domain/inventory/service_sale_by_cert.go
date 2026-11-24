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

// DeleteExpectedSale un-sells a purchase only while its current sale is
// expectedSaleID, so a confirmation never deletes a sale it did not show.
// An empty expectedSaleID behaves like DeleteSaleByPurchaseID.
func (s *service) DeleteExpectedSale(ctx context.Context, purchaseID, expectedSaleID string) error {
	if expectedSaleID == "" {
		return s.DeleteSaleByPurchaseID(ctx, purchaseID)
	}
	if s.mutationRequired {
		return s.deleteSaleCoordinated(ctx, purchaseID, expectedSaleID)
	}
	sale, err := s.sales.GetSaleByPurchaseID(ctx, purchaseID)
	if err != nil {
		return err
	}
	if sale.ID != expectedSaleID {
		return NewReturnConflict("sale_precondition_failed", "current sale differs from confirmed sale")
	}
	return s.DeleteSaleByPurchaseID(ctx, purchaseID)
}
