package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- PurchaseRepository: field updates and attribution ---

func (m *InMemoryCampaignStore) UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents, population int, confidence *int) error {
	if m.UpdatePurchaseCLValueFn != nil {
		return m.UpdatePurchaseCLValueFn(ctx, id, clValueCents, population, confidence)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.CLValueCents = clValueCents
	p.Population = population
	if confidence != nil {
		c := *confidence
		p.CLCardConfidenceAtPurchase = &c
	}
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseCLSyncedAt(ctx context.Context, id string, syncedAt string) error {
	if m.UpdatePurchaseCLSyncedAtFn != nil {
		return m.UpdatePurchaseCLSyncedAtFn(ctx, id, syncedAt)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.CLSyncedAt = syncedAt
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseCardMetadata(ctx context.Context, id, cardName, cardNumber, setName string) error {
	if m.UpdatePurchaseCardMetadataFn != nil {
		return m.UpdatePurchaseCardMetadataFn(ctx, id, cardName, cardNumber, setName)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.CardName = cardName
	p.CardNumber = cardNumber
	p.SetName = setName
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseImages(ctx context.Context, id, frontURL, backURL string) error {
	if m.UpdatePurchaseImagesFn != nil {
		return m.UpdatePurchaseImagesFn(ctx, id, frontURL, backURL)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.FrontImageURL = frontURL
	p.BackImageURL = backURL
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error {
	if m.UpdatePurchaseGradeFn != nil {
		return m.UpdatePurchaseGradeFn(ctx, id, gradeValue)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.GradeValue = gradeValue
	return nil
}

func (m *InMemoryCampaignStore) UpdateExternalPurchaseFields(ctx context.Context, id string, p *inventory.Purchase) error {
	if m.UpdateExternalPurchaseFieldsFn != nil {
		return m.UpdateExternalPurchaseFieldsFn(ctx, id, p)
	}
	existing, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	existing.CardName = p.CardName
	existing.CardNumber = p.CardNumber
	existing.SetName = p.SetName
	existing.Grader = p.Grader
	existing.GradeValue = p.GradeValue
	existing.BuyCostCents = p.BuyCostCents
	existing.CLValueCents = p.CLValueCents
	existing.FrontImageURL = p.FrontImageURL
	existing.BackImageURL = p.BackImageURL
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap inventory.MarketSnapshotData) error {
	if m.UpdatePurchaseMarketSnapshotFn != nil {
		return m.UpdatePurchaseMarketSnapshotFn(ctx, id, snap)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.MarketSnapshotData = snap
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseCampaign(ctx context.Context, purchaseID, campaignID string, sourcingFeeCents int) error {
	if m.UpdatePurchaseCampaignFn != nil {
		return m.UpdatePurchaseCampaignFn(ctx, purchaseID, campaignID, sourcingFeeCents)
	}
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	if m.PurchaseSales[purchaseID] {
		return inventory.ErrPurchaseHasSale
	}
	p.CampaignID = campaignID
	p.PSASourcingFeeCents = sourcingFeeCents
	p.AttributionSource = inventory.AttributionSourceManual
	return nil
}

// ReattributePurchase moves a purchase to a PSA-authoritative campaign and marks
// attribution_source='psa', refusing when a linked sale exists (mirrors the
// Postgres store's conditional-update guard).
func (m *InMemoryCampaignStore) ReattributePurchase(ctx context.Context, purchaseID string, r inventory.Reattribution) error {
	if m.ReattributePurchaseFn != nil {
		return m.ReattributePurchaseFn(ctx, purchaseID, r)
	}
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	if m.PurchaseSales[purchaseID] {
		return inventory.ErrPurchaseHasSale
	}
	p.CampaignID = r.CampaignID
	p.PSASourcingFeeCents = r.PSASourcingFeeCents
	p.CLConfidenceAtPurchase = r.CLConfidenceAtPurchase
	p.PSACampaignName = r.PSACampaignName
	p.AttributionSource = inventory.AttributionSourcePSA
	return nil
}

// UpdatePurchaseAttributionName records PSA's campaign name and attribution
// source without moving the campaign. Safe on sold purchases.
func (m *InMemoryCampaignStore) UpdatePurchaseAttributionName(ctx context.Context, purchaseID, psaName, source string) error {
	if m.UpdatePurchaseAttributionNameFn != nil {
		return m.UpdatePurchaseAttributionNameFn(ctx, purchaseID, psaName, source)
	}
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.PSACampaignName = psaName
	p.AttributionSource = source
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchasePSAFields(ctx context.Context, id string, fields inventory.PSAUpdateFields) error {
	if m.UpdatePurchasePSAFieldsFn != nil {
		return m.UpdatePurchasePSAFieldsFn(ctx, id, fields)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.PSAShipDate = fields.PSAShipDate
	p.InvoiceDate = fields.InvoiceDate
	p.WasRefunded = fields.WasRefunded
	p.FrontImageURL = fields.FrontImageURL
	p.BackImageURL = fields.BackImageURL
	p.PurchaseSource = fields.PurchaseSource
	p.PSAListingTitle = fields.PSAListingTitle
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseBuyCost(_ context.Context, id string, buyCostCents int) error {
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.BuyCostCents = buyCostCents
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseCardYear(_ context.Context, id string, year string) error {
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.CardYear = year
	return nil
}
