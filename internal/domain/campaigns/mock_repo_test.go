package campaigns

import (
	"context"
	"sort"
	"testing"
	"time"
)

// mockRepo is a simple in-memory repository for testing unexported functions
// in import_test.go and tuning_test.go. Service-layer tests should use the
// shared mock from testutil/mocks.MockCampaignRepository instead.
type mockRepo struct {
	campaigns       map[string]*Campaign
	purchases       map[string]*Purchase
	sales           map[string]*Sale
	invoices        map[string]*Invoice // keyed by ID
	certNumbers     map[string]bool
	purchaseSales   map[string]bool // purchaseID -> has sale
	pnlData         map[string]*CampaignPNL
	channelVelocity []ChannelVelocity
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		campaigns:     make(map[string]*Campaign),
		purchases:     make(map[string]*Purchase),
		sales:         make(map[string]*Sale),
		invoices:      make(map[string]*Invoice),
		certNumbers:   make(map[string]bool),
		purchaseSales: make(map[string]bool),
		pnlData:       make(map[string]*CampaignPNL),
	}
}

func (m *mockRepo) CreateCampaign(_ context.Context, c *Campaign) error {
	m.campaigns[c.ID] = c
	return nil
}

func (m *mockRepo) GetCampaign(_ context.Context, id string) (*Campaign, error) {
	c, ok := m.campaigns[id]
	if !ok {
		return nil, ErrCampaignNotFound
	}
	return c, nil
}

func (m *mockRepo) ListCampaigns(_ context.Context, activeOnly bool) ([]Campaign, error) {
	var result []Campaign
	for _, c := range m.campaigns {
		if activeOnly && c.Phase != PhaseActive {
			continue
		}
		result = append(result, *c)
	}
	return result, nil
}

func (m *mockRepo) DeleteCampaign(_ context.Context, id string) error {
	if _, ok := m.campaigns[id]; !ok {
		return ErrCampaignNotFound
	}
	for pid, p := range m.purchases {
		if p.CampaignID == id {
			delete(m.certNumbers, p.CertNumber)
			delete(m.purchaseSales, pid)
			for sid, s := range m.sales {
				if s.PurchaseID == pid {
					delete(m.sales, sid)
				}
			}
			delete(m.purchases, pid)
		}
	}
	delete(m.campaigns, id)
	return nil
}

func (m *mockRepo) UpdateCampaign(_ context.Context, c *Campaign) error {
	if _, ok := m.campaigns[c.ID]; !ok {
		return ErrCampaignNotFound
	}
	m.campaigns[c.ID] = c
	return nil
}

func (m *mockRepo) CreatePurchase(_ context.Context, p *Purchase) error {
	if p.Grader == "" {
		p.Grader = "PSA"
	}
	if m.certNumbers[p.CertNumber] {
		return ErrDuplicateCertNumber
	}
	m.purchases[p.ID] = p
	m.certNumbers[p.CertNumber] = true
	return nil
}

func (m *mockRepo) GetPurchase(_ context.Context, id string) (*Purchase, error) {
	p, ok := m.purchases[id]
	if !ok {
		return nil, ErrPurchaseNotFound
	}
	return p, nil
}

func (m *mockRepo) DeletePurchase(_ context.Context, id string) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	delete(m.certNumbers, p.CertNumber)
	delete(m.purchaseSales, id)
	for sid, s := range m.sales {
		if s.PurchaseID == id {
			delete(m.sales, sid)
		}
	}
	delete(m.purchases, id)
	return nil
}

func (m *mockRepo) ListPurchasesByCampaign(_ context.Context, campaignID string, limit, offset int) ([]Purchase, error) {
	var result []Purchase
	for _, p := range m.purchases {
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

func (m *mockRepo) ListUnsoldPurchases(_ context.Context, campaignID string) ([]Purchase, error) {
	var result []Purchase
	for _, p := range m.purchases {
		if p.CampaignID == campaignID && !m.purchaseSales[p.ID] {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *mockRepo) ListAllUnsoldPurchases(_ context.Context) ([]Purchase, error) {
	var result []Purchase
	for _, p := range m.purchases {
		if !m.purchaseSales[p.ID] {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *mockRepo) CountPurchasesByCampaign(_ context.Context, campaignID string) (int, error) {
	count := 0
	for _, p := range m.purchases {
		if p.CampaignID == campaignID {
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) CreateSale(_ context.Context, s *Sale) error {
	if m.purchaseSales[s.PurchaseID] {
		return ErrDuplicateSale
	}
	m.sales[s.ID] = s
	m.purchaseSales[s.PurchaseID] = true
	return nil
}

func (m *mockRepo) GetSaleByPurchaseID(_ context.Context, purchaseID string) (*Sale, error) {
	for _, s := range m.sales {
		if s.PurchaseID == purchaseID {
			return s, nil
		}
	}
	return nil, ErrSaleNotFound
}

func (m *mockRepo) DeleteSale(_ context.Context, saleID string) error {
	s, ok := m.sales[saleID]
	if !ok {
		return ErrSaleNotFound
	}
	delete(m.purchaseSales, s.PurchaseID)
	delete(m.sales, saleID)
	return nil
}

func (m *mockRepo) DeleteSaleByPurchaseID(_ context.Context, purchaseID string) error {
	for id, s := range m.sales {
		if s.PurchaseID == purchaseID {
			delete(m.purchaseSales, purchaseID)
			delete(m.sales, id)
			return nil
		}
	}
	return ErrSaleNotFound
}

func (m *mockRepo) ListSalesByCampaign(_ context.Context, campaignID string, limit, offset int) ([]Sale, error) {
	var result []Sale
	for _, s := range m.sales {
		if p, ok := m.purchases[s.PurchaseID]; ok && p.CampaignID == campaignID {
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

func (m *mockRepo) GetCampaignPNL(_ context.Context, campaignID string) (*CampaignPNL, error) {
	if pnl, ok := m.pnlData[campaignID]; ok {
		return pnl, nil
	}
	return &CampaignPNL{CampaignID: campaignID}, nil
}

func (m *mockRepo) GetPNLByChannel(_ context.Context, _ string) ([]ChannelPNL, error) {
	return nil, nil
}

func (m *mockRepo) GetDailySpend(_ context.Context, _ string, _ int) ([]DailySpend, error) {
	return nil, nil
}

func (m *mockRepo) GetDaysToSellDistribution(_ context.Context, _ string) ([]DaysToSellBucket, error) {
	return nil, nil
}

func (m *mockRepo) GetPerformanceByGrade(_ context.Context, _ string) ([]GradePerformance, error) {
	return nil, nil
}

func (m *mockRepo) GetPurchasesWithSales(_ context.Context, campaignID string) ([]PurchaseWithSale, error) {
	var result []PurchaseWithSale
	for _, p := range m.purchases {
		if p.CampaignID == campaignID {
			pws := PurchaseWithSale{Purchase: *p}
			for _, s := range m.sales {
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

func (m *mockRepo) GetPurchaseByCertNumber(_ context.Context, grader string, certNumber string) (*Purchase, error) {
	for _, p := range m.purchases {
		if p.Grader == grader && p.CertNumber == certNumber {
			return p, nil
		}
	}
	return nil, ErrPurchaseNotFound
}

func (m *mockRepo) GetPurchasesByGraderAndCertNumbers(_ context.Context, grader string, certNumbers []string) (map[string]*Purchase, error) {
	result := make(map[string]*Purchase, len(certNumbers))
	for _, cn := range certNumbers {
		for _, p := range m.purchases {
			if p.Grader == grader && p.CertNumber == cn {
				result[cn] = p
				break
			}
		}
	}
	return result, nil
}

func (m *mockRepo) GetPurchasesByCertNumbers(_ context.Context, certNumbers []string) (map[string]*Purchase, error) {
	result := make(map[string]*Purchase, len(certNumbers))
	for _, cn := range certNumbers {
		for _, p := range m.purchases {
			if p.CertNumber == cn {
				result[cn] = p
				break
			}
		}
	}
	return result, nil
}

func (m *mockRepo) GetPurchasesByIDs(_ context.Context, ids []string) (map[string]*Purchase, error) {
	result := make(map[string]*Purchase, len(ids))
	for _, id := range ids {
		if p, ok := m.purchases[id]; ok {
			result[id] = p
		}
	}
	return result, nil
}

func (m *mockRepo) UpdatePurchaseCLValue(_ context.Context, id string, clValueCents int, population int) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.CLValueCents = clValueCents
	p.Population = population
	return nil
}

func (m *mockRepo) UpdatePurchaseCardMetadata(_ context.Context, id string, cardName, cardNumber, setName string) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.CardName = cardName
	p.CardNumber = cardNumber
	p.SetName = setName
	return nil
}

func (m *mockRepo) UpdatePurchaseGrade(_ context.Context, id string, gradeValue float64) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.GradeValue = gradeValue
	return nil
}

func (m *mockRepo) UpdateExternalPurchaseFields(_ context.Context, id string, p *Purchase) error {
	existing, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
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

func (m *mockRepo) UpdatePurchaseMarketSnapshot(_ context.Context, id string, snap MarketSnapshotData) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.MarketSnapshotData = snap
	return nil
}

func (m *mockRepo) UpdatePurchaseCampaign(_ context.Context, purchaseID string, campaignID string, sourcingFeeCents int) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.CampaignID = campaignID
	p.PSASourcingFeeCents = sourcingFeeCents
	return nil
}

func (m *mockRepo) UpdatePurchasePSAFields(_ context.Context, id string, fields PSAUpdateFields) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
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

func (m *mockRepo) CreateInvoice(_ context.Context, inv *Invoice) error {
	m.invoices[inv.ID] = inv
	return nil
}
func (m *mockRepo) GetInvoice(_ context.Context, id string) (*Invoice, error) {
	if inv, ok := m.invoices[id]; ok {
		return inv, nil
	}
	return nil, ErrInvoiceNotFound
}
func (m *mockRepo) ListInvoices(_ context.Context) ([]Invoice, error) {
	result := make([]Invoice, 0, len(m.invoices))
	for _, inv := range m.invoices {
		result = append(result, *inv)
	}
	return result, nil
}
func (m *mockRepo) UpdateInvoice(_ context.Context, inv *Invoice) error {
	existing, ok := m.invoices[inv.ID]
	if !ok {
		return ErrInvoiceNotFound
	}
	*existing = *inv
	return nil
}
func (m *mockRepo) SumPurchaseCostByInvoiceDate(_ context.Context, invoiceDate string) (int, error) {
	total := 0
	for _, p := range m.purchases {
		if p.InvoiceDate == invoiceDate && !p.WasRefunded {
			total += p.BuyCostCents + p.PSASourcingFeeCents
		}
	}
	return total, nil
}

func (m *mockRepo) GetCashflowConfig(_ context.Context) (*CashflowConfig, error) {
	return &CashflowConfig{CreditLimitCents: 5000000, CashBufferCents: 1000000}, nil
}
func (m *mockRepo) UpdateCashflowConfig(_ context.Context, _ *CashflowConfig) error { return nil }

func (m *mockRepo) GetCreditSummary(_ context.Context) (*CreditSummary, error) {
	return &CreditSummary{CreditLimitCents: 5000000}, nil
}

func (m *mockRepo) GetPortfolioChannelVelocity(_ context.Context) ([]ChannelVelocity, error) {
	if m.channelVelocity != nil {
		return m.channelVelocity, nil
	}
	return []ChannelVelocity{}, nil
}

func (m *mockRepo) GetAllPurchasesWithSales(_ context.Context, _ ...PurchaseFilterOpt) ([]PurchaseWithSale, error) {
	return []PurchaseWithSale{}, nil
}

func (m *mockRepo) GetGlobalPNLByChannel(_ context.Context) ([]ChannelPNL, error) {
	return []ChannelPNL{}, nil
}

func (m *mockRepo) GetDailyCapitalTimeSeries(_ context.Context) ([]DailyCapitalPoint, error) {
	return []DailyCapitalPoint{}, nil
}

func (m *mockRepo) ListSnapshotPurchasesByStatus(_ context.Context, status SnapshotStatus, limit int) ([]Purchase, error) {
	if limit <= 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(m.purchases))
	for k := range m.purchases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result []Purchase
	for _, k := range keys {
		p := m.purchases[k]
		if p.SnapshotStatus == status {
			result = append(result, *p)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockRepo) UpdatePurchaseSnapshotStatus(_ context.Context, id string, status SnapshotStatus, retryCount int) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.SnapshotStatus = status
	p.SnapshotRetryCount = retryCount
	return nil
}

func (m *mockRepo) CreateRevocationFlag(_ context.Context, _ *RevocationFlag) error { return nil }
func (m *mockRepo) ListRevocationFlags(_ context.Context) ([]RevocationFlag, error) {
	return []RevocationFlag{}, nil
}
func (m *mockRepo) GetLatestRevocationFlag(_ context.Context) (*RevocationFlag, error) {
	return nil, nil
}
func (m *mockRepo) UpdateRevocationFlagStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}

func (m *mockRepo) GetPriceOverrideStats(_ context.Context) (*PriceOverrideStats, error) {
	return &PriceOverrideStats{}, nil
}

func (m *mockRepo) UpdatePurchaseBuyCost(_ context.Context, id string, buyCostCents int) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.BuyCostCents = buyCostCents
	return nil
}

func (m *mockRepo) UpdatePurchasePriceOverride(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = OverrideSource(source)
	if priceCents > 0 {
		p.OverrideSetAt = time.Now().Format(time.RFC3339)
	} else {
		p.OverrideSetAt = ""
	}
	return nil
}

func (m *mockRepo) UpdatePurchaseAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = priceCents
	p.AISuggestedAt = time.Now().Format(time.RFC3339)
	return nil
}

func (m *mockRepo) ClearPurchaseAISuggestion(_ context.Context, purchaseID string) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *mockRepo) AcceptAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	if p.AISuggestedPriceCents != priceCents {
		return ErrNoAISuggestion
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = OverrideSourceAIAccepted
	p.OverrideSetAt = time.Now().Format(time.RFC3339)
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *mockRepo) SetEbayExportFlag(_ context.Context, purchaseID string, flaggedAt time.Time) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.EbayExportFlaggedAt = &flaggedAt
	return nil
}

func (m *mockRepo) ClearEbayExportFlags(_ context.Context, purchaseIDs []string) error {
	for _, id := range purchaseIDs {
		if p, ok := m.purchases[id]; ok {
			p.EbayExportFlaggedAt = nil
		}
	}
	return nil
}

func (m *mockRepo) ListEbayFlaggedPurchases(_ context.Context) ([]Purchase, error) {
	var result []Purchase
	for _, p := range m.purchases {
		if p.EbayExportFlaggedAt == nil || m.purchaseSales[p.ID] || p.Grader != "PSA" {
			continue
		}
		c, ok := m.campaigns[p.CampaignID]
		if !ok || c.Phase == PhaseClosed {
			continue
		}
		result = append(result, *p)
	}
	return result, nil
}

func (m *mockRepo) UpdatePurchaseCardYear(_ context.Context, id string, year string) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.CardYear = year
	return nil
}

// --- PriceReviewRepository stubs ---

func (m *mockRepo) UpdateReviewedPrice(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.ReviewedPriceCents = priceCents
	if priceCents > 0 {
		p.ReviewedAt = time.Now().Format(time.RFC3339)
		p.ReviewSource = ReviewSource(source)
	} else {
		p.ReviewedAt = ""
		p.ReviewSource = ""
	}
	return nil
}

func (m *mockRepo) GetReviewStats(_ context.Context, _ string) (ReviewStats, error) {
	return ReviewStats{}, nil
}

func (m *mockRepo) GetGlobalReviewStats(_ context.Context) (ReviewStats, error) {
	return ReviewStats{}, nil
}

func (m *mockRepo) CreatePriceFlag(_ context.Context, _ *PriceFlag) (int64, error) {
	return 0, nil
}

func (m *mockRepo) ListPriceFlags(_ context.Context, _ string) ([]PriceFlagWithContext, error) {
	return []PriceFlagWithContext{}, nil
}

func (m *mockRepo) ResolvePriceFlag(_ context.Context, _ int64, _ int64) error {
	return nil
}

func (m *mockRepo) HasOpenFlag(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockRepo) OpenFlagPurchaseIDs(_ context.Context) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (m *mockRepo) UpdatePurchaseDHFields(_ context.Context, _ string, _ DHFieldsUpdate) error {
	return nil
}

func (m *mockRepo) GetPurchasesByDHCertStatus(_ context.Context, _ string, _ int) ([]Purchase, error) {
	return nil, nil
}

// mockPriceLookup is a test double for PriceLookup used by in-package tests
// (import_test.go, tuning_test.go) that access unexported symbols.
// Service-layer tests in campaigns_test use the version in service_test.go.
type mockPriceLookup struct {
	GetLastSoldCentsFn  func(ctx context.Context, card CardIdentity, grade float64) (int, error)
	GetMarketSnapshotFn func(ctx context.Context, card CardIdentity, grade float64) (*MarketSnapshot, error)
}

func (m *mockPriceLookup) GetLastSoldCents(ctx context.Context, card CardIdentity, grade float64) (int, error) {
	if m.GetLastSoldCentsFn != nil {
		return m.GetLastSoldCentsFn(ctx, card, grade)
	}
	return 0, nil
}

func (m *mockPriceLookup) GetMarketSnapshot(ctx context.Context, card CardIdentity, grade float64) (*MarketSnapshot, error) {
	if m.GetMarketSnapshotFn != nil {
		return m.GetMarketSnapshotFn(ctx, card, grade)
	}
	return nil, nil
}

// newDefaultPriceLookup returns a mockPriceLookup that returns fixed market data.
func newDefaultPriceLookup(t *testing.T, expectSetName string) *mockPriceLookup {
	return &mockPriceLookup{
		GetLastSoldCentsFn: func(_ context.Context, _ CardIdentity, _ float64) (int, error) {
			return 55000, nil
		},
		GetMarketSnapshotFn: func(_ context.Context, identity CardIdentity, _ float64) (*MarketSnapshot, error) {
			if t != nil && expectSetName != "" {
				if identity.SetName != expectSetName {
					t.Errorf("GetMarketSnapshot: SetName = %q, want %q", identity.SetName, expectSetName)
				}
			}
			return &MarketSnapshot{
				LastSoldCents:     55000,
				GradePriceCents:   60000,
				MedianCents:       57000,
				ConservativeCents: 50000,
				OptimisticCents:   65000,
				SalesLast30d:      12,
			}, nil
		},
	}
}
