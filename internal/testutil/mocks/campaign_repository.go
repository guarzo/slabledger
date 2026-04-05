package mocks

import (
	"context"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// MockCampaignRepository is an in-memory mock implementing campaigns.Repository.
// It uses function-field pattern: set individual Fn fields to override default behaviour.
// The default implementations provide a minimal working in-memory store with
// cascade deletes and duplicate cert detection, suitable for service-layer tests.
type MockCampaignRepository struct {
	Campaigns       map[string]*campaigns.Campaign
	Purchases       map[string]*campaigns.Purchase
	Sales           map[string]*campaigns.Sale
	Invoices        map[string]*campaigns.Invoice
	CertNumbers     map[string]bool
	PurchaseSales   map[string]bool // purchaseID -> has sale
	PNLData         map[string]*campaigns.CampaignPNL
	ChannelVelocity []campaigns.ChannelVelocity

	// Optional overrides (Fn-field pattern)
	CreateCampaignFn               func(ctx context.Context, c *campaigns.Campaign) error
	GetCampaignFn                  func(ctx context.Context, id string) (*campaigns.Campaign, error)
	ListCampaignsFn                func(ctx context.Context, activeOnly bool) ([]campaigns.Campaign, error)
	UpdateCampaignFn               func(ctx context.Context, c *campaigns.Campaign) error
	DeleteCampaignFn               func(ctx context.Context, id string) error
	CreatePurchaseFn               func(ctx context.Context, p *campaigns.Purchase) error
	GetPurchaseFn                  func(ctx context.Context, id string) (*campaigns.Purchase, error)
	DeletePurchaseFn               func(ctx context.Context, id string) error
	ListPurchasesByCampaignFn      func(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Purchase, error)
	ListUnsoldPurchasesFn          func(ctx context.Context, campaignID string) ([]campaigns.Purchase, error)
	ListAllUnsoldPurchasesFn       func(ctx context.Context) ([]campaigns.Purchase, error)
	CountPurchasesByCampaignFn     func(ctx context.Context, campaignID string) (int, error)
	CreateSaleFn                   func(ctx context.Context, s *campaigns.Sale) error
	GetSaleByPurchaseIDFn          func(ctx context.Context, purchaseID string) (*campaigns.Sale, error)
	ListSalesByCampaignFn          func(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Sale, error)
	DeleteSaleFn                   func(ctx context.Context, saleID string) error
	DeleteSaleByPurchaseIDFn       func(ctx context.Context, purchaseID string) error
	GetCampaignPNLFn               func(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error)
	GetPNLByChannelFn              func(ctx context.Context, campaignID string) ([]campaigns.ChannelPNL, error)
	GetDailySpendFn                func(ctx context.Context, campaignID string, days int) ([]campaigns.DailySpend, error)
	GetDaysToSellDistributionFn    func(ctx context.Context, campaignID string) ([]campaigns.DaysToSellBucket, error)
	GetPerformanceByGradeFn        func(ctx context.Context, campaignID string) ([]campaigns.GradePerformance, error)
	GetPurchasesWithSalesFn        func(ctx context.Context, campaignID string) ([]campaigns.PurchaseWithSale, error)
	GetPurchaseByCertNumberFn      func(ctx context.Context, grader, certNumber string) (*campaigns.Purchase, error)
	UpdatePurchaseCLValueFn        func(ctx context.Context, id string, clValueCents int, population int) error
	UpdatePurchaseCardMetadataFn   func(ctx context.Context, id string, cardName, cardNumber, setName string) error
	UpdatePurchaseGradeFn          func(ctx context.Context, id string, gradeValue float64) error
	UpdateExternalPurchaseFieldsFn func(ctx context.Context, id string, p *campaigns.Purchase) error
	UpdatePurchaseMarketSnapshotFn func(ctx context.Context, id string, snap campaigns.MarketSnapshotData) error
	UpdatePurchaseCampaignFn       func(ctx context.Context, purchaseID, campaignID string, sourcingFeeCents int) error
	UpdatePurchasePSAFieldsFn      func(ctx context.Context, id string, fields campaigns.PSAUpdateFields) error
	GetAllPurchasesWithSalesFn     func(ctx context.Context, opts ...campaigns.PurchaseFilterOpt) ([]campaigns.PurchaseWithSale, error)
	GetGlobalPNLByChannelFn        func(ctx context.Context) ([]campaigns.ChannelPNL, error)
	GetPurchasesByCertNumbersFn    func(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error)
	UpdatePurchaseDHFieldsFn       func(ctx context.Context, id string, update campaigns.DHFieldsUpdate) error
	GetPurchasesByDHCertStatusFn   func(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error)
	GetSellSheetItemsFn            func(ctx context.Context, userID int64) ([]string, error)
	AddSellSheetItemsFn            func(ctx context.Context, userID int64, purchaseIDs []string) error
	RemoveSellSheetItemsFn         func(ctx context.Context, userID int64, purchaseIDs []string) error
	ClearSellSheetFn               func(ctx context.Context, userID int64) error
}

// NewMockCampaignRepository creates a ready-to-use MockCampaignRepository with initialized maps.
func NewMockCampaignRepository() *MockCampaignRepository {
	return &MockCampaignRepository{
		Campaigns:     make(map[string]*campaigns.Campaign),
		Purchases:     make(map[string]*campaigns.Purchase),
		Sales:         make(map[string]*campaigns.Sale),
		Invoices:      make(map[string]*campaigns.Invoice),
		CertNumbers:   make(map[string]bool),
		PurchaseSales: make(map[string]bool),
		PNLData:       make(map[string]*campaigns.CampaignPNL),
	}
}

func (m *MockCampaignRepository) CreateCampaign(ctx context.Context, c *campaigns.Campaign) error {
	if m.CreateCampaignFn != nil {
		return m.CreateCampaignFn(ctx, c)
	}
	m.Campaigns[c.ID] = c
	return nil
}

func (m *MockCampaignRepository) GetCampaign(ctx context.Context, id string) (*campaigns.Campaign, error) {
	if m.GetCampaignFn != nil {
		return m.GetCampaignFn(ctx, id)
	}
	c, ok := m.Campaigns[id]
	if !ok {
		return nil, campaigns.ErrCampaignNotFound
	}
	return c, nil
}

func (m *MockCampaignRepository) ListCampaigns(ctx context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
	if m.ListCampaignsFn != nil {
		return m.ListCampaignsFn(ctx, activeOnly)
	}
	var result []campaigns.Campaign
	for _, c := range m.Campaigns {
		if activeOnly && c.Phase != campaigns.PhaseActive {
			continue
		}
		result = append(result, *c)
	}
	return result, nil
}

func (m *MockCampaignRepository) DeleteCampaign(ctx context.Context, id string) error {
	if m.DeleteCampaignFn != nil {
		return m.DeleteCampaignFn(ctx, id)
	}
	if _, ok := m.Campaigns[id]; !ok {
		return campaigns.ErrCampaignNotFound
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

func (m *MockCampaignRepository) UpdateCampaign(ctx context.Context, c *campaigns.Campaign) error {
	if m.UpdateCampaignFn != nil {
		return m.UpdateCampaignFn(ctx, c)
	}
	if _, ok := m.Campaigns[c.ID]; !ok {
		return campaigns.ErrCampaignNotFound
	}
	m.Campaigns[c.ID] = c
	return nil
}

func (m *MockCampaignRepository) CreatePurchase(ctx context.Context, p *campaigns.Purchase) error {
	if m.CreatePurchaseFn != nil {
		return m.CreatePurchaseFn(ctx, p)
	}
	if p.Grader == "" {
		p.Grader = "PSA"
	}
	if m.CertNumbers[p.CertNumber] {
		return campaigns.ErrDuplicateCertNumber
	}
	m.Purchases[p.ID] = p
	m.CertNumbers[p.CertNumber] = true
	return nil
}

func (m *MockCampaignRepository) GetPurchase(ctx context.Context, id string) (*campaigns.Purchase, error) {
	if m.GetPurchaseFn != nil {
		return m.GetPurchaseFn(ctx, id)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return nil, campaigns.ErrPurchaseNotFound
	}
	return p, nil
}

func (m *MockCampaignRepository) DeletePurchase(ctx context.Context, id string) error {
	if m.DeletePurchaseFn != nil {
		return m.DeletePurchaseFn(ctx, id)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
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

func (m *MockCampaignRepository) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Purchase, error) {
	if m.ListPurchasesByCampaignFn != nil {
		return m.ListPurchasesByCampaignFn(ctx, campaignID, limit, offset)
	}
	var result []campaigns.Purchase
	for _, p := range m.Purchases {
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

func (m *MockCampaignRepository) ListUnsoldPurchases(ctx context.Context, campaignID string) ([]campaigns.Purchase, error) {
	if m.ListUnsoldPurchasesFn != nil {
		return m.ListUnsoldPurchasesFn(ctx, campaignID)
	}
	var result []campaigns.Purchase
	for _, p := range m.Purchases {
		if p.CampaignID == campaignID && !m.PurchaseSales[p.ID] {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *MockCampaignRepository) ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error) {
	if m.ListAllUnsoldPurchasesFn != nil {
		return m.ListAllUnsoldPurchasesFn(ctx)
	}
	var result []campaigns.Purchase
	for _, p := range m.Purchases {
		if !m.PurchaseSales[p.ID] {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *MockCampaignRepository) CountPurchasesByCampaign(ctx context.Context, campaignID string) (int, error) {
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

func (m *MockCampaignRepository) CreateSale(ctx context.Context, s *campaigns.Sale) error {
	if m.CreateSaleFn != nil {
		return m.CreateSaleFn(ctx, s)
	}
	if m.PurchaseSales[s.PurchaseID] {
		return campaigns.ErrDuplicateSale
	}
	m.Sales[s.ID] = s
	m.PurchaseSales[s.PurchaseID] = true
	return nil
}

func (m *MockCampaignRepository) GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*campaigns.Sale, error) {
	if m.GetSaleByPurchaseIDFn != nil {
		return m.GetSaleByPurchaseIDFn(ctx, purchaseID)
	}
	for _, s := range m.Sales {
		if s.PurchaseID == purchaseID {
			return s, nil
		}
	}
	return nil, campaigns.ErrSaleNotFound
}

func (m *MockCampaignRepository) GetSalesByPurchaseIDs(_ context.Context, purchaseIDs []string) (map[string]*campaigns.Sale, error) {
	result := make(map[string]*campaigns.Sale, len(purchaseIDs))
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

func (m *MockCampaignRepository) DeleteSale(ctx context.Context, saleID string) error {
	if m.DeleteSaleFn != nil {
		return m.DeleteSaleFn(ctx, saleID)
	}
	s, ok := m.Sales[saleID]
	if !ok {
		return campaigns.ErrSaleNotFound
	}
	delete(m.PurchaseSales, s.PurchaseID)
	delete(m.Sales, saleID)
	return nil
}

func (m *MockCampaignRepository) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
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
	return campaigns.ErrSaleNotFound
}

func (m *MockCampaignRepository) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Sale, error) {
	if m.ListSalesByCampaignFn != nil {
		return m.ListSalesByCampaignFn(ctx, campaignID, limit, offset)
	}
	var result []campaigns.Sale
	for _, s := range m.Sales {
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

func (m *MockCampaignRepository) GetCampaignPNL(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	if pnl, ok := m.PNLData[campaignID]; ok {
		return pnl, nil
	}
	return &campaigns.CampaignPNL{CampaignID: campaignID}, nil
}

func (m *MockCampaignRepository) GetPNLByChannel(ctx context.Context, campaignID string) ([]campaigns.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *MockCampaignRepository) GetDailySpend(ctx context.Context, campaignID string, days int) ([]campaigns.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return nil, nil
}

func (m *MockCampaignRepository) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]campaigns.DaysToSellBucket, error) {
	if m.GetDaysToSellDistributionFn != nil {
		return m.GetDaysToSellDistributionFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *MockCampaignRepository) GetPerformanceByGrade(ctx context.Context, campaignID string) ([]campaigns.GradePerformance, error) {
	if m.GetPerformanceByGradeFn != nil {
		return m.GetPerformanceByGradeFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *MockCampaignRepository) GetPurchasesWithSales(ctx context.Context, campaignID string) ([]campaigns.PurchaseWithSale, error) {
	if m.GetPurchasesWithSalesFn != nil {
		return m.GetPurchasesWithSalesFn(ctx, campaignID)
	}
	var result []campaigns.PurchaseWithSale
	for _, p := range m.Purchases {
		if p.CampaignID == campaignID {
			pws := campaigns.PurchaseWithSale{Purchase: *p}
			for _, s := range m.Sales {
				if s.PurchaseID == p.ID {
					pws.Sale = s
					break
				}
			}
			result = append(result, pws)
		}
	}
	return result, nil
}

func (m *MockCampaignRepository) GetPurchaseByCertNumber(ctx context.Context, grader, certNumber string) (*campaigns.Purchase, error) {
	if m.GetPurchaseByCertNumberFn != nil {
		return m.GetPurchaseByCertNumberFn(ctx, grader, certNumber)
	}
	for _, p := range m.Purchases {
		if p.Grader == grader && p.CertNumber == certNumber {
			return p, nil
		}
	}
	return nil, campaigns.ErrPurchaseNotFound
}

func (m *MockCampaignRepository) GetPurchasesByGraderAndCertNumbers(_ context.Context, grader string, certNumbers []string) (map[string]*campaigns.Purchase, error) {
	result := make(map[string]*campaigns.Purchase, len(certNumbers))
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

func (m *MockCampaignRepository) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error) {
	if m.GetPurchasesByCertNumbersFn != nil {
		return m.GetPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	result := make(map[string]*campaigns.Purchase, len(certNumbers))
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

func (m *MockCampaignRepository) GetPurchasesByIDs(_ context.Context, ids []string) (map[string]*campaigns.Purchase, error) {
	result := make(map[string]*campaigns.Purchase, len(ids))
	for _, id := range ids {
		if p, ok := m.Purchases[id]; ok {
			result[id] = p
		}
	}
	return result, nil
}

func (m *MockCampaignRepository) UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents int, population int) error {
	if m.UpdatePurchaseCLValueFn != nil {
		return m.UpdatePurchaseCLValueFn(ctx, id, clValueCents, population)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.CLValueCents = clValueCents
	p.Population = population
	return nil
}

func (m *MockCampaignRepository) UpdatePurchaseCardMetadata(ctx context.Context, id, cardName, cardNumber, setName string) error {
	if m.UpdatePurchaseCardMetadataFn != nil {
		return m.UpdatePurchaseCardMetadataFn(ctx, id, cardName, cardNumber, setName)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.CardName = cardName
	p.CardNumber = cardNumber
	p.SetName = setName
	return nil
}

func (m *MockCampaignRepository) UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error {
	if m.UpdatePurchaseGradeFn != nil {
		return m.UpdatePurchaseGradeFn(ctx, id, gradeValue)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.GradeValue = gradeValue
	return nil
}

func (m *MockCampaignRepository) UpdateExternalPurchaseFields(ctx context.Context, id string, p *campaigns.Purchase) error {
	if m.UpdateExternalPurchaseFieldsFn != nil {
		return m.UpdateExternalPurchaseFieldsFn(ctx, id, p)
	}
	existing, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
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

func (m *MockCampaignRepository) UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap campaigns.MarketSnapshotData) error {
	if m.UpdatePurchaseMarketSnapshotFn != nil {
		return m.UpdatePurchaseMarketSnapshotFn(ctx, id, snap)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.MarketSnapshotData = snap
	return nil
}

func (m *MockCampaignRepository) UpdatePurchaseCampaign(ctx context.Context, purchaseID, campaignID string, sourcingFeeCents int) error {
	if m.UpdatePurchaseCampaignFn != nil {
		return m.UpdatePurchaseCampaignFn(ctx, purchaseID, campaignID, sourcingFeeCents)
	}
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.CampaignID = campaignID
	p.PSASourcingFeeCents = sourcingFeeCents
	return nil
}

func (m *MockCampaignRepository) UpdatePurchasePSAFields(ctx context.Context, id string, fields campaigns.PSAUpdateFields) error {
	if m.UpdatePurchasePSAFieldsFn != nil {
		return m.UpdatePurchasePSAFieldsFn(ctx, id, fields)
	}
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.VaultStatus = fields.VaultStatus
	p.InvoiceDate = fields.InvoiceDate
	p.WasRefunded = fields.WasRefunded
	p.FrontImageURL = fields.FrontImageURL
	p.BackImageURL = fields.BackImageURL
	p.PurchaseSource = fields.PurchaseSource
	p.PSAListingTitle = fields.PSAListingTitle
	return nil
}

func (m *MockCampaignRepository) CreateInvoice(_ context.Context, inv *campaigns.Invoice) error {
	m.Invoices[inv.ID] = inv
	return nil
}
func (m *MockCampaignRepository) GetInvoice(_ context.Context, id string) (*campaigns.Invoice, error) {
	if inv, ok := m.Invoices[id]; ok {
		return inv, nil
	}
	return nil, campaigns.ErrInvoiceNotFound
}
func (m *MockCampaignRepository) ListInvoices(_ context.Context) ([]campaigns.Invoice, error) {
	result := make([]campaigns.Invoice, 0, len(m.Invoices))
	for _, inv := range m.Invoices {
		result = append(result, *inv)
	}
	return result, nil
}
func (m *MockCampaignRepository) UpdateInvoice(_ context.Context, inv *campaigns.Invoice) error {
	existing, ok := m.Invoices[inv.ID]
	if !ok {
		return campaigns.ErrInvoiceNotFound
	}
	*existing = *inv
	return nil
}
func (m *MockCampaignRepository) SumPurchaseCostByInvoiceDate(_ context.Context, invoiceDate string) (int, error) {
	total := 0
	for _, p := range m.Purchases {
		if p.InvoiceDate == invoiceDate && !p.WasRefunded {
			total += p.BuyCostCents + p.PSASourcingFeeCents
		}
	}
	return total, nil
}

func (m *MockCampaignRepository) GetCashflowConfig(_ context.Context) (*campaigns.CashflowConfig, error) {
	return &campaigns.CashflowConfig{CreditLimitCents: 5000000, CashBufferCents: 1000000}, nil
}
func (m *MockCampaignRepository) UpdateCashflowConfig(_ context.Context, _ *campaigns.CashflowConfig) error {
	return nil
}

func (m *MockCampaignRepository) GetCreditSummary(_ context.Context) (*campaigns.CreditSummary, error) {
	return &campaigns.CreditSummary{CreditLimitCents: 5000000}, nil
}

func (m *MockCampaignRepository) GetPortfolioChannelVelocity(_ context.Context) ([]campaigns.ChannelVelocity, error) {
	if m.ChannelVelocity != nil {
		return m.ChannelVelocity, nil
	}
	return []campaigns.ChannelVelocity{}, nil
}

func (m *MockCampaignRepository) GetAllPurchasesWithSales(ctx context.Context, opts ...campaigns.PurchaseFilterOpt) ([]campaigns.PurchaseWithSale, error) {
	if m.GetAllPurchasesWithSalesFn != nil {
		return m.GetAllPurchasesWithSalesFn(ctx, opts...)
	}
	return []campaigns.PurchaseWithSale{}, nil
}

func (m *MockCampaignRepository) GetGlobalPNLByChannel(ctx context.Context) ([]campaigns.ChannelPNL, error) {
	if m.GetGlobalPNLByChannelFn != nil {
		return m.GetGlobalPNLByChannelFn(ctx)
	}
	return []campaigns.ChannelPNL{}, nil
}

func (m *MockCampaignRepository) GetDailyCapitalTimeSeries(_ context.Context) ([]campaigns.DailyCapitalPoint, error) {
	return []campaigns.DailyCapitalPoint{}, nil
}

func (m *MockCampaignRepository) ListSnapshotPurchasesByStatus(_ context.Context, status campaigns.SnapshotStatus, limit int) ([]campaigns.Purchase, error) {
	if limit <= 0 {
		return nil, nil
	}
	// Collect keys and sort for deterministic ordering
	keys := make([]string, 0, len(m.Purchases))
	for k := range m.Purchases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result []campaigns.Purchase
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

func (m *MockCampaignRepository) UpdatePurchaseSnapshotStatus(_ context.Context, id string, status campaigns.SnapshotStatus, retryCount int) error {
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.SnapshotStatus = status
	p.SnapshotRetryCount = retryCount
	return nil
}

func (m *MockCampaignRepository) CreateRevocationFlag(_ context.Context, _ *campaigns.RevocationFlag) error {
	return nil
}
func (m *MockCampaignRepository) ListRevocationFlags(_ context.Context) ([]campaigns.RevocationFlag, error) {
	return []campaigns.RevocationFlag{}, nil
}
func (m *MockCampaignRepository) GetLatestRevocationFlag(_ context.Context) (*campaigns.RevocationFlag, error) {
	return nil, nil
}
func (m *MockCampaignRepository) UpdateRevocationFlagStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}

func (m *MockCampaignRepository) GetPriceOverrideStats(_ context.Context) (*campaigns.PriceOverrideStats, error) {
	var stats campaigns.PriceOverrideStats
	for id, p := range m.Purchases {
		if m.PurchaseSales[id] {
			continue // sold
		}
		stats.TotalUnsold++
		if p.OverridePriceCents > 0 {
			stats.OverrideCount++
			stats.OverrideTotalCents += p.OverridePriceCents
			switch p.OverrideSource {
			case campaigns.OverrideSourceManual:
				stats.ManualCount++
			case campaigns.OverrideSourceCostMarkup:
				stats.CostMarkupCount++
			case campaigns.OverrideSourceAIAccepted:
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

func (m *MockCampaignRepository) UpdatePurchaseBuyCost(_ context.Context, id string, buyCostCents int) error {
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.BuyCostCents = buyCostCents
	return nil
}

func (m *MockCampaignRepository) UpdatePurchasePriceOverride(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = campaigns.OverrideSource(source)
	if priceCents > 0 {
		p.OverrideSetAt = time.Now().Format(time.RFC3339)
	} else {
		p.OverrideSetAt = ""
	}
	return nil
}

func (m *MockCampaignRepository) UpdatePurchaseAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = priceCents
	p.AISuggestedAt = time.Now().Format(time.RFC3339)
	return nil
}

func (m *MockCampaignRepository) ClearPurchaseAISuggestion(_ context.Context, purchaseID string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *MockCampaignRepository) AcceptAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	if p.AISuggestedPriceCents != priceCents {
		return campaigns.ErrNoAISuggestion
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = campaigns.OverrideSourceAIAccepted
	p.OverrideSetAt = time.Now().Format(time.RFC3339)
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *MockCampaignRepository) SetEbayExportFlag(_ context.Context, purchaseID string, flaggedAt time.Time) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.EbayExportFlaggedAt = &flaggedAt
	return nil
}

func (m *MockCampaignRepository) ClearEbayExportFlags(_ context.Context, purchaseIDs []string) error {
	for _, id := range purchaseIDs {
		if p, ok := m.Purchases[id]; ok {
			p.EbayExportFlaggedAt = nil
		}
	}
	return nil
}

func (m *MockCampaignRepository) ListEbayFlaggedPurchases(_ context.Context) ([]campaigns.Purchase, error) {
	var result []campaigns.Purchase
	for _, p := range m.Purchases {
		if p.EbayExportFlaggedAt == nil || m.PurchaseSales[p.ID] || p.Grader != "PSA" {
			continue
		}
		// Exclude purchases with missing or closed campaigns (matches INNER JOIN in real SQL).
		c, ok := m.Campaigns[p.CampaignID]
		if !ok || c.Phase == campaigns.PhaseClosed {
			continue
		}
		result = append(result, *p)
	}
	return result, nil
}

func (m *MockCampaignRepository) UpdatePurchaseCardYear(_ context.Context, id string, year string) error {
	p, ok := m.Purchases[id]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.CardYear = year
	return nil
}

// --- PriceReviewRepository stubs ---

func (m *MockCampaignRepository) UpdateReviewedPrice(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return campaigns.ErrPurchaseNotFound
	}
	p.ReviewedPriceCents = priceCents
	if priceCents > 0 {
		p.ReviewedAt = time.Now().Format(time.RFC3339)
		p.ReviewSource = campaigns.ReviewSource(source)
	} else {
		p.ReviewedAt = ""
		p.ReviewSource = ""
	}
	return nil
}

func (m *MockCampaignRepository) GetReviewStats(_ context.Context, _ string) (campaigns.ReviewStats, error) {
	return campaigns.ReviewStats{}, nil
}

func (m *MockCampaignRepository) GetGlobalReviewStats(_ context.Context) (campaigns.ReviewStats, error) {
	return campaigns.ReviewStats{}, nil
}

func (m *MockCampaignRepository) CreatePriceFlag(_ context.Context, _ *campaigns.PriceFlag) (int64, error) {
	return 0, nil
}

func (m *MockCampaignRepository) ListPriceFlags(_ context.Context, _ string) ([]campaigns.PriceFlagWithContext, error) {
	return []campaigns.PriceFlagWithContext{}, nil
}

func (m *MockCampaignRepository) ResolvePriceFlag(_ context.Context, _ int64, _ int64) error {
	return nil
}

func (m *MockCampaignRepository) HasOpenFlag(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *MockCampaignRepository) OpenFlagPurchaseIDs(_ context.Context) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (m *MockCampaignRepository) UpdatePurchaseDHFields(ctx context.Context, id string, update campaigns.DHFieldsUpdate) error {
	if m.UpdatePurchaseDHFieldsFn != nil {
		return m.UpdatePurchaseDHFieldsFn(ctx, id, update)
	}
	return nil
}

func (m *MockCampaignRepository) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error) {
	if m.GetPurchasesByDHCertStatusFn != nil {
		return m.GetPurchasesByDHCertStatusFn(ctx, status, limit)
	}
	return nil, nil
}

func (m *MockCampaignRepository) GetSellSheetItems(ctx context.Context, userID int64) ([]string, error) {
	if m.GetSellSheetItemsFn != nil {
		return m.GetSellSheetItemsFn(ctx, userID)
	}
	return nil, nil
}

func (m *MockCampaignRepository) AddSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if m.AddSellSheetItemsFn != nil {
		return m.AddSellSheetItemsFn(ctx, userID, purchaseIDs)
	}
	return nil
}

func (m *MockCampaignRepository) RemoveSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if m.RemoveSellSheetItemsFn != nil {
		return m.RemoveSellSheetItemsFn(ctx, userID, purchaseIDs)
	}
	return nil
}

func (m *MockCampaignRepository) ClearSellSheet(ctx context.Context, userID int64) error {
	if m.ClearSellSheetFn != nil {
		return m.ClearSellSheetFn(ctx, userID)
	}
	return nil
}
