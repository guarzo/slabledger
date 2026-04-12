package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// SaleRepositoryMock implements inventory.SaleRepository with Fn-field pattern.
type SaleRepositoryMock struct {
	CreateSaleFn             func(ctx context.Context, s *inventory.Sale) error
	GetSaleByPurchaseIDFn    func(ctx context.Context, purchaseID string) (*inventory.Sale, error)
	GetSalesByPurchaseIDsFn  func(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Sale, error)
	ListSalesByCampaignFn    func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error)
	DeleteSaleFn             func(ctx context.Context, saleID string) error
	DeleteSaleByPurchaseIDFn func(ctx context.Context, purchaseID string) error
}

var _ inventory.SaleRepository = (*SaleRepositoryMock)(nil)

func (m *SaleRepositoryMock) CreateSale(ctx context.Context, s *inventory.Sale) error {
	if m.CreateSaleFn != nil {
		return m.CreateSaleFn(ctx, s)
	}
	return nil
}

func (m *SaleRepositoryMock) GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*inventory.Sale, error) {
	if m.GetSaleByPurchaseIDFn != nil {
		return m.GetSaleByPurchaseIDFn(ctx, purchaseID)
	}
	return nil, inventory.ErrSaleNotFound
}

func (m *SaleRepositoryMock) GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Sale, error) {
	if m.GetSalesByPurchaseIDsFn != nil {
		return m.GetSalesByPurchaseIDsFn(ctx, purchaseIDs)
	}
	return map[string]*inventory.Sale{}, nil
}

func (m *SaleRepositoryMock) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error) {
	if m.ListSalesByCampaignFn != nil {
		return m.ListSalesByCampaignFn(ctx, campaignID, limit, offset)
	}
	return []inventory.Sale{}, nil
}

func (m *SaleRepositoryMock) DeleteSale(ctx context.Context, saleID string) error {
	if m.DeleteSaleFn != nil {
		return m.DeleteSaleFn(ctx, saleID)
	}
	return nil
}

func (m *SaleRepositoryMock) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	if m.DeleteSaleByPurchaseIDFn != nil {
		return m.DeleteSaleByPurchaseIDFn(ctx, purchaseID)
	}
	return nil
}
