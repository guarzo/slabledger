package mocks

import (
	"context"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- PurchaseRepository: receipt, eBay export flags, and snapshot status ---

func (m *InMemoryCampaignStore) SetReceivedAt(_ context.Context, purchaseID string, receivedAt time.Time) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	receivedAtStr := receivedAt.Format("2006-01-02T15:04:05Z07:00")
	p.ReceivedAt = &receivedAtStr
	return nil
}

func (m *InMemoryCampaignStore) SetEbayExportFlag(_ context.Context, purchaseID string, flaggedAt time.Time) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.EbayExportFlaggedAt = &flaggedAt
	return nil
}

func (m *InMemoryCampaignStore) ClearEbayExportFlags(_ context.Context, purchaseIDs []string) error {
	for _, id := range purchaseIDs {
		if p, ok := m.Purchases[id]; ok {
			p.EbayExportFlaggedAt = nil
		}
	}
	return nil
}

func (m *InMemoryCampaignStore) ListEbayFlaggedPurchases(_ context.Context) ([]inventory.Purchase, error) {
	var result []inventory.Purchase
	for _, p := range m.Purchases {
		if p.EbayExportFlaggedAt == nil || m.PurchaseSales[p.ID] || p.Grader != "PSA" {
			continue
		}
		c, ok := m.Campaigns[p.CampaignID]
		if !ok || c.Phase == inventory.PhaseClosed {
			continue
		}
		result = append(result, *p)
	}
	return result, nil
}

// --- PurchaseRepository: Snapshot Status Methods ---

func (m *InMemoryCampaignStore) ListSnapshotPurchasesByStatus(_ context.Context, status inventory.SnapshotStatus, limit int) ([]inventory.Purchase, error) {
	if limit <= 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(m.Purchases))
	for k := range m.Purchases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result []inventory.Purchase
	for _, k := range keys {
		p := m.Purchases[k]
		if p.SnapshotStatus == status {
			result = append(result, *p)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseSnapshotStatus(_ context.Context, id string, status inventory.SnapshotStatus, retryCount int) error {
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.SnapshotStatus = status
	p.SnapshotRetryCount = retryCount
	return nil
}
