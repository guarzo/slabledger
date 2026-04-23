package inventory

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// mockRepo is a simple in-memory repository for testing unexported functions
// in import_test.go and tuning_test.go. Service-layer tests should use the
// shared mock from testutil/mocks.NewInMemoryCampaignStore() instead.
type mockRepo struct {
	campaigns       map[string]*Campaign
	purchases       map[string]*Purchase
	sales           map[string]*Sale
	invoices        map[string]*Invoice // keyed by ID
	certNumbers     map[string]bool
	purchaseSales   map[string]bool // purchaseID -> has sale
	pnlData         map[string]*CampaignPNL
	channelVelocity []ChannelVelocity
	dhPushConfig    *DHPushConfig
	dhHoldReasons   map[string]string // purchaseID -> hold reason
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
		dhHoldReasons: make(map[string]string),
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

func (m *mockRepo) GetSalesByPurchaseIDs(_ context.Context, purchaseIDs []string) (map[string]*Sale, error) {
	result := make(map[string]*Sale, len(purchaseIDs))
	for _, pid := range purchaseIDs {
		for _, s := range m.sales {
			if s.PurchaseID == pid {
				result[pid] = s
				break
			}
		}
	}
	return result, nil
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

func (m *mockRepo) GetPurchasesByDHInventoryIDs(_ context.Context, dhIDs []int) (map[int]*Purchase, error) {
	result := make(map[int]*Purchase, len(dhIDs))
	for _, id := range dhIDs {
		for _, p := range m.purchases {
			if p.DHInventoryID == id {
				result[id] = p
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

func (m *mockRepo) UpdatePurchaseCLSyncedAt(_ context.Context, id string, syncedAt string) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.CLSyncedAt = syncedAt
	return nil
}

func (m *mockRepo) UpdatePurchaseMMValue(_ context.Context, id string, mmValueCents int) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.MMValueCents = mmValueCents
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

func (m *mockRepo) UpdatePurchaseImages(_ context.Context, id string, frontURL, backURL string) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.FrontImageURL = frontURL
	p.BackImageURL = backURL
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
	if m.purchaseSales[purchaseID] {
		return ErrPurchaseHasSale
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
	p.PSAShipDate = fields.PSAShipDate
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

func (m *mockRepo) GetPendingReceiptByInvoiceDate(_ context.Context, invoiceDates []string) (map[string]int, error) {
	result := make(map[string]int)
	dateSet := make(map[string]bool)
	for _, d := range invoiceDates {
		dateSet[d] = true
	}
	for _, p := range m.purchases {
		if dateSet[p.InvoiceDate] && !p.WasRefunded && p.ReceivedAt == nil {
			result[p.InvoiceDate] += p.BuyCostCents
		}
	}
	return result, nil
}

func (m *mockRepo) GetCashflowConfig(_ context.Context) (*CashflowConfig, error) {
	return &CashflowConfig{CapitalBudgetCents: 5000000, CashBufferCents: 1000000}, nil
}
func (m *mockRepo) UpdateCashflowConfig(_ context.Context, _ *CashflowConfig) error {
	return nil
}
func (m *mockRepo) GetCapitalRawData(_ context.Context) (*CapitalRawData, error) {
	return &CapitalRawData{OutstandingCents: 0, RecoveryRate30dCents: 0, RecoveryRate30dPriorCents: 0}, nil
}

func (m *mockRepo) GetInvoiceSellThrough(_ context.Context, invoiceDate string) (InvoiceSellThrough, error) {
	var result InvoiceSellThrough
	for _, p := range m.purchases {
		if p.InvoiceDate != invoiceDate || p.WasRefunded {
			continue
		}
		if p.ReceivedAt == nil {
			continue
		}
		result.TotalPurchaseCount++
		result.TotalCostCents += p.BuyCostCents
		if m.purchaseSales[p.ID] {
			result.SoldCount++
			// Sale revenue: look it up from Sales map by purchase ID
			for _, s := range m.sales {
				if s.PurchaseID == p.ID {
					result.SaleRevenueCents += s.SalePriceCents
					break
				}
			}
		}
	}
	return result, nil
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

func (m *mockRepo) SetReceivedAt(_ context.Context, purchaseID string, receivedAt time.Time) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	receivedAtStr := receivedAt.Format("2006-01-02T15:04:05Z07:00")
	p.ReceivedAt = &receivedAtStr
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

func (m *mockRepo) OpenFlagPurchaseIDs(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (m *mockRepo) UpdatePurchaseDHFields(_ context.Context, _ string, _ DHFieldsUpdate) error {
	return nil
}

func (m *mockRepo) GetPurchasesByDHCertStatus(_ context.Context, _ string, _ int) ([]Purchase, error) {
	return nil, nil
}

func (m *mockRepo) IncrementDHPushAttempts(_ context.Context, id string) (int, error) {
	p, ok := m.purchases[id]
	if !ok {
		return 0, ErrPurchaseNotFound
	}
	p.DHPushAttempts++
	return p.DHPushAttempts, nil
}

func (m *mockRepo) UpdatePurchaseDHPushStatus(_ context.Context, id string, status string) error {
	if p, ok := m.purchases[id]; ok && p != nil {
		p.DHPushStatus = status
		// Mirror production semantics: transitions into pending (re-enrollment)
		// or matched (success) reset the retry counter so the next retry budget
		// starts fresh. Unmatched preserves the count as diagnostic signal.
		if status == DHPushStatusPending || status == DHPushStatusMatched {
			p.DHPushAttempts = 0
		}
	}
	return nil
}

func (m *mockRepo) UpdatePurchaseDHStatus(_ context.Context, id string, status string) error {
	if p, ok := m.purchases[id]; ok && p != nil {
		p.DHStatus = DHStatus(status)
	}
	return nil
}

func (m *mockRepo) UpdatePurchaseDHCardID(_ context.Context, id string, cardID int) error {
	if p, ok := m.purchases[id]; ok && p != nil {
		p.DHCardID = cardID
	}
	return nil
}

func (m *mockRepo) GetPurchasesByDHPushStatus(_ context.Context, _ string, _ int) ([]Purchase, error) {
	return nil, nil
}

func (m *mockRepo) CountUnsoldByDHPushStatus(_ context.Context) (map[string]int, error) {
	return map[string]int{}, nil
}

func (m *mockRepo) CountDHPipelineHealth(_ context.Context) (DHPipelineHealth, error) {
	return DHPipelineHealth{}, nil
}

func (m *mockRepo) UpdatePurchaseDHCandidates(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockRepo) UpdatePurchaseDHHoldReason(_ context.Context, id string, reason string) error {
	if m.purchases == nil {
		return ErrPurchaseNotFound
	}
	if _, ok := m.purchases[id]; !ok {
		return ErrPurchaseNotFound
	}
	m.dhHoldReasons[id] = reason
	return nil
}

func (m *mockRepo) SetHeldWithReason(_ context.Context, purchaseID string, reason string) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.DHPushStatus = DHPushStatusHeld
	m.dhHoldReasons[purchaseID] = reason
	return nil
}

func (m *mockRepo) ApproveHeldPurchase(_ context.Context, purchaseID string) error {
	if _, ok := m.purchases[purchaseID]; !ok {
		return ErrPurchaseNotFound
	}
	p := m.purchases[purchaseID]
	p.DHPushStatus = DHPushStatusPending
	p.DHPushAttempts = 0
	m.dhHoldReasons[purchaseID] = ""
	return nil
}

func (m *mockRepo) ResetDHFieldsForRepush(_ context.Context, purchaseID string) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.DHInventoryID = 0
	p.DHPushStatus = DHPushStatusPending
	p.DHPushAttempts = 0
	p.DHStatus = ""
	p.DHListingPriceCents = 0
	p.DHChannelsJSON = "[]"
	return nil
}

func (m *mockRepo) ResetDHFieldsForRepushDueToDelete(_ context.Context, _ string) error {
	return nil
}

func (m *mockRepo) UpdatePurchaseDHPriceSync(_ context.Context, _ string, _ int, _ time.Time) error {
	return nil
}

func (m *mockRepo) ListDHPriceDrift(_ context.Context) ([]Purchase, error) {
	return nil, nil
}

func (m *mockRepo) GetDHPushConfig(_ context.Context) (*DHPushConfig, error) {
	if m.dhPushConfig != nil {
		return m.dhPushConfig, nil
	}
	def := DefaultDHPushConfig()
	return &def, nil
}

func (m *mockRepo) SaveDHPushConfig(_ context.Context, cfg *DHPushConfig) error {
	if cfg == nil {
		return fmt.Errorf("dh push config cannot be nil")
	}
	m.dhPushConfig = cfg
	return nil
}

// --- SellSheetRepository stubs ---

func (m *mockRepo) GetSellSheetItems(_ context.Context) ([]string, error) {
	return []string{}, nil
}
func (m *mockRepo) AddSellSheetItems(_ context.Context, _ []string) error {
	return nil
}
func (m *mockRepo) RemoveSellSheetItems(_ context.Context, _ []string) error {
	return nil
}
func (m *mockRepo) ClearSellSheet(_ context.Context) error {
	return nil
}
