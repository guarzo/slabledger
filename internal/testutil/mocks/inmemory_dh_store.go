package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- DHRepository ---

func (m *InMemoryCampaignStore) UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error {
	if m.UpdatePurchaseDHFieldsFn != nil {
		return m.UpdatePurchaseDHFieldsFn(ctx, id, update)
	}
	return nil
}

func (m *InMemoryCampaignStore) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	if m.GetPurchasesByDHCertStatusFn != nil {
		return m.GetPurchasesByDHCertStatusFn(ctx, status, limit)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
	if m.UpdatePurchaseDHPushStatusFn != nil {
		return m.UpdatePurchaseDHPushStatusFn(ctx, id, status)
	}
	return nil
}

func (m *InMemoryCampaignStore) IncrementDHPushAttempts(ctx context.Context, id string) (int, error) {
	if m.IncrementDHPushAttemptsFn != nil {
		return m.IncrementDHPushAttemptsFn(ctx, id)
	}
	return 0, nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseDHStatus(ctx context.Context, id string, status string) error {
	if m.UpdatePurchaseDHStatusFn != nil {
		return m.UpdatePurchaseDHStatusFn(ctx, id, status)
	}
	return nil
}

func (m *InMemoryCampaignStore) ListStaleDHStatusSoldPurchases(ctx context.Context) ([]string, error) {
	if m.ListStaleDHStatusSoldPurchasesFn != nil {
		return m.ListStaleDHStatusSoldPurchasesFn(ctx)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseDHCardID(ctx context.Context, id string, cardID int) error {
	if m.UpdatePurchaseDHCardIDFn != nil {
		return m.UpdatePurchaseDHCardIDFn(ctx, id, cardID)
	}
	if p, ok := m.Purchases[id]; ok && p != nil {
		p.DHCardID = cardID
	}
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error {
	if m.UpdatePurchaseDHCandidatesFn != nil {
		return m.UpdatePurchaseDHCandidatesFn(ctx, id, candidatesJSON)
	}
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error {
	if m.UpdatePurchaseDHHoldReasonFn != nil {
		return m.UpdatePurchaseDHHoldReasonFn(ctx, id, reason)
	}
	return nil
}

func (m *InMemoryCampaignStore) SetHeldWithReason(ctx context.Context, purchaseID string, reason string) error {
	if m.SetHeldWithReasonFn != nil {
		return m.SetHeldWithReasonFn(ctx, purchaseID, reason)
	}
	return nil
}

func (m *InMemoryCampaignStore) ApproveHeldPurchase(ctx context.Context, purchaseID string) error {
	if m.ApproveHeldPurchaseFn != nil {
		return m.ApproveHeldPurchaseFn(ctx, purchaseID)
	}
	return nil
}

func (m *InMemoryCampaignStore) ResetDHFieldsForRepush(ctx context.Context, purchaseID string) error {
	if m.ResetDHFieldsForRepushFn != nil {
		return m.ResetDHFieldsForRepushFn(ctx, purchaseID)
	}
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.DHInventoryID = 0
	p.DHPushStatus = inventory.DHPushStatusPending
	p.DHStatus = ""
	p.DHListingPriceCents = 0
	p.DHChannelsJSON = "[]"
	p.DHHoldReason = ""
	p.UpdatedAt = time.Now()
	return nil
}

func (m *InMemoryCampaignStore) ResetDHFieldsForRepushDueToDelete(ctx context.Context, purchaseID string) error {
	if m.ResetDHFieldsForRepushDueToDeleteFn != nil {
		return m.ResetDHFieldsForRepushDueToDeleteFn(ctx, purchaseID)
	}
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.DHInventoryID = 0
	p.DHPushStatus = inventory.DHPushStatusPending
	p.DHStatus = ""
	p.DHListingPriceCents = 0
	p.DHChannelsJSON = "[]"
	p.DHHoldReason = ""
	now := time.Now()
	p.DHUnlistedDetectedAt = &now
	p.UpdatedAt = now
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseDHPriceSync(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error {
	if m.UpdatePurchaseDHPriceSyncFn != nil {
		return m.UpdatePurchaseDHPriceSyncFn(ctx, id, listingPriceCents, syncedAt)
	}
	return nil
}

func (m *InMemoryCampaignStore) UnmatchPurchaseDH(ctx context.Context, purchaseID string, pushStatus string) error {
	if m.UnmatchPurchaseDHFn != nil {
		return m.UnmatchPurchaseDHFn(ctx, purchaseID, pushStatus)
	}
	return nil
}

func (m *InMemoryCampaignStore) ListDHPriceDrift(ctx context.Context) ([]inventory.Purchase, error) {
	if m.ListDHPriceDriftFn != nil {
		return m.ListDHPriceDriftFn(ctx)
	}
	return []inventory.Purchase{}, nil
}

func (m *InMemoryCampaignStore) GetDHPushConfig(ctx context.Context) (*inventory.DHPushConfig, error) {
	if m.GetDHPushConfigFn != nil {
		return m.GetDHPushConfigFn(ctx)
	}
	def := inventory.DefaultDHPushConfig()
	return &def, nil
}

func (m *InMemoryCampaignStore) SaveDHPushConfig(ctx context.Context, cfg *inventory.DHPushConfig) error {
	if m.SaveDHPushConfigFn != nil {
		return m.SaveDHPushConfigFn(ctx, cfg)
	}
	return nil
}

func (m *InMemoryCampaignStore) GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	if m.GetPurchasesByDHPushStatusFn != nil {
		return m.GetPurchasesByDHPushStatusFn(ctx, status, limit)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) CountUnsoldByDHPushStatus(_ context.Context) (map[string]int, error) {
	return map[string]int{}, nil
}

func (m *InMemoryCampaignStore) CountDHPipelineHealth(ctx context.Context) (inventory.DHPipelineHealth, error) {
	if m.CountDHPipelineHealthFn != nil {
		return m.CountDHPipelineHealthFn(ctx)
	}
	return inventory.DHPipelineHealth{}, nil
}

func (m *InMemoryCampaignStore) GetSellSheetItems(ctx context.Context) ([]string, error) {
	if m.GetSellSheetItemsFn != nil {
		return m.GetSellSheetItemsFn(ctx)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) AddSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if m.AddSellSheetItemsFn != nil {
		return m.AddSellSheetItemsFn(ctx, purchaseIDs)
	}
	return nil
}

func (m *InMemoryCampaignStore) RemoveSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if m.RemoveSellSheetItemsFn != nil {
		return m.RemoveSellSheetItemsFn(ctx, purchaseIDs)
	}
	return nil
}

func (m *InMemoryCampaignStore) ClearSellSheet(ctx context.Context) error {
	if m.ClearSellSheetFn != nil {
		return m.ClearSellSheetFn(ctx)
	}
	return nil
}
