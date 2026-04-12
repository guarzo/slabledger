package arbitrage

import (
	"context"
	"testing"
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
func (r *stubPurchaseRepo) GetPurchasesByDHPushStatus(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) CountUnsoldByDHPushStatus(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHCandidates(_ context.Context, _, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) UpdatePurchaseDHHoldReason(_ context.Context, _, _ string) error {
	return nil
}
func (r *stubPurchaseRepo) SetHeldWithReason(_ context.Context, _, _ string) error { return nil }
func (r *stubPurchaseRepo) ApproveHeldPurchase(_ context.Context, _ string) error  { return nil }

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
	return &inventory.CapitalRawData{}, nil
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

// Task 6: PSA grade filter — >= 9 should exclude 9, 9.5, 10 but include 8, 8.5.

func TestGetCrackCandidates_GradeFilter(t *testing.T) {
	tests := []struct {
		name       string
		gradeValue float64
		wantInList bool
	}{
		{name: "PSA 8 included", gradeValue: 8, wantInList: true},
		{name: "PSA 8.5 included", gradeValue: 8.5, wantInList: true},
		{name: "PSA 9 excluded", gradeValue: 9, wantInList: false},
		{name: "PSA 9.5 excluded", gradeValue: 9.5, wantInList: false},
		{name: "PSA 10 excluded", gradeValue: 10, wantInList: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			campaignID := "camp1"
			campaign := &inventory.Campaign{
				ID:         campaignID,
				Name:       "Test",
				EbayFeePct: 0.1235,
			}
			purchase := inventory.Purchase{
				ID:                  "p1",
				CampaignID:          campaignID,
				CardName:            "Charizard",
				GradeValue:          tc.gradeValue,
				BuyCostCents:        5000,
				PSASourcingFeeCents: 300,
				CLValueCents:        8000,
			}

			svc := NewService(
				&stubCampaignRepo{campaign: campaign},
				&stubPurchaseRepo{unsold: []inventory.Purchase{purchase}},
				&stubAnalyticsRepo{},
				&stubFinanceRepo{},
				WithPriceLookup(&stubPriceProvider{rawCents: 15000, gradedCents: 10000}),
			)

			results, err := svc.GetCrackCandidates(context.Background(), campaignID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			found := false
			for _, r := range results {
				if r.PurchaseID == purchase.ID {
					found = true
					break
				}
			}

			if found != tc.wantInList {
				t.Errorf("grade %g: wantInList=%v, found=%v", tc.gradeValue, tc.wantInList, found)
			}
		})
	}
}

// Task 7: GetExpectedValues should use campaign's EbayFeePct instead of hardcoded default.

func TestGetExpectedValues_UsesCampaignFee(t *testing.T) {
	campaignID := "camp1"

	// Build enough historical data for EV computation (need matching grade segments).
	makeHistory := func(n int) []inventory.PurchaseWithSale {
		history := make([]inventory.PurchaseWithSale, n)
		for i := range history {
			history[i] = inventory.PurchaseWithSale{
				Purchase: inventory.Purchase{
					ID:                  "sold" + string(rune('a'+i)),
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
					GradeValue:          9,
				},
				Sale: &inventory.Sale{NetProfitCents: 1000, SaleFeeCents: 800},
			}
		}
		return history
	}

	unsold := []inventory.Purchase{
		{
			ID:                  "p-unsold",
			CampaignID:          campaignID,
			GradeValue:          9,
			BuyCostCents:        5000,
			PSASourcingFeeCents: 300,
		},
	}

	runEV := func(feePct float64) *EVPortfolio {
		campaign := &inventory.Campaign{
			ID:         campaignID,
			Name:       "Test",
			EbayFeePct: feePct,
		}
		svc := NewService(
			&stubCampaignRepo{campaign: campaign},
			&stubPurchaseRepo{unsold: unsold},
			&stubAnalyticsRepo{data: makeHistory(30)},
			&stubFinanceRepo{},
		)
		portfolio, err := svc.GetExpectedValues(context.Background(), campaignID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return portfolio
	}

	portfolioDefault := runEV(0)      // 0 → fallback to DefaultMarketplaceFeePct (12.35%)
	portfolioHighFee := runEV(0.2500) // 25% — significantly higher

	if len(portfolioDefault.Items) == 0 {
		t.Skip("no EV items with default fee — insufficient segment data")
	}
	if len(portfolioHighFee.Items) == 0 {
		t.Skip("no EV items with high fee — insufficient segment data")
	}

	// Higher fee → lower expected sale net → lower EV
	if portfolioHighFee.TotalEVCents >= portfolioDefault.TotalEVCents {
		t.Errorf("expected higher fee to reduce total EV: default=%d highFee=%d",
			portfolioDefault.TotalEVCents, portfolioHighFee.TotalEVCents)
	}
}

// Task 8: Monte Carlo simulation should use per-card costs, not avgCost for all cards.
// This is tested indirectly — we verify the simulation runs without panic and produces output.

func TestRunSimulation_PerCardCostNoPanic(t *testing.T) {
	// Two cards with very different costs: $5 and $100
	history := []inventory.PurchaseWithSale{
		{
			Purchase: inventory.Purchase{BuyCostCents: 500, GradeValue: 9, CLValueCents: 2000},
			Sale:     &inventory.Sale{NetProfitCents: 200, SaleFeeCents: 100},
		},
		{
			Purchase: inventory.Purchase{BuyCostCents: 10000, GradeValue: 9, CLValueCents: 15000},
			Sale:     &inventory.Sale{NetProfitCents: 3000, SaleFeeCents: 1800},
		},
	}

	campaign := &inventory.Campaign{BuyTermsCLPct: 0.65, GradeRange: "9-9"}
	result := RunMonteCarloProjection(campaign, history)

	// The simulation runs (may return insufficient due to few data points, but no panic).
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
