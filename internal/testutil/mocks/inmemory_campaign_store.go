package mocks

import (
	"context"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// InMemoryCampaignStore is an in-memory store implementing all inventory repository interfaces.
// It uses the Fn-field pattern: set individual Fn fields to override default behaviour.
// The default implementations provide a minimal working in-memory store with
// cascade deletes and duplicate cert detection, suitable for service-layer tests.
//
// Pass the same *InMemoryCampaignStore for all 7 repository slots in inventory.NewService.
type InMemoryCampaignStore struct {
	Campaigns       map[string]*inventory.Campaign
	Purchases       map[string]*inventory.Purchase
	Sales           map[string]*inventory.Sale
	Invoices        map[string]*inventory.Invoice
	CertNumbers     map[string]bool
	PurchaseSales   map[string]bool // purchaseID -> has sale
	PNLData         map[string]*inventory.CampaignPNL
	ChannelVelocity []inventory.ChannelVelocity
	CashflowConfig  *inventory.CashflowConfig

	// Optional overrides (Fn-field pattern)
	CreateCampaignFn               func(ctx context.Context, c *inventory.Campaign) error
	GetCampaignFn                  func(ctx context.Context, id string) (*inventory.Campaign, error)
	ListCampaignsFn                func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
	UpdateCampaignFn               func(ctx context.Context, c *inventory.Campaign) error
	DeleteCampaignFn               func(ctx context.Context, id string) error
	CreatePurchaseFn               func(ctx context.Context, p *inventory.Purchase) error
	GetPurchaseFn                  func(ctx context.Context, id string) (*inventory.Purchase, error)
	DeletePurchaseFn               func(ctx context.Context, id string) error
	ListPurchasesByCampaignFn      func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error)
	ListUnsoldPurchasesFn          func(ctx context.Context, campaignID string) ([]inventory.Purchase, error)
	ListAllUnsoldPurchasesFn       func(ctx context.Context) ([]inventory.Purchase, error)
	CountPurchasesByCampaignFn     func(ctx context.Context, campaignID string) (int, error)
	CreateSaleFn                   func(ctx context.Context, s *inventory.Sale) error
	GetSaleByPurchaseIDFn          func(ctx context.Context, purchaseID string) (*inventory.Sale, error)
	ListSalesByCampaignFn          func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error)
	DeleteSaleFn                   func(ctx context.Context, saleID string) error
	DeleteSaleByPurchaseIDFn       func(ctx context.Context, purchaseID string) error
	GetCampaignPNLFn               func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error)
	GetPNLByChannelFn              func(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error)
	GetDailySpendFn                func(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error)
	GetDaysToSellDistributionFn    func(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error)
	GetPerformanceByGradeFn        func(ctx context.Context, campaignID string) ([]inventory.GradePerformance, error)
	GetPurchasesWithSalesFn        func(ctx context.Context, campaignID string) ([]inventory.PurchaseWithSale, error)
	GetPurchaseByCertNumberFn      func(ctx context.Context, grader, certNumber string) (*inventory.Purchase, error)
	UpdatePurchaseCLValueFn        func(ctx context.Context, id string, clValueCents int, population int) error
	UpdatePurchaseCLSyncedAtFn     func(ctx context.Context, id string, syncedAt string) error
	UpdatePurchaseMMValueFn        func(ctx context.Context, id string, mmValueCents int) error
	UpdatePurchaseCardMetadataFn   func(ctx context.Context, id string, cardName, cardNumber, setName string) error
	UpdatePurchaseImagesFn         func(ctx context.Context, id string, frontURL, backURL string) error
	UpdatePurchaseGradeFn          func(ctx context.Context, id string, gradeValue float64) error
	UpdateExternalPurchaseFieldsFn func(ctx context.Context, id string, p *inventory.Purchase) error
	UpdatePurchaseMarketSnapshotFn func(ctx context.Context, id string, snap inventory.MarketSnapshotData) error
	UpdatePurchaseCampaignFn       func(ctx context.Context, purchaseID, campaignID string, sourcingFeeCents int) error
	UpdatePurchasePSAFieldsFn      func(ctx context.Context, id string, fields inventory.PSAUpdateFields) error
	GetAllPurchasesWithSalesFn     func(ctx context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error)
	GetGlobalPNLByChannelFn        func(ctx context.Context) ([]inventory.ChannelPNL, error)
	GetPurchasesByCertNumbersFn    func(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
	UpdatePurchaseDHFieldsFn       func(ctx context.Context, id string, update inventory.DHFieldsUpdate) error
	GetPurchasesByDHCertStatusFn   func(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
	UpdatePurchaseDHPushStatusFn   func(ctx context.Context, id string, status string) error
	UpdatePurchaseDHStatusFn       func(ctx context.Context, id string, status string) error
	UpdatePurchaseDHCardIDFn       func(ctx context.Context, id string, cardID int) error
	UpdatePurchaseDHCandidatesFn   func(ctx context.Context, id string, candidatesJSON string) error
	UpdatePurchaseDHHoldReasonFn   func(ctx context.Context, id string, reason string) error
	SetHeldWithReasonFn            func(ctx context.Context, purchaseID string, reason string) error
	ApproveHeldPurchaseFn          func(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRepushFn       func(ctx context.Context, purchaseID string) error
	UpdatePurchaseDHPriceSyncFn    func(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error
	ListDHPriceDriftFn             func(ctx context.Context) ([]inventory.Purchase, error)
	GetDHPushConfigFn              func(ctx context.Context) (*inventory.DHPushConfig, error)
	SaveDHPushConfigFn             func(ctx context.Context, cfg *inventory.DHPushConfig) error
	GetPurchasesByDHPushStatusFn   func(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
	CountDHPipelineHealthFn        func(ctx context.Context) (inventory.DHPipelineHealth, error)
	GetSellSheetItemsFn            func(ctx context.Context) ([]string, error)
	AddSellSheetItemsFn            func(ctx context.Context, purchaseIDs []string) error
	RemoveSellSheetItemsFn         func(ctx context.Context, purchaseIDs []string) error
	ClearSellSheetFn               func(ctx context.Context) error
	OpenFlagPurchaseIDsFn          func(ctx context.Context) (map[string]int64, error)
	GetCapitalRawDataFn            func(ctx context.Context) (*inventory.CapitalRawData, error)
	GetInvoiceSellThroughFn        func(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error)
	UpdateCashflowConfigFn         func(ctx context.Context, cfg *inventory.CashflowConfig) error
}

// Compile-time interface checks.
var _ inventory.CampaignRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.PurchaseRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.SaleRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.AnalyticsRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.FinanceRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.PricingRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.DHRepository = (*InMemoryCampaignStore)(nil)

// NewInMemoryCampaignStore creates a ready-to-use InMemoryCampaignStore with initialized maps.
func NewInMemoryCampaignStore() *InMemoryCampaignStore {
	return &InMemoryCampaignStore{
		Campaigns:     make(map[string]*inventory.Campaign),
		Purchases:     make(map[string]*inventory.Purchase),
		Sales:         make(map[string]*inventory.Sale),
		Invoices:      make(map[string]*inventory.Invoice),
		CertNumbers:   make(map[string]bool),
		PurchaseSales: make(map[string]bool),
		PNLData:       make(map[string]*inventory.CampaignPNL),
	}
}

// --- CampaignRepository ---

func (m *InMemoryCampaignStore) CreateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.CreateCampaignFn != nil {
		return m.CreateCampaignFn(ctx, c)
	}
	m.Campaigns[c.ID] = c
	return nil
}

func (m *InMemoryCampaignStore) GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error) {
	if m.GetCampaignFn != nil {
		return m.GetCampaignFn(ctx, id)
	}
	c, ok := m.Campaigns[id]
	if !ok {
		return nil, inventory.ErrCampaignNotFound
	}
	return c, nil
}

func (m *InMemoryCampaignStore) ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
	if m.ListCampaignsFn != nil {
		return m.ListCampaignsFn(ctx, activeOnly)
	}
	var result []inventory.Campaign
	for _, c := range m.Campaigns {
		if activeOnly && c.Phase != inventory.PhaseActive {
			continue
		}
		result = append(result, *c)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) DeleteCampaign(ctx context.Context, id string) error {
	if m.DeleteCampaignFn != nil {
		return m.DeleteCampaignFn(ctx, id)
	}
	if _, ok := m.Campaigns[id]; !ok {
		return inventory.ErrCampaignNotFound
	}
	for pid, p := range m.Purchases {
		if p.CampaignID == id {
			delete(m.CertNumbers, p.CertNumber)
			delete(m.PurchaseSales, pid)
			for sid, s := range m.Sales {
				if s.PurchaseID == pid {
					delete(m.Sales, sid)
				}
			}
			delete(m.Purchases, pid)
		}
	}
	delete(m.Campaigns, id)
	return nil
}

func (m *InMemoryCampaignStore) UpdateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.UpdateCampaignFn != nil {
		return m.UpdateCampaignFn(ctx, c)
	}
	if _, ok := m.Campaigns[c.ID]; !ok {
		return inventory.ErrCampaignNotFound
	}
	m.Campaigns[c.ID] = c
	return nil
}

// --- PurchaseRepository ---

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

func (m *InMemoryCampaignStore) GetPurchasesByIDs(_ context.Context, ids []string) (map[string]*inventory.Purchase, error) {
	result := make(map[string]*inventory.Purchase, len(ids))
	for _, id := range ids {
		if p, ok := m.Purchases[id]; ok {
			result[id] = p
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents int, population int) error {
	if m.UpdatePurchaseCLValueFn != nil {
		return m.UpdatePurchaseCLValueFn(ctx, id, clValueCents, population)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.CLValueCents = clValueCents
	p.Population = population
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

func (m *InMemoryCampaignStore) UpdatePurchaseMMValue(ctx context.Context, id string, mmValueCents int) error {
	if m.UpdatePurchaseMMValueFn != nil {
		return m.UpdatePurchaseMMValueFn(ctx, id, mmValueCents)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.MMValueCents = mmValueCents
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

func (m *InMemoryCampaignStore) UpdatePurchasePriceOverride(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = inventory.OverrideSource(source)
	if priceCents > 0 {
		p.OverrideSetAt = time.Now().Format(time.RFC3339)
	} else {
		p.OverrideSetAt = ""
	}
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = priceCents
	p.AISuggestedAt = time.Now().Format(time.RFC3339)
	return nil
}

func (m *InMemoryCampaignStore) ClearPurchaseAISuggestion(_ context.Context, purchaseID string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *InMemoryCampaignStore) AcceptAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	if p.AISuggestedPriceCents != priceCents {
		return inventory.ErrNoAISuggestion
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = inventory.OverrideSourceAIAccepted
	p.OverrideSetAt = time.Now().Format(time.RFC3339)
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *InMemoryCampaignStore) GetPriceOverrideStats(_ context.Context) (*inventory.PriceOverrideStats, error) {
	var stats inventory.PriceOverrideStats
	for id, p := range m.Purchases {
		if m.PurchaseSales[id] {
			continue // sold
		}
		stats.TotalUnsold++
		if p.OverridePriceCents > 0 {
			stats.OverrideCount++
			stats.OverrideTotalCents += p.OverridePriceCents
			switch p.OverrideSource {
			case inventory.OverrideSourceManual:
				stats.ManualCount++
			case inventory.OverrideSourceCostMarkup:
				stats.CostMarkupCount++
			case inventory.OverrideSourceAIAccepted:
				stats.AIAcceptedCount++
			}
		}
		if p.AISuggestedPriceCents > 0 {
			stats.PendingSuggestions++
			stats.SuggestionTotalCents += p.AISuggestedPriceCents
		}
	}
	return &stats, nil
}

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

func (m *InMemoryCampaignStore) UpdatePurchaseCardYear(_ context.Context, id string, year string) error {
	p, ok := m.Purchases[id]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.CardYear = year
	return nil
}

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

// --- AnalyticsRepository ---

func (m *InMemoryCampaignStore) GetCampaignPNL(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	if pnl, ok := m.PNLData[campaignID]; ok {
		return pnl, nil
	}
	return &inventory.CampaignPNL{CampaignID: campaignID}, nil
}

func (m *InMemoryCampaignStore) GetPNLByChannel(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetDailySpend(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error) {
	if m.GetDaysToSellDistributionFn != nil {
		return m.GetDaysToSellDistributionFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetPerformanceByGrade(ctx context.Context, campaignID string) ([]inventory.GradePerformance, error) {
	if m.GetPerformanceByGradeFn != nil {
		return m.GetPerformanceByGradeFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetPurchasesWithSales(ctx context.Context, campaignID string) ([]inventory.PurchaseWithSale, error) {
	if m.GetPurchasesWithSalesFn != nil {
		return m.GetPurchasesWithSalesFn(ctx, campaignID)
	}
	ids := make([]string, 0, len(m.Purchases))
	for id, p := range m.Purchases {
		if p.CampaignID == campaignID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var result []inventory.PurchaseWithSale
	for _, id := range ids {
		p := m.Purchases[id]
		pws := inventory.PurchaseWithSale{Purchase: *p}
		for _, s := range m.Sales {
			if s.PurchaseID == p.ID {
				pws.Sale = s
				break
			}
		}
		result = append(result, pws)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetAllPurchasesWithSales(ctx context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
	if m.GetAllPurchasesWithSalesFn != nil {
		return m.GetAllPurchasesWithSalesFn(ctx, opts...)
	}
	var f inventory.PurchaseFilter
	for _, o := range opts {
		o(&f)
	}

	// Collect purchase IDs in sorted order for deterministic output.
	ids := make([]string, 0, len(m.Purchases))
	for id := range m.Purchases {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var result []inventory.PurchaseWithSale
	for _, id := range ids {
		p := m.Purchases[id]
		if f.SinceDate != "" && p.PurchaseDate < f.SinceDate {
			continue
		}
		if f.ExcludeArchived {
			if c, ok := m.Campaigns[p.CampaignID]; ok && c.Phase == inventory.PhaseClosed {
				continue
			}
		}
		pws := inventory.PurchaseWithSale{Purchase: *p}
		for _, s := range m.Sales {
			if s.PurchaseID == p.ID {
				pws.Sale = s
				break
			}
		}
		result = append(result, pws)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetPortfolioChannelVelocity(_ context.Context) ([]inventory.ChannelVelocity, error) {
	if m.ChannelVelocity != nil {
		return m.ChannelVelocity, nil
	}
	return []inventory.ChannelVelocity{}, nil
}

func (m *InMemoryCampaignStore) GetGlobalPNLByChannel(ctx context.Context) ([]inventory.ChannelPNL, error) {
	if m.GetGlobalPNLByChannelFn != nil {
		return m.GetGlobalPNLByChannelFn(ctx)
	}
	return []inventory.ChannelPNL{}, nil
}

func (m *InMemoryCampaignStore) GetDailyCapitalTimeSeries(_ context.Context) ([]inventory.DailyCapitalPoint, error) {
	return []inventory.DailyCapitalPoint{}, nil
}

// --- FinanceRepository ---

func (m *InMemoryCampaignStore) CreateInvoice(_ context.Context, inv *inventory.Invoice) error {
	m.Invoices[inv.ID] = inv
	return nil
}

func (m *InMemoryCampaignStore) GetInvoice(_ context.Context, id string) (*inventory.Invoice, error) {
	if inv, ok := m.Invoices[id]; ok {
		return inv, nil
	}
	return nil, inventory.ErrInvoiceNotFound
}

func (m *InMemoryCampaignStore) ListInvoices(_ context.Context) ([]inventory.Invoice, error) {
	result := make([]inventory.Invoice, 0, len(m.Invoices))
	for _, inv := range m.Invoices {
		result = append(result, *inv)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) UpdateInvoice(_ context.Context, inv *inventory.Invoice) error {
	existing, ok := m.Invoices[inv.ID]
	if !ok {
		return inventory.ErrInvoiceNotFound
	}
	*existing = *inv
	return nil
}

func (m *InMemoryCampaignStore) SumPurchaseCostByInvoiceDate(_ context.Context, invoiceDate string) (int, error) {
	total := 0
	for _, p := range m.Purchases {
		if p.InvoiceDate == invoiceDate && !p.WasRefunded {
			total += p.BuyCostCents + p.PSASourcingFeeCents
		}
	}
	return total, nil
}

func (m *InMemoryCampaignStore) GetPendingReceiptByInvoiceDate(_ context.Context, invoiceDates []string) (map[string]int, error) {
	result := make(map[string]int)
	dateSet := make(map[string]bool)
	for _, d := range invoiceDates {
		dateSet[d] = true
	}
	for _, p := range m.Purchases {
		if dateSet[p.InvoiceDate] && !p.WasRefunded && p.ReceivedAt == nil {
			result[p.InvoiceDate] += p.BuyCostCents
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetCashflowConfig(_ context.Context) (*inventory.CashflowConfig, error) {
	if m.CashflowConfig != nil {
		cfg := *m.CashflowConfig
		return &cfg, nil
	}
	return &inventory.CashflowConfig{CapitalBudgetCents: 5000000, CashBufferCents: 1000000}, nil
}

func (m *InMemoryCampaignStore) UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error {
	if m.UpdateCashflowConfigFn != nil {
		return m.UpdateCashflowConfigFn(ctx, cfg)
	}
	if cfg == nil {
		return nil
	}
	cp := *cfg
	m.CashflowConfig = &cp
	return nil
}

func (m *InMemoryCampaignStore) GetCapitalRawData(ctx context.Context) (*inventory.CapitalRawData, error) {
	if m.GetCapitalRawDataFn != nil {
		return m.GetCapitalRawDataFn(ctx)
	}
	return &inventory.CapitalRawData{OutstandingCents: 0, RecoveryRate30dCents: 0, RecoveryRate30dPriorCents: 0}, nil
}

func (m *InMemoryCampaignStore) GetInvoiceSellThrough(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error) {
	if m.GetInvoiceSellThroughFn != nil {
		return m.GetInvoiceSellThroughFn(ctx, invoiceDate)
	}
	var result inventory.InvoiceSellThrough
	for _, p := range m.Purchases {
		if p.InvoiceDate != invoiceDate || p.WasRefunded {
			continue
		}
		if p.ReceivedAt == nil {
			continue
		}
		result.TotalPurchaseCount++
		result.TotalCostCents += p.BuyCostCents
		if m.PurchaseSales[p.ID] {
			result.SoldCount++
			for _, s := range m.Sales {
				if s.PurchaseID == p.ID {
					result.SaleRevenueCents += s.SalePriceCents
					break
				}
			}
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) CreateRevocationFlag(_ context.Context, _ *inventory.RevocationFlag) error {
	return nil
}

func (m *InMemoryCampaignStore) ListRevocationFlags(_ context.Context) ([]inventory.RevocationFlag, error) {
	return []inventory.RevocationFlag{}, nil
}

func (m *InMemoryCampaignStore) GetLatestRevocationFlag(_ context.Context) (*inventory.RevocationFlag, error) {
	return nil, nil
}

func (m *InMemoryCampaignStore) UpdateRevocationFlagStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}

// --- PricingRepository ---

func (m *InMemoryCampaignStore) UpdateReviewedPrice(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.ReviewedPriceCents = priceCents
	if priceCents > 0 {
		p.ReviewedAt = time.Now().Format(time.RFC3339)
		p.ReviewSource = inventory.ReviewSource(source)
	} else {
		p.ReviewedAt = ""
		p.ReviewSource = ""
	}
	return nil
}

func (m *InMemoryCampaignStore) GetReviewStats(_ context.Context, _ string) (inventory.ReviewStats, error) {
	return inventory.ReviewStats{}, nil
}

func (m *InMemoryCampaignStore) GetGlobalReviewStats(_ context.Context) (inventory.ReviewStats, error) {
	return inventory.ReviewStats{}, nil
}

func (m *InMemoryCampaignStore) CreatePriceFlag(_ context.Context, _ *inventory.PriceFlag) (int64, error) {
	return 0, nil
}

func (m *InMemoryCampaignStore) ListPriceFlags(_ context.Context, _ string) ([]inventory.PriceFlagWithContext, error) {
	return []inventory.PriceFlagWithContext{}, nil
}

func (m *InMemoryCampaignStore) ResolvePriceFlag(_ context.Context, _ int64, _ int64) error {
	return nil
}

func (m *InMemoryCampaignStore) HasOpenFlag(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *InMemoryCampaignStore) OpenFlagPurchaseIDs(ctx context.Context) (map[string]int64, error) {
	if m.OpenFlagPurchaseIDsFn != nil {
		return m.OpenFlagPurchaseIDsFn(ctx)
	}
	return map[string]int64{}, nil
}

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

func (m *InMemoryCampaignStore) UpdatePurchaseDHStatus(ctx context.Context, id string, status string) error {
	if m.UpdatePurchaseDHStatusFn != nil {
		return m.UpdatePurchaseDHStatusFn(ctx, id, status)
	}
	return nil
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
	p.UpdatedAt = time.Now()
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseDHPriceSync(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error {
	if m.UpdatePurchaseDHPriceSyncFn != nil {
		return m.UpdatePurchaseDHPriceSyncFn(ctx, id, listingPriceCents, syncedAt)
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
