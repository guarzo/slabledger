package arbitrage

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Inline test stubs (testutil/mocks imports arbitrage → cycle; define minimal stubs here).

type stubCampaignRepo struct {
	campaign *inventory.Campaign
}

func (r *stubCampaignRepo) GetCampaign(_ context.Context, _ string) (*inventory.Campaign, error) {
	return r.campaign, nil
}
func (r *stubCampaignRepo) CreateCampaign(_ context.Context, _ *inventory.Campaign) error { return nil }
func (r *stubCampaignRepo) ListCampaigns(_ context.Context, _ bool) ([]inventory.Campaign, error) {
	if r.campaign != nil {
		return []inventory.Campaign{*r.campaign}, nil
	}
	return nil, nil
}
func (r *stubCampaignRepo) UpdateCampaign(_ context.Context, _ *inventory.Campaign) error { return nil }
func (r *stubCampaignRepo) DeleteCampaign(_ context.Context, _ string) error              { return nil }

type stubPurchaseRepo struct {
	unsold []inventory.Purchase
}

func (r *stubPurchaseRepo) ListUnsoldPurchases(_ context.Context, _ string) ([]inventory.Purchase, error) {
	return r.unsold, nil
}
func (r *stubPurchaseRepo) CreatePurchase(_ context.Context, _ *inventory.Purchase) error { return nil }
func (r *stubPurchaseRepo) GetPurchase(_ context.Context, _ string) (*inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) DeletePurchase(_ context.Context, _ string) error { return nil }
func (r *stubPurchaseRepo) ListPurchasesByCampaign(_ context.Context, _ string, _, _ int) ([]inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) ListAllUnsoldPurchases(_ context.Context) ([]inventory.Purchase, error) {
	return r.unsold, nil
}
func (r *stubPurchaseRepo) CountPurchasesByCampaign(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (r *stubPurchaseRepo) GetPurchaseByCertNumber(_ context.Context, _, _ string) (*inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) GetPurchasesByGraderAndCertNumbers(_ context.Context, _ string, _ []string) (map[string]*inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) GetPurchasesByIDs(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) GetPurchasesByCertNumbers(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) GetPurchasesByDHInventoryIDs(_ context.Context, _ []int) (map[int]*inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseCLValue(_ context.Context, _ string, _, _ int) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseCLSyncedAt(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseMMValue(_ context.Context, _ string, _ int) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseCardMetadata(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseImages(_ context.Context, _, _, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseGrade(_ context.Context, _ string, _ float64) error {
	return nil
}
func (r *stubPurchaseRepo) UpdateExternalPurchaseFields(_ context.Context, _ string, _ *inventory.Purchase) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseMarketSnapshot(_ context.Context, _ string, _ inventory.MarketSnapshotData) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseCampaign(_ context.Context, _, _ string, _ int) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchasePSAFields(_ context.Context, _ string, _ inventory.PSAUpdateFields) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseBuyCost(_ context.Context, _ string, _ int) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchasePriceOverride(_ context.Context, _ string, _ int, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseAISuggestion(_ context.Context, _ string, _ int) error {
	return nil
}
func (r *stubPurchaseRepo) ClearPurchaseAISuggestion(_ context.Context, _ string) error { return nil }
func (r *stubPurchaseRepo) AcceptAISuggestion(_ context.Context, _ string, _ int) error { return nil }
func (r *stubPurchaseRepo) GetPriceOverrideStats(_ context.Context) (*inventory.PriceOverrideStats, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) SetReceivedAt(_ context.Context, _ string, _ time.Time) error { return nil }
func (r *stubPurchaseRepo) SetEbayExportFlag(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (r *stubPurchaseRepo) ClearEbayExportFlags(_ context.Context, _ []string) error { return nil }
func (r *stubPurchaseRepo) ListEbayFlaggedPurchases(_ context.Context) ([]inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseCardYear(_ context.Context, _, _ string) error { return nil }
func (r *stubPurchaseRepo) ListSnapshotPurchasesByStatus(_ context.Context, _ inventory.SnapshotStatus, _ int) ([]inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseSnapshotStatus(_ context.Context, _ string, _ inventory.SnapshotStatus, _ int) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHFields(_ context.Context, _ string, _ inventory.DHFieldsUpdate) error {
	return nil
}
func (r *stubPurchaseRepo) GetPurchasesByDHCertStatus(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHPushStatus(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) IncrementDHPushAttempts(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHStatus(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) ListStaleDHStatusSoldPurchases(_ context.Context) ([]string, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHCardID(_ context.Context, _ string, _ int) error {
	return nil
}
func (r *stubPurchaseRepo) GetPurchasesByDHPushStatus(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) CountUnsoldByDHPushStatus(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) CountDHPipelineHealth(_ context.Context) (inventory.DHPipelineHealth, error) {
	return inventory.DHPipelineHealth{}, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHCandidates(_ context.Context, _, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHHoldReason(_ context.Context, _, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) SetHeldWithReason(_ context.Context, _, _ string) error { return nil }
func (r *stubPurchaseRepo) ApproveHeldPurchase(_ context.Context, _ string) error  { return nil }
func (r *stubPurchaseRepo) ResetDHFieldsForRepush(_ context.Context, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) ResetDHFieldsForRepushDueToDelete(_ context.Context, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHPriceSync(_ context.Context, _ string, _ int, _ time.Time) error {
	return nil
}
func (r *stubPurchaseRepo) UnmatchPurchaseDH(_ context.Context, _ string, _ string) error {
	return nil
}

func (r *stubPurchaseRepo) ListDHPriceDrift(_ context.Context) ([]inventory.Purchase, error) {
	return nil, nil
}

type stubAnalyticsRepo struct {
	data []inventory.PurchaseWithSale
}

func (r *stubAnalyticsRepo) GetCampaignPNL(_ context.Context, _ string) (*inventory.CampaignPNL, error) {
	return nil, nil
}
func (r *stubAnalyticsRepo) GetPNLByChannel(_ context.Context, _ string) ([]inventory.ChannelPNL, error) {
	return nil, nil
}
func (r *stubAnalyticsRepo) GetDailySpend(_ context.Context, _ string, _ int) ([]inventory.DailySpend, error) {
	return nil, nil
}
func (r *stubAnalyticsRepo) GetDaysToSellDistribution(_ context.Context, _ string) ([]inventory.DaysToSellBucket, error) {
	return nil, nil
}
func (r *stubAnalyticsRepo) GetPerformanceByGrade(_ context.Context, _ string) ([]inventory.GradePerformance, error) {
	return nil, nil
}
func (r *stubAnalyticsRepo) GetPurchasesWithSales(_ context.Context, _ string) ([]inventory.PurchaseWithSale, error) {
	return r.data, nil
}
func (r *stubAnalyticsRepo) GetAllPurchasesWithSales(_ context.Context, _ ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
	return r.data, nil
}
func (r *stubAnalyticsRepo) GetPortfolioChannelVelocity(_ context.Context) ([]inventory.ChannelVelocity, error) {
	return nil, nil
}
func (r *stubAnalyticsRepo) GetGlobalPNLByChannel(_ context.Context) ([]inventory.ChannelPNL, error) {
	return nil, nil
}
func (r *stubAnalyticsRepo) GetDailyCapitalTimeSeries(_ context.Context) ([]inventory.DailyCapitalPoint, error) {
	return nil, nil
}

type stubPriceProvider struct {
	rawCents    int
	gradedCents int
}

func (p *stubPriceProvider) GetLastSoldCents(_ context.Context, _ inventory.CardIdentity, grade float64) (int, error) {
	if grade == 0 {
		return p.rawCents, nil
	}
	return p.gradedCents, nil
}

func (p *stubPriceProvider) GetMarketSnapshot(_ context.Context, _ inventory.CardIdentity, _ float64) (*inventory.MarketSnapshot, error) {
	return nil, nil
}

type stubFinanceRepo struct{}

func (r *stubFinanceRepo) CreateInvoice(_ context.Context, _ *inventory.Invoice) error { return nil }
func (r *stubFinanceRepo) GetInvoice(_ context.Context, _ string) (*inventory.Invoice, error) {
	return nil, nil
}
func (r *stubFinanceRepo) ListInvoices(_ context.Context) ([]inventory.Invoice, error) {
	return nil, nil
}
func (r *stubFinanceRepo) UpdateInvoice(_ context.Context, _ *inventory.Invoice) error { return nil }
func (r *stubFinanceRepo) SumPurchaseCostByInvoiceDate(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (r *stubFinanceRepo) GetPendingReceiptByInvoiceDate(_ context.Context, _ []string) (map[string]int, error) {
	return nil, nil
}
func (r *stubFinanceRepo) GetInvoiceSellThrough(_ context.Context, _ string) (inventory.InvoiceSellThrough, error) {
	return inventory.InvoiceSellThrough{}, nil
}
func (r *stubFinanceRepo) GetCashflowConfig(_ context.Context) (*inventory.CashflowConfig, error) {
	return nil, nil
}
func (r *stubFinanceRepo) UpdateCashflowConfig(_ context.Context, _ *inventory.CashflowConfig) error {
	return nil
}
func (r *stubFinanceRepo) GetCapitalRawData(_ context.Context) (*inventory.CapitalRawData, error) {
	return nil, nil
}
func (r *stubFinanceRepo) CreateRevocationFlag(_ context.Context, _ *inventory.RevocationFlag) error {
	return nil
}
func (r *stubFinanceRepo) ListRevocationFlags(_ context.Context) ([]inventory.RevocationFlag, error) {
	return nil, nil
}
func (r *stubFinanceRepo) GetLatestRevocationFlag(_ context.Context) (*inventory.RevocationFlag, error) {
	return nil, nil
}
func (r *stubFinanceRepo) GetRevocationFlagByID(_ context.Context, _ string) (*inventory.RevocationFlag, error) {
	return nil, nil
}
func (r *stubFinanceRepo) UpdateRevocationFlagStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}

type errAnalyticsRepo struct {
	err error
}

func (r *errAnalyticsRepo) GetCampaignPNL(_ context.Context, _ string) (*inventory.CampaignPNL, error) {
	return nil, nil
}
func (r *errAnalyticsRepo) GetPNLByChannel(_ context.Context, _ string) ([]inventory.ChannelPNL, error) {
	return nil, nil
}
func (r *errAnalyticsRepo) GetDailySpend(_ context.Context, _ string, _ int) ([]inventory.DailySpend, error) {
	return nil, nil
}
func (r *errAnalyticsRepo) GetDaysToSellDistribution(_ context.Context, _ string) ([]inventory.DaysToSellBucket, error) {
	return nil, nil
}
func (r *errAnalyticsRepo) GetPerformanceByGrade(_ context.Context, _ string) ([]inventory.GradePerformance, error) {
	return nil, nil
}
func (r *errAnalyticsRepo) GetPurchasesWithSales(_ context.Context, _ string) ([]inventory.PurchaseWithSale, error) {
	return nil, r.err
}
func (r *errAnalyticsRepo) GetAllPurchasesWithSales(_ context.Context, _ ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
	return nil, r.err
}
func (r *errAnalyticsRepo) GetPortfolioChannelVelocity(_ context.Context) ([]inventory.ChannelVelocity, error) {
	return nil, nil
}
func (r *errAnalyticsRepo) GetGlobalPNLByChannel(_ context.Context) ([]inventory.ChannelPNL, error) {
	return nil, nil
}
func (r *errAnalyticsRepo) GetDailyCapitalTimeSeries(_ context.Context) ([]inventory.DailyCapitalPoint, error) {
	return nil, nil
}

type stubFinanceRepoWithData struct {
	capital  *inventory.CapitalRawData
	invoices []inventory.Invoice
}

func (r *stubFinanceRepoWithData) CreateInvoice(_ context.Context, _ *inventory.Invoice) error {
	return nil
}
func (r *stubFinanceRepoWithData) GetInvoice(_ context.Context, _ string) (*inventory.Invoice, error) {
	return nil, nil
}
func (r *stubFinanceRepoWithData) ListInvoices(_ context.Context) ([]inventory.Invoice, error) {
	return r.invoices, nil
}
func (r *stubFinanceRepoWithData) UpdateInvoice(_ context.Context, _ *inventory.Invoice) error {
	return nil
}
func (r *stubFinanceRepoWithData) SumPurchaseCostByInvoiceDate(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (r *stubFinanceRepoWithData) GetPendingReceiptByInvoiceDate(_ context.Context, _ []string) (map[string]int, error) {
	return nil, nil
}
func (r *stubFinanceRepoWithData) GetInvoiceSellThrough(_ context.Context, _ string) (inventory.InvoiceSellThrough, error) {
	return inventory.InvoiceSellThrough{}, nil
}
func (r *stubFinanceRepoWithData) GetCashflowConfig(_ context.Context) (*inventory.CashflowConfig, error) {
	return nil, nil
}
func (r *stubFinanceRepoWithData) UpdateCashflowConfig(_ context.Context, _ *inventory.CashflowConfig) error {
	return nil
}
func (r *stubFinanceRepoWithData) GetCapitalRawData(_ context.Context) (*inventory.CapitalRawData, error) {
	return r.capital, nil
}
func (r *stubFinanceRepoWithData) CreateRevocationFlag(_ context.Context, _ *inventory.RevocationFlag) error {
	return nil
}
func (r *stubFinanceRepoWithData) ListRevocationFlags(_ context.Context) ([]inventory.RevocationFlag, error) {
	return nil, nil
}
func (r *stubFinanceRepoWithData) GetLatestRevocationFlag(_ context.Context) (*inventory.RevocationFlag, error) {
	return nil, nil
}
func (r *stubFinanceRepoWithData) GetRevocationFlagByID(_ context.Context, _ string) (*inventory.RevocationFlag, error) {
	return nil, nil
}
func (r *stubFinanceRepoWithData) UpdateRevocationFlagStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}

type stubBatchPricer struct {
	cardIDs       map[string]int
	distributions map[int]GradedDistribution
}

func (s *stubBatchPricer) ResolveDHCardID(_ context.Context, cardName, setName, cardNumber string) (int, error) {
	return s.cardIDs[cardName+"|"+setName+"|"+cardNumber], nil
}

func (s *stubBatchPricer) BatchPriceDistribution(_ context.Context, cardIDs []int) (map[int]GradedDistribution, error) {
	out := make(map[int]GradedDistribution, len(cardIDs))
	for _, id := range cardIDs {
		if d, ok := s.distributions[id]; ok {
			out[id] = d
		}
	}
	return out, nil
}
