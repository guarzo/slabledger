package portfolio_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newPortfolioSvc creates a portfolio.Service using a single mock repository
// for all three dependency roles (campaigns, analytics, finance).
func newPortfolioSvc(repo *mocks.InMemoryCampaignStore) portfolio.Service {
	return portfolio.NewService(repo, repo, repo, mocks.NewMockLogger())
}

func TestService_GetPortfolioHealth_Healthy(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c := &inventory.Campaign{ID: "c1", Name: "Profitable", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c.ID] = c
	repo.PNLData[c.ID] = &inventory.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   100000,
		TotalRevenueCents: 120000,
		TotalFeesCents:    15000,
		ROI:               0.05,
		TotalPurchases:    10,
		TotalSold:         8,
		TotalUnsold:       2,
		SellThroughPct:    0.80,
		AvgDaysToSell:     14,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if len(health.Campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(health.Campaigns))
	}
	if health.Campaigns[0].HealthStatus != "healthy" {
		t.Errorf("HealthStatus = %q, want healthy", health.Campaigns[0].HealthStatus)
	}
	if health.TotalDeployed != 100000 {
		t.Errorf("TotalDeployed = %d, want 100000", health.TotalDeployed)
	}
}

func TestService_GetPortfolioHealth_Warning(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c := &inventory.Campaign{ID: "c1", Name: "Losing", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c.ID] = c
	repo.PNLData[c.ID] = &inventory.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   100000,
		TotalRevenueCents: 90000,
		TotalFeesCents:    10000,
		ROI:               -0.05,
		TotalPurchases:    10,
		TotalSold:         5,
		TotalUnsold:       5,
		AvgDaysToSell:     20,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if health.Campaigns[0].HealthStatus != "warning" {
		t.Errorf("HealthStatus = %q, want warning (ROI -5%%)", health.Campaigns[0].HealthStatus)
	}
}

func TestService_GetPortfolioHealth_Critical(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c := &inventory.Campaign{ID: "c1", Name: "Bleeding", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c.ID] = c
	repo.PNLData[c.ID] = &inventory.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   200000,
		TotalRevenueCents: 100000,
		TotalFeesCents:    15000,
		ROI:               -0.50,
		TotalPurchases:    20,
		TotalSold:         5,
		TotalUnsold:       15,
		AvgDaysToSell:     60,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if health.Campaigns[0].HealthStatus != "critical" {
		t.Errorf("HealthStatus = %q, want critical (ROI -50%%, 15 unsold)", health.Campaigns[0].HealthStatus)
	}
}

func TestService_GetPortfolioHealth_SlowSellThrough(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c := &inventory.Campaign{ID: "c1", Name: "Slow", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c.ID] = c
	repo.PNLData[c.ID] = &inventory.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   100000,
		TotalRevenueCents: 110000,
		TotalFeesCents:    5000,
		ROI:               0.05,
		TotalPurchases:    10,
		TotalSold:         3,
		TotalUnsold:       7,
		AvgDaysToSell:     50, // > 45 threshold
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	// Positive ROI but slow sell-through → warning
	if health.Campaigns[0].HealthStatus != "warning" {
		t.Errorf("HealthStatus = %q, want warning (slow sell-through)", health.Campaigns[0].HealthStatus)
	}
}

func TestService_GetPortfolioHealth_OverallROI(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c1 := &inventory.Campaign{ID: "c1", Name: "Campaign A", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	c2 := &inventory.Campaign{ID: "c2", Name: "Campaign B", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c1.ID] = c1
	repo.Campaigns[c2.ID] = c2

	repo.PNLData[c1.ID] = &inventory.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 100000, TotalRevenueCents: 120000, TotalFeesCents: 10000,
		ROI: 0.10, TotalSold: 5, TotalUnsold: 0,
	}
	repo.PNLData[c2.ID] = &inventory.CampaignPNL{
		CampaignID: c2.ID, TotalSpendCents: 50000, TotalRevenueCents: 40000, TotalFeesCents: 5000,
		ROI: -0.20, TotalSold: 3, TotalUnsold: 2,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if len(health.Campaigns) != 2 {
		t.Fatalf("expected 2 campaigns, got %d", len(health.Campaigns))
	}
	// Total deployed = 150000, total recovered = (120000-10000) + (40000-5000) = 145000
	if health.TotalDeployed != 150000 {
		t.Errorf("TotalDeployed = %d, want 150000", health.TotalDeployed)
	}
	expectedRecovered := 110000 + 35000
	if health.TotalRecovered != expectedRecovered {
		t.Errorf("TotalRecovered = %d, want %d", health.TotalRecovered, expectedRecovered)
	}
}

func TestService_GetPortfolioHealth_RealizedROI(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c1 := &inventory.Campaign{ID: "c1", Name: "Fully Sold", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	c2 := &inventory.Campaign{ID: "c2", Name: "Mixed", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c1.ID] = c1
	repo.Campaigns[c2.ID] = c2

	// Campaign 1: all sold (5/5) → soldCostBasis=100000, soldProfit=120000-10000-100000=10000
	repo.PNLData[c1.ID] = &inventory.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 100000, TotalRevenueCents: 120000, TotalFeesCents: 10000,
		ROI: 0.10, TotalSold: 5, TotalUnsold: 0, TotalPurchases: 5,
	}
	// Campaign 2: partially sold (2/5) → soldCostBasis=20000, soldProfit=30000-3000-20000=7000
	repo.PNLData[c2.ID] = &inventory.CampaignPNL{
		CampaignID: c2.ID, TotalSpendCents: 50000, TotalRevenueCents: 30000, TotalFeesCents: 3000,
		ROI: -0.20, TotalSold: 2, TotalUnsold: 3, TotalPurchases: 5,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	// totalSoldCostBasis = 100000 + 20000 = 120000
	// totalSoldNetProfit = 10000 + 7000 = 17000
	// RealizedROI = 17000 / 120000 ≈ 0.14167
	wantROI := float64(17000) / float64(120000)
	if diff := health.RealizedROI - wantROI; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("RealizedROI = %f, want ~%f", health.RealizedROI, wantROI)
	}
}

func TestService_GetPortfolioHealth_RealizedROI_Rounding(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c1 := &inventory.Campaign{ID: "c1", Name: "Odd Split", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c1.ID] = c1

	// 1000 cents spent, 1 of 3 sold → soldCostBasis = round(1000*1/3) = round(333.33) = 333
	// soldProfit = 500 - 50 - 333 = 117
	repo.PNLData[c1.ID] = &inventory.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 1000, TotalRevenueCents: 500, TotalFeesCents: 50,
		ROI: 0, TotalSold: 1, TotalUnsold: 2, TotalPurchases: 3,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	wantROI := float64(117) / float64(333)
	if diff := health.RealizedROI - wantROI; diff > 0.001 || diff < -0.001 {
		t.Errorf("RealizedROI = %f, want ~%f", health.RealizedROI, wantROI)
	}
}

func TestService_GetPortfolioHealth_RealizedROI_NoSales(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c1 := &inventory.Campaign{ID: "c1", Name: "No Sales", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c1.ID] = c1
	repo.PNLData[c1.ID] = &inventory.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 50000, TotalRevenueCents: 0, TotalFeesCents: 0,
		ROI: 0, TotalSold: 0, TotalUnsold: 5, TotalPurchases: 5,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if health.RealizedROI != 0 {
		t.Errorf("RealizedROI = %f, want 0 (no sales)", health.RealizedROI)
	}
}

func TestService_GetPortfolioHealth_LiquidationSignals(t *testing.T) {
	type saleFixture struct {
		channel   inventory.SaleChannel
		salePrice int
		netProfit int
	}

	cases := []struct {
		name                 string
		sales                []saleFixture
		wantLossCents        int
		wantLossCount        int
		wantEbayMarginPct    float64
		ebayMarginPctEpsilon float64
	}{
		{
			name: "no sales", sales: nil, wantLossCents: 0, wantLossCount: 0,
			wantEbayMarginPct: 0, ebayMarginPctEpsilon: 0.00001,
		},
		{
			name: "only eBay sales all profitable",
			sales: []saleFixture{
				{channel: inventory.SaleChannelEbay, salePrice: 10000, netProfit: 2000},
				{channel: inventory.SaleChannelEbay, salePrice: 15000, netProfit: 4000},
			},
			wantLossCents: 0, wantLossCount: 0,
			wantEbayMarginPct: float64(6000) / float64(25000), ebayMarginPctEpsilon: 0.0001,
		},
		{
			name: "only eBay sales some losses — loss stays 0, margin still computed",
			sales: []saleFixture{
				{channel: inventory.SaleChannelEbay, salePrice: 10000, netProfit: 2000},
				{channel: inventory.SaleChannelEbay, salePrice: 8000, netProfit: -1500},
			},
			wantLossCents: 0, wantLossCount: 0,
			wantEbayMarginPct: float64(500) / float64(18000), ebayMarginPctEpsilon: 0.0001,
		},
		{
			name: "only inperson sales all profitable — loss stays 0",
			sales: []saleFixture{
				{channel: inventory.SaleChannelInPerson, salePrice: 5000, netProfit: 1000},
				{channel: inventory.SaleChannelInPerson, salePrice: 7000, netProfit: 500},
			},
			wantLossCents: 0, wantLossCount: 0, wantEbayMarginPct: 0, ebayMarginPctEpsilon: 0.00001,
		},
		{
			name: "only inperson sales all losses",
			sales: []saleFixture{
				{channel: inventory.SaleChannelInPerson, salePrice: 4000, netProfit: -800},
				{channel: inventory.SaleChannelInPerson, salePrice: 3000, netProfit: -1200},
			},
			wantLossCents: -2000, wantLossCount: 2, wantEbayMarginPct: 0, ebayMarginPctEpsilon: 0.00001,
		},
		{
			name: "mixed channels — eBay profitable, inperson losses",
			sales: []saleFixture{
				{channel: inventory.SaleChannelEbay, salePrice: 20000, netProfit: 5000},
				{channel: inventory.SaleChannelEbay, salePrice: 10000, netProfit: 3000},
				{channel: inventory.SaleChannelInPerson, salePrice: 5000, netProfit: -1500},
				{channel: inventory.SaleChannelInPerson, salePrice: 4000, netProfit: -2000},
			},
			wantLossCents: -3500, wantLossCount: 2,
			wantEbayMarginPct: float64(8000) / float64(30000), ebayMarginPctEpsilon: 0.0001,
		},
		{
			name: "cardshow losses counted alongside inperson",
			sales: []saleFixture{
				{channel: inventory.SaleChannelCardShow, salePrice: 5000, netProfit: -1000},
				{channel: inventory.SaleChannelInPerson, salePrice: 4500, netProfit: -750},
			},
			wantLossCents: -1750, wantLossCount: 2, wantEbayMarginPct: 0, ebayMarginPctEpsilon: 0.00001,
		},
		{
			name: "profitable cardshow sale does not reduce liquidation loss",
			sales: []saleFixture{
				{channel: inventory.SaleChannelCardShow, salePrice: 5000, netProfit: 1500},
				{channel: inventory.SaleChannelCardShow, salePrice: 4000, netProfit: -900},
				{channel: inventory.SaleChannelInPerson, salePrice: 3000, netProfit: -600},
			},
			wantLossCents: -1500, wantLossCount: 2, wantEbayMarginPct: 0, ebayMarginPctEpsilon: 0.00001,
		},
		{
			name: "website and other channels are neither liquidation nor marketplace",
			sales: []saleFixture{
				{channel: inventory.SaleChannelWebsite, salePrice: 10000, netProfit: -500},
				{channel: inventory.SaleChannelOther, salePrice: 5000, netProfit: -1000},
				{channel: inventory.SaleChannelEbay, salePrice: 20000, netProfit: 4000},
			},
			wantLossCents: 0, wantLossCount: 0,
			wantEbayMarginPct: float64(4000) / float64(20000), ebayMarginPctEpsilon: 0.0001,
		},
		{
			name: "tcgplayer sales are counted with eBay for marketplace margin",
			sales: []saleFixture{
				{channel: inventory.SaleChannelEbay, salePrice: 10000, netProfit: 2000},
				{channel: inventory.SaleChannelTCGPlayer, salePrice: 5000, netProfit: 1000},
			},
			wantLossCents: 0, wantLossCount: 0,
			wantEbayMarginPct: float64(3000) / float64(15000), ebayMarginPctEpsilon: 0.0001,
		},
		{
			name: "tcgplayer-only sales still populate marketplace margin",
			sales: []saleFixture{
				{channel: inventory.SaleChannelTCGPlayer, salePrice: 8000, netProfit: 1200},
			},
			wantLossCents: 0, wantLossCount: 0,
			wantEbayMarginPct: float64(1200) / float64(8000), ebayMarginPctEpsilon: 0.0001,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			svc := newPortfolioSvc(repo)
			ctx := context.Background()

			c := &inventory.Campaign{ID: "c1", Name: "Health Test", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
			repo.Campaigns[c.ID] = c
			repo.PNLData[c.ID] = &inventory.CampaignPNL{
				CampaignID:      c.ID,
				TotalSpendCents: 100000,
				TotalPurchases:  10,
				TotalSold:       len(tc.sales),
				ROI:             0.01,
			}

			fixtures := make([]inventory.PurchaseWithSale, 0, len(tc.sales))
			for i, sf := range tc.sales {
				fixtures = append(fixtures, inventory.PurchaseWithSale{
					Purchase: inventory.Purchase{ID: fmt.Sprintf("p-%d", i), CampaignID: c.ID},
					Sale: &inventory.Sale{
						PurchaseID:     fmt.Sprintf("p-%d", i),
						SaleChannel:    sf.channel,
						SalePriceCents: sf.salePrice,
						NetProfitCents: sf.netProfit,
					},
				})
			}

			repo.GetAllPurchasesWithSalesFn = func(_ context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
				return fixtures, nil
			}

			health, err := svc.GetPortfolioHealth(ctx)
			if err != nil {
				t.Fatalf("GetPortfolioHealth: %v", err)
			}
			if len(health.Campaigns) != 1 {
				t.Fatalf("expected 1 campaign, got %d", len(health.Campaigns))
			}
			got := health.Campaigns[0]

			if got.LiquidationLossCents != tc.wantLossCents {
				t.Errorf("LiquidationLossCents = %d, want %d", got.LiquidationLossCents, tc.wantLossCents)
			}
			if got.LiquidationSaleCount != tc.wantLossCount {
				t.Errorf("LiquidationSaleCount = %d, want %d", got.LiquidationSaleCount, tc.wantLossCount)
			}
			if diff := got.EbayChannelMarginPct - tc.wantEbayMarginPct; diff > tc.ebayMarginPctEpsilon || diff < -tc.ebayMarginPctEpsilon {
				t.Errorf("EbayChannelMarginPct = %f, want ~%f", got.EbayChannelMarginPct, tc.wantEbayMarginPct)
			}
		})
	}
}

func TestService_GetPortfolioHealth_LiquidationReason(t *testing.T) {
	cases := []struct {
		name          string
		pnl           inventory.CampaignPNL
		sales         []inventory.PurchaseWithSale
		wantStatus    string
		wantReasonHas []string
		wantLossCents int
		wantLossCount int
	}{
		{
			name: "healthy base is downgraded to warning with liquidation reason",
			pnl: inventory.CampaignPNL{
				TotalSpendCents: 100000, TotalPurchases: 10, TotalSold: 6, ROI: 0.02,
			},
			sales: []inventory.PurchaseWithSale{
				{Purchase: inventory.Purchase{ID: "p1"}, Sale: &inventory.Sale{PurchaseID: "p1", SaleChannel: inventory.SaleChannelEbay, SalePriceCents: 20000, NetProfitCents: 4000}},
				{Purchase: inventory.Purchase{ID: "p2"}, Sale: &inventory.Sale{PurchaseID: "p2", SaleChannel: inventory.SaleChannelInPerson, SalePriceCents: 3000, NetProfitCents: -60000}},
				{Purchase: inventory.Purchase{ID: "p3"}, Sale: &inventory.Sale{PurchaseID: "p3", SaleChannel: inventory.SaleChannelCardShow, SalePriceCents: 2500, NetProfitCents: -40000}},
			},
			wantStatus:    "warning",
			wantReasonHas: []string{"forced liquidation", "marketplace channels profitable"},
			wantLossCents: -100000,
			wantLossCount: 2,
		},
		{
			name: "critical base is preserved and liquidation is appended",
			pnl: inventory.CampaignPNL{
				TotalSpendCents: 100000, TotalPurchases: 10, TotalSold: 4, TotalUnsold: 6, ROI: -0.15,
			},
			sales: []inventory.PurchaseWithSale{
				{Purchase: inventory.Purchase{ID: "p1"}, Sale: &inventory.Sale{PurchaseID: "p1", SaleChannel: inventory.SaleChannelEbay, SalePriceCents: 20000, NetProfitCents: 4000}},
				{Purchase: inventory.Purchase{ID: "p2"}, Sale: &inventory.Sale{PurchaseID: "p2", SaleChannel: inventory.SaleChannelInPerson, SalePriceCents: 3000, NetProfitCents: -60000}},
				{Purchase: inventory.Purchase{ID: "p3"}, Sale: &inventory.Sale{PurchaseID: "p3", SaleChannel: inventory.SaleChannelCardShow, SalePriceCents: 2500, NetProfitCents: -40000}},
			},
			wantStatus:    "critical",
			wantReasonHas: []string{"Significant negative ROI", "forced liquidation"},
			wantLossCents: -100000,
			wantLossCount: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			svc := newPortfolioSvc(repo)
			ctx := context.Background()

			c := &inventory.Campaign{ID: "c1", Name: "Modern", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
			repo.Campaigns[c.ID] = c
			pnl := tc.pnl
			pnl.CampaignID = c.ID
			repo.PNLData[c.ID] = &pnl

			sales := make([]inventory.PurchaseWithSale, len(tc.sales))
			copy(sales, tc.sales)
			for i := range sales {
				sales[i].Purchase.CampaignID = c.ID
			}
			repo.GetAllPurchasesWithSalesFn = func(_ context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
				return sales, nil
			}

			health, err := svc.GetPortfolioHealth(ctx)
			if err != nil {
				t.Fatalf("GetPortfolioHealth: %v", err)
			}
			if len(health.Campaigns) != 1 {
				t.Fatalf("expected 1 campaign, got %d", len(health.Campaigns))
			}
			got := health.Campaigns[0]

			if got.LiquidationLossCents != tc.wantLossCents {
				t.Errorf("LiquidationLossCents = %d, want %d", got.LiquidationLossCents, tc.wantLossCents)
			}
			if got.LiquidationSaleCount != tc.wantLossCount {
				t.Errorf("LiquidationSaleCount = %d, want %d", got.LiquidationSaleCount, tc.wantLossCount)
			}
			if got.HealthStatus != tc.wantStatus {
				t.Errorf("HealthStatus = %q, want %q", got.HealthStatus, tc.wantStatus)
			}
			for _, want := range tc.wantReasonHas {
				if !strings.Contains(got.HealthReason, want) {
					t.Errorf("HealthReason = %q, want substring %q", got.HealthReason, want)
				}
			}
		})
	}
}

func TestService_GetPortfolioChannelVelocity_Empty(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	velocity, err := svc.GetPortfolioChannelVelocity(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioChannelVelocity: %v", err)
	}
	if len(velocity) != 0 {
		t.Errorf("expected empty result, got %d entries", len(velocity))
	}
}

func TestService_GetPortfolioChannelVelocity_WithData(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.ChannelVelocity = []inventory.ChannelVelocity{
		{Channel: inventory.SaleChannelEbay, SaleCount: 10, AvgDaysToSell: 14.5, RevenueCents: 500000},
		{Channel: inventory.SaleChannelInPerson, SaleCount: 5, AvgDaysToSell: 0, RevenueCents: 250000},
		{Channel: inventory.SaleChannelWebsite, SaleCount: 3, AvgDaysToSell: 7.0, RevenueCents: 120000},
	}
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	velocity, err := svc.GetPortfolioChannelVelocity(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioChannelVelocity: %v", err)
	}
	if len(velocity) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(velocity))
	}

	if velocity[0].Channel != inventory.SaleChannelEbay {
		t.Errorf("velocity[0].Channel = %q, want ebay", velocity[0].Channel)
	}
	if velocity[0].SaleCount != 10 {
		t.Errorf("velocity[0].SaleCount = %d, want 10", velocity[0].SaleCount)
	}
	if velocity[0].AvgDaysToSell != 14.5 {
		t.Errorf("velocity[0].AvgDaysToSell = %f, want 14.5", velocity[0].AvgDaysToSell)
	}
	if velocity[0].RevenueCents != 500000 {
		t.Errorf("velocity[0].RevenueCents = %d, want 500000", velocity[0].RevenueCents)
	}
	if velocity[1].Channel != inventory.SaleChannelInPerson {
		t.Errorf("velocity[1].Channel = %q, want inperson", velocity[1].Channel)
	}
	if velocity[2].Channel != inventory.SaleChannelWebsite {
		t.Errorf("velocity[2].Channel = %q, want website", velocity[2].Channel)
	}
}

// end of tests

func TestService_GetWeeklyReviewSummary_PerformerCounts(t *testing.T) {
	cases := []struct {
		name          string
		salesCount    int
		wantTopLen    int
		wantBottomLen int
	}{
		{
			name:          "12 sales: top 5, bottom 5",
			salesCount:    12,
			wantTopLen:    5,
			wantBottomLen: 5,
		},
		{
			name:          "7 sales: top 5, bottom 2",
			salesCount:    7,
			wantTopLen:    5,
			wantBottomLen: 2,
		},
		{
			name:          "4 sales: all top, no bottom",
			salesCount:    4,
			wantTopLen:    4,
			wantBottomLen: 0,
		},
		{
			name:          "11 sales: top 5, bottom 5 (exactly > 2*5)",
			salesCount:    11,
			wantTopLen:    5,
			wantBottomLen: 5,
		},
		{
			name:          "10 sales: top 5, bottom 5 (= 2*5, >5 branch)",
			salesCount:    10,
			wantTopLen:    5,
			wantBottomLen: 5,
		},
		{
			name:          "5 sales: all top, no bottom (= maxPerformers, else branch)",
			salesCount:    5,
			wantTopLen:    5,
			wantBottomLen: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			svc := newPortfolioSvc(repo)
			ctx := context.Background()

			// Use today's date so sales land in "this week"
			today := time.Now().Format("2006-01-02")

			fixtures := make([]inventory.PurchaseWithSale, tc.salesCount)
			for i := range fixtures {
				fixtures[i] = inventory.PurchaseWithSale{
					Purchase: inventory.Purchase{
						ID:         fmt.Sprintf("p-%d", i),
						CampaignID: "c1",
						CardName:   fmt.Sprintf("Card %d", i),
					},
					Sale: &inventory.Sale{
						PurchaseID:     fmt.Sprintf("p-%d", i),
						SaleChannel:    inventory.SaleChannelEbay,
						SalePriceCents: 10000 + i*1000,
						NetProfitCents: (i + 1) * 100, // distinct profits for deterministic ordering
						SaleDate:       today,
					},
				}
			}

			repo.GetAllPurchasesWithSalesFn = func(_ context.Context, _ ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
				return fixtures, nil
			}

			summary, err := svc.GetWeeklyReviewSummary(ctx)
			if err != nil {
				t.Fatalf("GetWeeklyReviewSummary: %v", err)
			}

			if len(summary.TopPerformers) != tc.wantTopLen {
				t.Errorf("TopPerformers len = %d, want %d", len(summary.TopPerformers), tc.wantTopLen)
			}
			if len(summary.BottomPerformers) != tc.wantBottomLen {
				t.Errorf("BottomPerformers len = %d, want %d", len(summary.BottomPerformers), tc.wantBottomLen)
			}
		})
	}
}

func TestService_GetWeeklyHistory(t *testing.T) {
	cases := []struct {
		name     string
		weeks    int
		wantLen  int
	}{
		{name: "explicit 4 weeks", weeks: 4, wantLen: 4},
		{name: "default when <= 0", weeks: 0, wantLen: 8},
		{name: "default when negative", weeks: -3, wantLen: 8},
		{name: "clamped at 52", weeks: 100, wantLen: 52},
		{name: "upper bound exact", weeks: 52, wantLen: 52},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			svc := newPortfolioSvc(repo)
			ctx := context.Background()

			// Empty fixtures are fine — we're verifying windowing and count.
			repo.GetAllPurchasesWithSalesFn = func(_ context.Context, _ ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
				return nil, nil
			}

			summaries, err := svc.GetWeeklyHistory(ctx, tc.weeks)
			if err != nil {
				t.Fatalf("GetWeeklyHistory: %v", err)
			}

			if len(summaries) != tc.wantLen {
				t.Fatalf("summaries len = %d, want %d", len(summaries), tc.wantLen)
			}

			// Verify descending WeekStart order (most recent first).
			for i := 1; i < len(summaries); i++ {
				if summaries[i-1].WeekStart <= summaries[i].WeekStart {
					t.Errorf("summaries[%d].WeekStart %q should be > summaries[%d].WeekStart %q",
						i-1, summaries[i-1].WeekStart, i, summaries[i].WeekStart)
				}
			}

			// Verify that weeks are exactly 7 days apart.
			for i := 1; i < len(summaries); i++ {
				prev, err1 := time.Parse("2006-01-02", summaries[i-1].WeekStart)
				cur, err2 := time.Parse("2006-01-02", summaries[i].WeekStart)
				if err1 != nil || err2 != nil {
					t.Fatalf("parse week starts: %v / %v", err1, err2)
				}
				if diff := prev.Sub(cur); diff != 7*24*time.Hour {
					t.Errorf("summaries[%d] - summaries[%d] = %v, want 168h", i-1, i, diff)
				}
			}
		})
	}
}
