package mocks

import (
	"context"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- PurchaseRepository: CRUD and listing ---

func (m *InMemoryCampaignStore) CreatePurchase(ctx context.Context, p *inventory.Purchase) error {
	if m.CreatePurchaseFn != nil {
		return m.CreatePurchaseFn(ctx, p)
	}
	if p.Grader == "" {
		p.Grader = "PSA"
	}
	if m.CertNumbers[p.CertNumber] {
		return inventory.ErrDuplicateCertNumber
	}
	m.Purchases[p.ID] = p
	m.CertNumbers[p.CertNumber] = true
	return nil
}

func (m *InMemoryCampaignStore) GetPurchase(ctx context.Context, id string) (*inventory.Purchase, error) {
	if m.GetPurchaseFn != nil {
		return m.GetPurchaseFn(ctx, id)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return nil, inventory.ErrPurchaseNotFound
	}
	return p, nil
}

func (m *InMemoryCampaignStore) DeletePurchase(ctx context.Context, id string) error {
	if m.DeletePurchaseFn != nil {
		return m.DeletePurchaseFn(ctx, id)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	delete(m.CertNumbers, p.CertNumber)
	delete(m.PurchaseSales, id)
	for sid, s := range m.Sales {
		if s.PurchaseID == id {
			delete(m.Sales, sid)
		}
	}
	delete(m.Purchases, id)
	return nil
}

func (m *InMemoryCampaignStore) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error) {
	if m.ListPurchasesByCampaignFn != nil {
		return m.ListPurchasesByCampaignFn(ctx, campaignID, limit, offset)
	}
	ids := make([]string, 0, len(m.Purchases))
	for id := range m.Purchases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var result []inventory.Purchase
	for _, id := range ids {
		p := m.Purchases[id]
		if p.CampaignID == campaignID {
			result = append(result, *p)
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

func (m *InMemoryCampaignStore) ListUnsoldPurchases(ctx context.Context, campaignID string) ([]inventory.Purchase, error) {
	if m.ListUnsoldPurchasesFn != nil {
		return m.ListUnsoldPurchasesFn(ctx, campaignID)
	}
	var result []inventory.Purchase
	for _, p := range m.Purchases {
		if p.CampaignID == campaignID && !m.PurchaseSales[p.ID] {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	if m.ListAllUnsoldPurchasesFn != nil {
		return m.ListAllUnsoldPurchasesFn(ctx)
	}
	var result []inventory.Purchase
	for _, p := range m.Purchases {
		if !m.PurchaseSales[p.ID] {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) CountPurchasesByCampaign(ctx context.Context, campaignID string) (int, error) {
	if m.CountPurchasesByCampaignFn != nil {
		return m.CountPurchasesByCampaignFn(ctx, campaignID)
	}
	count := 0
	for _, p := range m.Purchases {
		if p.CampaignID == campaignID {
			count++
		}
	}
	return count, nil
}
