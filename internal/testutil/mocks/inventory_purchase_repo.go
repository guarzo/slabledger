package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// PurchaseRepositoryMock implements inventory.PurchaseRepository with Fn-field pattern.
type PurchaseRepositoryMock struct {
	CreatePurchaseFn                     func(ctx context.Context, p *inventory.Purchase) error
	GetPurchaseFn                        func(ctx context.Context, id string) (*inventory.Purchase, error)
	DeletePurchaseFn                     func(ctx context.Context, id string) error
	ListPurchasesByCampaignFn            func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error)
	ListUnsoldPurchasesFn                func(ctx context.Context, campaignID string) ([]inventory.Purchase, error)
	ListAllUnsoldPurchasesFn             func(ctx context.Context) ([]inventory.Purchase, error)
	CountPurchasesByCampaignFn           func(ctx context.Context, campaignID string) (int, error)
	GetPurchaseByCertNumberFn            func(ctx context.Context, grader, certNumber string) (*inventory.Purchase, error)
	GetPurchasesByGraderAndCertNumbersFn func(ctx context.Context, grader string, certNumbers []string) (map[string]*inventory.Purchase, error)
	GetPurchasesByIDsFn                  func(ctx context.Context, ids []string) (map[string]*inventory.Purchase, error)
	GetPurchasesByCertNumbersFn          func(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
	UpdatePurchaseCLValueFn              func(ctx context.Context, id string, clValueCents int, population int) error
	UpdatePurchaseCLSyncedAtFn           func(ctx context.Context, id string, syncedAt string) error
	UpdatePurchaseMMValueFn              func(ctx context.Context, id string, mmValueCents int) error
	UpdatePurchaseCardMetadataFn         func(ctx context.Context, id, cardName, cardNumber, setName string) error
	UpdatePurchaseImagesFn               func(ctx context.Context, id, frontURL, backURL string) error
	UpdatePurchaseGradeFn                func(ctx context.Context, id string, gradeValue float64) error
	UpdateExternalPurchaseFieldsFn       func(ctx context.Context, id string, p *inventory.Purchase) error
	UpdatePurchaseMarketSnapshotFn       func(ctx context.Context, id string, snap inventory.MarketSnapshotData) error
	UpdatePurchaseCampaignFn             func(ctx context.Context, purchaseID, campaignID string, sourcingFeeCents int) error
	UpdatePurchasePSAFieldsFn            func(ctx context.Context, id string, fields inventory.PSAUpdateFields) error
	UpdatePurchaseBuyCostFn              func(ctx context.Context, id string, buyCostCents int) error
	UpdatePurchasePriceOverrideFn        func(ctx context.Context, purchaseID string, priceCents int, source string) error
	UpdatePurchaseAISuggestionFn         func(ctx context.Context, purchaseID string, priceCents int) error
	ClearPurchaseAISuggestionFn          func(ctx context.Context, purchaseID string) error
	AcceptAISuggestionFn                 func(ctx context.Context, purchaseID string, priceCents int) error
	GetPriceOverrideStatsFn              func(ctx context.Context) (*inventory.PriceOverrideStats, error)
	SetReceivedAtFn                      func(ctx context.Context, purchaseID string, receivedAt time.Time) error
	SetEbayExportFlagFn                  func(ctx context.Context, purchaseID string, flaggedAt time.Time) error
	ClearEbayExportFlagsFn               func(ctx context.Context, purchaseIDs []string) error
	ListEbayFlaggedPurchasesFn           func(ctx context.Context) ([]inventory.Purchase, error)
	UpdatePurchaseCardYearFn             func(ctx context.Context, id, year string) error
	ListSnapshotPurchasesByStatusFn      func(ctx context.Context, status inventory.SnapshotStatus, limit int) ([]inventory.Purchase, error)
	UpdatePurchaseSnapshotStatusFn       func(ctx context.Context, id string, status inventory.SnapshotStatus, retryCount int) error
	UpdatePurchaseDHFieldsFn             func(ctx context.Context, id string, update inventory.DHFieldsUpdate) error
	GetPurchasesByDHCertStatusFn         func(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
	UpdatePurchaseDHPushStatusFn         func(ctx context.Context, id string, status string) error
	IncrementDHPushAttemptsFn            func(ctx context.Context, id string) (int, error)
	UpdatePurchaseDHStatusFn             func(ctx context.Context, id string, status string) error
	UpdatePurchaseDHCardIDFn             func(ctx context.Context, id string, cardID int) error
	GetPurchasesByDHPushStatusFn         func(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
	CountUnsoldByDHPushStatusFn          func(ctx context.Context) (map[string]int, error)
	CountDHPipelineHealthFn              func(ctx context.Context) (inventory.DHPipelineHealth, error)
	UpdatePurchaseDHCandidatesFn         func(ctx context.Context, id, candidatesJSON string) error
	UpdatePurchaseDHHoldReasonFn         func(ctx context.Context, id, reason string) error
	SetHeldWithReasonFn                  func(ctx context.Context, purchaseID, reason string) error
	ApproveHeldPurchaseFn                func(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRepushFn             func(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRepushDueToDeleteFn  func(ctx context.Context, purchaseID string) error
	UpdatePurchaseDHPriceSyncFn          func(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error
	UnmatchPurchaseDHFn                  func(ctx context.Context, purchaseID string, pushStatus string) error
	ListDHPriceDriftFn                   func(ctx context.Context) ([]inventory.Purchase, error)
}

var _ inventory.PurchaseRepository = (*PurchaseRepositoryMock)(nil)

func (m *PurchaseRepositoryMock) CreatePurchase(ctx context.Context, p *inventory.Purchase) error {
	if m.CreatePurchaseFn != nil {
		return m.CreatePurchaseFn(ctx, p)
	}
	return nil
}

func (m *PurchaseRepositoryMock) GetPurchase(ctx context.Context, id string) (*inventory.Purchase, error) {
	if m.GetPurchaseFn != nil {
		return m.GetPurchaseFn(ctx, id)
	}
	return nil, inventory.ErrPurchaseNotFound
}

func (m *PurchaseRepositoryMock) DeletePurchase(ctx context.Context, id string) error {
	if m.DeletePurchaseFn != nil {
		return m.DeletePurchaseFn(ctx, id)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error) {
	if m.ListPurchasesByCampaignFn != nil {
		return m.ListPurchasesByCampaignFn(ctx, campaignID, limit, offset)
	}
	return []inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) ListUnsoldPurchases(ctx context.Context, campaignID string) ([]inventory.Purchase, error) {
	if m.ListUnsoldPurchasesFn != nil {
		return m.ListUnsoldPurchasesFn(ctx, campaignID)
	}
	return []inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	if m.ListAllUnsoldPurchasesFn != nil {
		return m.ListAllUnsoldPurchasesFn(ctx)
	}
	return []inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) CountPurchasesByCampaign(ctx context.Context, campaignID string) (int, error) {
	if m.CountPurchasesByCampaignFn != nil {
		return m.CountPurchasesByCampaignFn(ctx, campaignID)
	}
	return 0, nil
}

func (m *PurchaseRepositoryMock) GetPurchaseByCertNumber(ctx context.Context, grader string, certNumber string) (*inventory.Purchase, error) {
	if m.GetPurchaseByCertNumberFn != nil {
		return m.GetPurchaseByCertNumberFn(ctx, grader, certNumber)
	}
	return nil, inventory.ErrPurchaseNotFound
}

func (m *PurchaseRepositoryMock) GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if m.GetPurchasesByGraderAndCertNumbersFn != nil {
		return m.GetPurchasesByGraderAndCertNumbersFn(ctx, grader, certNumbers)
	}
	return map[string]*inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if m.GetPurchasesByCertNumbersFn != nil {
		return m.GetPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	return map[string]*inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) GetPurchasesByIDs(ctx context.Context, ids []string) (map[string]*inventory.Purchase, error) {
	if m.GetPurchasesByIDsFn != nil {
		return m.GetPurchasesByIDsFn(ctx, ids)
	}
	return map[string]*inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents int, population int) error {
	if m.UpdatePurchaseCLValueFn != nil {
		return m.UpdatePurchaseCLValueFn(ctx, id, clValueCents, population)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseCLSyncedAt(ctx context.Context, id string, syncedAt string) error {
	if m.UpdatePurchaseCLSyncedAtFn != nil {
		return m.UpdatePurchaseCLSyncedAtFn(ctx, id, syncedAt)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseMMValue(ctx context.Context, id string, mmValueCents int) error {
	if m.UpdatePurchaseMMValueFn != nil {
		return m.UpdatePurchaseMMValueFn(ctx, id, mmValueCents)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseCardMetadata(ctx context.Context, id string, cardName, cardNumber, setName string) error {
	if m.UpdatePurchaseCardMetadataFn != nil {
		return m.UpdatePurchaseCardMetadataFn(ctx, id, cardName, cardNumber, setName)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseImages(ctx context.Context, id string, frontURL, backURL string) error {
	if m.UpdatePurchaseImagesFn != nil {
		return m.UpdatePurchaseImagesFn(ctx, id, frontURL, backURL)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error {
	if m.UpdatePurchaseGradeFn != nil {
		return m.UpdatePurchaseGradeFn(ctx, id, gradeValue)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdateExternalPurchaseFields(ctx context.Context, id string, p *inventory.Purchase) error {
	if m.UpdateExternalPurchaseFieldsFn != nil {
		return m.UpdateExternalPurchaseFieldsFn(ctx, id, p)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap inventory.MarketSnapshotData) error {
	if m.UpdatePurchaseMarketSnapshotFn != nil {
		return m.UpdatePurchaseMarketSnapshotFn(ctx, id, snap)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseCampaign(ctx context.Context, purchaseID string, campaignID string, sourcingFeeCents int) error {
	if m.UpdatePurchaseCampaignFn != nil {
		return m.UpdatePurchaseCampaignFn(ctx, purchaseID, campaignID, sourcingFeeCents)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchasePSAFields(ctx context.Context, id string, fields inventory.PSAUpdateFields) error {
	if m.UpdatePurchasePSAFieldsFn != nil {
		return m.UpdatePurchasePSAFieldsFn(ctx, id, fields)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseBuyCost(ctx context.Context, id string, buyCostCents int) error {
	if m.UpdatePurchaseBuyCostFn != nil {
		return m.UpdatePurchaseBuyCostFn(ctx, id, buyCostCents)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchasePriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.UpdatePurchasePriceOverrideFn != nil {
		return m.UpdatePurchasePriceOverrideFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseAISuggestion(ctx context.Context, purchaseID string, priceCents int) error {
	if m.UpdatePurchaseAISuggestionFn != nil {
		return m.UpdatePurchaseAISuggestionFn(ctx, purchaseID, priceCents)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ClearPurchaseAISuggestion(ctx context.Context, purchaseID string) error {
	if m.ClearPurchaseAISuggestionFn != nil {
		return m.ClearPurchaseAISuggestionFn(ctx, purchaseID)
	}
	return nil
}

func (m *PurchaseRepositoryMock) AcceptAISuggestion(ctx context.Context, purchaseID string, priceCents int) error {
	if m.AcceptAISuggestionFn != nil {
		return m.AcceptAISuggestionFn(ctx, purchaseID, priceCents)
	}
	return nil
}

func (m *PurchaseRepositoryMock) GetPriceOverrideStats(ctx context.Context) (*inventory.PriceOverrideStats, error) {
	if m.GetPriceOverrideStatsFn != nil {
		return m.GetPriceOverrideStatsFn(ctx)
	}
	return &inventory.PriceOverrideStats{}, nil
}

func (m *PurchaseRepositoryMock) SetReceivedAt(ctx context.Context, purchaseID string, receivedAt time.Time) error {
	if m.SetReceivedAtFn != nil {
		return m.SetReceivedAtFn(ctx, purchaseID, receivedAt)
	}
	return nil
}

func (m *PurchaseRepositoryMock) SetEbayExportFlag(ctx context.Context, purchaseID string, flaggedAt time.Time) error {
	if m.SetEbayExportFlagFn != nil {
		return m.SetEbayExportFlagFn(ctx, purchaseID, flaggedAt)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error {
	if m.ClearEbayExportFlagsFn != nil {
		return m.ClearEbayExportFlagsFn(ctx, purchaseIDs)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ListEbayFlaggedPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	if m.ListEbayFlaggedPurchasesFn != nil {
		return m.ListEbayFlaggedPurchasesFn(ctx)
	}
	return []inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseCardYear(ctx context.Context, id string, year string) error {
	if m.UpdatePurchaseCardYearFn != nil {
		return m.UpdatePurchaseCardYearFn(ctx, id, year)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ListSnapshotPurchasesByStatus(ctx context.Context, status inventory.SnapshotStatus, limit int) ([]inventory.Purchase, error) {
	if m.ListSnapshotPurchasesByStatusFn != nil {
		return m.ListSnapshotPurchasesByStatusFn(ctx, status, limit)
	}
	return []inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseSnapshotStatus(ctx context.Context, id string, status inventory.SnapshotStatus, retryCount int) error {
	if m.UpdatePurchaseSnapshotStatusFn != nil {
		return m.UpdatePurchaseSnapshotStatusFn(ctx, id, status, retryCount)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error {
	if m.UpdatePurchaseDHFieldsFn != nil {
		return m.UpdatePurchaseDHFieldsFn(ctx, id, update)
	}
	return nil
}

func (m *PurchaseRepositoryMock) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	if m.GetPurchasesByDHCertStatusFn != nil {
		return m.GetPurchasesByDHCertStatusFn(ctx, status, limit)
	}
	return []inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
	if m.UpdatePurchaseDHPushStatusFn != nil {
		return m.UpdatePurchaseDHPushStatusFn(ctx, id, status)
	}
	return nil
}

func (m *PurchaseRepositoryMock) IncrementDHPushAttempts(ctx context.Context, id string) (int, error) {
	if m.IncrementDHPushAttemptsFn != nil {
		return m.IncrementDHPushAttemptsFn(ctx, id)
	}
	return 0, nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseDHStatus(ctx context.Context, id string, status string) error {
	if m.UpdatePurchaseDHStatusFn != nil {
		return m.UpdatePurchaseDHStatusFn(ctx, id, status)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseDHCardID(ctx context.Context, id string, cardID int) error {
	if m.UpdatePurchaseDHCardIDFn != nil {
		return m.UpdatePurchaseDHCardIDFn(ctx, id, cardID)
	}
	return nil
}

func (m *PurchaseRepositoryMock) GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	if m.GetPurchasesByDHPushStatusFn != nil {
		return m.GetPurchasesByDHPushStatusFn(ctx, status, limit)
	}
	return []inventory.Purchase{}, nil
}

func (m *PurchaseRepositoryMock) CountUnsoldByDHPushStatus(ctx context.Context) (map[string]int, error) {
	if m.CountUnsoldByDHPushStatusFn != nil {
		return m.CountUnsoldByDHPushStatusFn(ctx)
	}
	return map[string]int{}, nil
}

func (m *PurchaseRepositoryMock) CountDHPipelineHealth(ctx context.Context) (inventory.DHPipelineHealth, error) {
	if m.CountDHPipelineHealthFn != nil {
		return m.CountDHPipelineHealthFn(ctx)
	}
	return inventory.DHPipelineHealth{}, nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error {
	if m.UpdatePurchaseDHCandidatesFn != nil {
		return m.UpdatePurchaseDHCandidatesFn(ctx, id, candidatesJSON)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error {
	if m.UpdatePurchaseDHHoldReasonFn != nil {
		return m.UpdatePurchaseDHHoldReasonFn(ctx, id, reason)
	}
	return nil
}

func (m *PurchaseRepositoryMock) SetHeldWithReason(ctx context.Context, purchaseID string, reason string) error {
	if m.SetHeldWithReasonFn != nil {
		return m.SetHeldWithReasonFn(ctx, purchaseID, reason)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ApproveHeldPurchase(ctx context.Context, purchaseID string) error {
	if m.ApproveHeldPurchaseFn != nil {
		return m.ApproveHeldPurchaseFn(ctx, purchaseID)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ResetDHFieldsForRepush(ctx context.Context, purchaseID string) error {
	if m.ResetDHFieldsForRepushFn != nil {
		return m.ResetDHFieldsForRepushFn(ctx, purchaseID)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ResetDHFieldsForRepushDueToDelete(ctx context.Context, purchaseID string) error {
	if m.ResetDHFieldsForRepushDueToDeleteFn != nil {
		return m.ResetDHFieldsForRepushDueToDeleteFn(ctx, purchaseID)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UpdatePurchaseDHPriceSync(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error {
	if m.UpdatePurchaseDHPriceSyncFn != nil {
		return m.UpdatePurchaseDHPriceSyncFn(ctx, id, listingPriceCents, syncedAt)
	}
	return nil
}

func (m *PurchaseRepositoryMock) UnmatchPurchaseDH(ctx context.Context, purchaseID string, pushStatus string) error {
	if m.UnmatchPurchaseDHFn != nil {
		return m.UnmatchPurchaseDHFn(ctx, purchaseID, pushStatus)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ListDHPriceDrift(ctx context.Context) ([]inventory.Purchase, error) {
	if m.ListDHPriceDriftFn != nil {
		return m.ListDHPriceDriftFn(ctx)
	}
	return []inventory.Purchase{}, nil
}
