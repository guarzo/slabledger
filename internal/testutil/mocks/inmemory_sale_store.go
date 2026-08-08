package mocks

import (
	"context"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- SaleRepository ---

func (m *InMemoryCampaignStore) CreateSale(ctx context.Context, s *inventory.Sale) error {
	if m.CreateSaleFn != nil {
		return m.CreateSaleFn(ctx, s)
	}
	if m.PurchaseSales[s.PurchaseID] {
		return inventory.ErrDuplicateSale
	}
	m.Sales[s.ID] = s
	m.PurchaseSales[s.PurchaseID] = true
	return nil
}

func (m *InMemoryCampaignStore) GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*inventory.Sale, error) {
	if m.GetSaleByPurchaseIDFn != nil {
		return m.GetSaleByPurchaseIDFn(ctx, purchaseID)
	}
	for _, s := range m.Sales {
		if s.PurchaseID == purchaseID {
			return s, nil
		}
	}
	return nil, inventory.ErrSaleNotFound
}

func (m *InMemoryCampaignStore) GetSalesByPurchaseIDs(_ context.Context, purchaseIDs []string) (map[string]*inventory.Sale, error) {
	result := make(map[string]*inventory.Sale, len(purchaseIDs))
	for _, pid := range purchaseIDs {
		for _, s := range m.Sales {
			if s.PurchaseID == pid {
				result[pid] = s
				break
			}
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) DeleteSale(ctx context.Context, saleID string) error {
	if m.DeleteSaleFn != nil {
		return m.DeleteSaleFn(ctx, saleID)
	}
	s, ok := m.Sales[saleID]
	if !ok {
		return inventory.ErrSaleNotFound
	}
	delete(m.PurchaseSales, s.PurchaseID)
	delete(m.Sales, saleID)
	return nil
}

func (m *InMemoryCampaignStore) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	if m.DeleteSaleByPurchaseIDFn != nil {
		return m.DeleteSaleByPurchaseIDFn(ctx, purchaseID)
	}
	for id, s := range m.Sales {
		if s.PurchaseID == purchaseID {
			delete(m.PurchaseSales, purchaseID)
			delete(m.Sales, id)
			return nil
		}
	}
	return inventory.ErrSaleNotFound
}

func (m *InMemoryCampaignStore) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string, forcedLiquidation bool) error {
	if m.UpdateSaleReasonFn != nil {
		return m.UpdateSaleReasonFn(ctx, campaignID, saleID, reason, forcedLiquidation)
	}
	s, ok := m.Sales[saleID]
	if !ok {
		return inventory.ErrSaleNotFound
	}
	p, ok := m.Purchases[s.PurchaseID]
	if !ok || p.CampaignID != campaignID {
		return inventory.ErrSaleNotFound
	}
	s.SaleReason = reason
	s.ForcedLiquidation = forcedLiquidation
	return nil
}

func (m *InMemoryCampaignStore) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error) {
	if m.ListSalesByCampaignFn != nil {
		return m.ListSalesByCampaignFn(ctx, campaignID, limit, offset)
	}
	saleIDs := make([]string, 0, len(m.Sales))
	for id := range m.Sales {
		saleIDs = append(saleIDs, id)
	}
	sort.Strings(saleIDs)
	var result []inventory.Sale
	for _, id := range saleIDs {
		s := m.Sales[id]
		if p, ok := m.Purchases[s.PurchaseID]; ok && p.CampaignID == campaignID {
			result = append(result, *s)
		}
	}
	if offset > len(result) {
		return nil, nil
	}
	result = result[offset:]
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}
