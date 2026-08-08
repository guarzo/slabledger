package mocks

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- PurchaseRepository: cert and bulk-key lookups ---

func (m *InMemoryCampaignStore) GetPurchaseByCertNumber(ctx context.Context, grader, certNumber string) (*inventory.Purchase, error) {
	if m.GetPurchaseByCertNumberFn != nil {
		return m.GetPurchaseByCertNumberFn(ctx, grader, certNumber)
	}
	for _, p := range m.Purchases {
		if p.Grader == grader && p.CertNumber == certNumber {
			return p, nil
		}
	}
	return nil, inventory.ErrPurchaseNotFound
}

func (m *InMemoryCampaignStore) GetPurchasesByGraderAndCertNumbers(_ context.Context, grader string, certNumbers []string) (map[string]*inventory.Purchase, error) {
	result := make(map[string]*inventory.Purchase, len(certNumbers))
	certSet := make(map[string]bool, len(certNumbers))
	for _, cn := range certNumbers {
		certSet[cn] = true
	}
	for _, p := range m.Purchases {
		if p.Grader == grader && certSet[p.CertNumber] {
			result[p.CertNumber] = p
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if m.GetPurchasesByCertNumbersFn != nil {
		return m.GetPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	result := make(map[string]*inventory.Purchase, len(certNumbers))
	certSet := make(map[string]bool, len(certNumbers))
	for _, cn := range certNumbers {
		certSet[cn] = true
	}
	for _, p := range m.Purchases {
		if certSet[p.CertNumber] {
			result[p.CertNumber] = p
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetPurchasesByDHInventoryIDs(ctx context.Context, dhIDs []int) (map[int]*inventory.Purchase, error) {
	if m.GetPurchasesByDHInventoryIDsFn != nil {
		return m.GetPurchasesByDHInventoryIDsFn(ctx, dhIDs)
	}
	result := make(map[int]*inventory.Purchase, len(dhIDs))
	idSet := make(map[int]bool, len(dhIDs))
	for _, id := range dhIDs {
		idSet[id] = true
	}
	for _, p := range m.Purchases {
		if idSet[p.DHInventoryID] {
			if _, exists := result[p.DHInventoryID]; exists {
				return nil, fmt.Errorf("duplicate dh_inventory_id %d", p.DHInventoryID)
			}
			result[p.DHInventoryID] = p
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetPurchasesByIDs(_ context.Context, ids []string) (map[string]*inventory.Purchase, error) {
	result := make(map[string]*inventory.Purchase, len(ids))
	for _, id := range ids {
		if p, ok := m.Purchases[id]; ok {
			result[id] = p
		}
	}
	return result, nil
}
