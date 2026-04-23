package portfolio_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestComputePNLByCampaign(t *testing.T) {
	data := []inventory.PurchaseWithSale{
		{
			Purchase: inventory.Purchase{ID: "p1", CampaignID: "c1", BuyCostCents: 5000, PSASourcingFeeCents: 300},
			Sale:     &inventory.Sale{PurchaseID: "p1", SalePriceCents: 8000, SaleFeeCents: 800, NetProfitCents: 1900, DaysToSell: 10},
		},
		{
			Purchase: inventory.Purchase{ID: "p2", CampaignID: "c1", BuyCostCents: 6000, PSASourcingFeeCents: 300},
			Sale:     nil, // unsold
		},
		{
			Purchase: inventory.Purchase{ID: "p3", CampaignID: "c2", BuyCostCents: 10000, PSASourcingFeeCents: 300},
			Sale:     &inventory.Sale{PurchaseID: "p3", SalePriceCents: 15000, SaleFeeCents: 1500, NetProfitCents: 3200, DaysToSell: 20},
		},
	}

	// Use computeHealthFromData to exercise computePNLByCampaign indirectly,
	// since computePNLByCampaign is unexported.
	campaigns := []inventory.Campaign{
		{ID: "c1", Name: "Camp 1", Phase: inventory.PhaseActive},
		{ID: "c2", Name: "Camp 2", Phase: inventory.PhaseActive},
	}
	health := portfolio.ComputeHealthFromData(campaigns, data)

	if len(health.Campaigns) != 2 {
		t.Fatalf("expected 2 campaigns, got %d", len(health.Campaigns))
	}

	// Find campaign c1
	var c1Health inventory.CampaignHealth
	for _, ch := range health.Campaigns {
		if ch.CampaignID == "c1" {
			c1Health = ch
			break
		}
	}

	if c1Health.TotalPurchases != 2 {
		t.Errorf("c1 TotalPurchases = %d, want 2", c1Health.TotalPurchases)
	}
	if c1Health.TotalUnsold != 1 {
		t.Errorf("c1 TotalUnsold = %d, want 1", c1Health.TotalUnsold)
	}
	// ROI = netProfit / totalSpend = 1900 / 11600 ≈ 0.1638
	if c1Health.ROI < 0.16 || c1Health.ROI > 0.17 {
		t.Errorf("c1 ROI = %f, want ~0.1638", c1Health.ROI)
	}
}

func TestComputeHealthFromData_StatusLogic(t *testing.T) {
	cases := []struct {
		name       string
		data       []inventory.PurchaseWithSale
		wantStatus string
	}{
		{
			name: "healthy",
			data: []inventory.PurchaseWithSale{
				{Purchase: inventory.Purchase{ID: "p1", CampaignID: "c1", BuyCostCents: 5000, PSASourcingFeeCents: 300},
					Sale: &inventory.Sale{PurchaseID: "p1", SalePriceCents: 8000, SaleFeeCents: 800, NetProfitCents: 1900, DaysToSell: 10}},
			},
			wantStatus: "healthy",
		},
		{
			name: "warning negative ROI",
			data: []inventory.PurchaseWithSale{
				{Purchase: inventory.Purchase{ID: "p1", CampaignID: "c1", BuyCostCents: 10000, PSASourcingFeeCents: 300},
					Sale: &inventory.Sale{PurchaseID: "p1", SalePriceCents: 5000, SaleFeeCents: 500, NetProfitCents: -5800, DaysToSell: 10}},
			},
			wantStatus: "warning",
		},
		{
			name: "critical deep negative + many unsold",
			data: func() []inventory.PurchaseWithSale {
				var d []inventory.PurchaseWithSale
				// 1 sold at big loss
				d = append(d, inventory.PurchaseWithSale{
					Purchase: inventory.Purchase{ID: "p0", CampaignID: "c1", BuyCostCents: 10000, PSASourcingFeeCents: 300},
					Sale:     &inventory.Sale{PurchaseID: "p0", SalePriceCents: 2000, SaleFeeCents: 200, NetProfitCents: -8500, DaysToSell: 5},
				})
				// 6 unsold
				for i := 1; i <= 6; i++ {
					d = append(d, inventory.PurchaseWithSale{
						Purchase: inventory.Purchase{ID: "pu" + string(rune('0'+i)), CampaignID: "c1", BuyCostCents: 10000, PSASourcingFeeCents: 300},
					})
				}
				return d
			}(),
			wantStatus: "critical",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			campaigns := []inventory.Campaign{{ID: "c1", Name: "Test", Phase: inventory.PhaseActive}}
			health := portfolio.ComputeHealthFromData(campaigns, tc.data)
			if len(health.Campaigns) != 1 {
				t.Fatalf("expected 1 campaign, got %d", len(health.Campaigns))
			}
			if health.Campaigns[0].HealthStatus != tc.wantStatus {
				t.Errorf("HealthStatus = %q, want %q", health.Campaigns[0].HealthStatus, tc.wantStatus)
			}
		})
	}
}

func TestGetSnapshot(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := newPortfolioSvc(repo)
	ctx := context.Background()

	c := &inventory.Campaign{ID: "c1", Name: "Active", BuyTermsCLPct: 0.78, Phase: inventory.PhaseActive}
	repo.Campaigns[c.ID] = c

	callCount := 0
	repo.GetAllPurchasesWithSalesFn = func(_ context.Context, _ ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
		callCount++
		return []inventory.PurchaseWithSale{
			{
				Purchase: inventory.Purchase{ID: "p1", CampaignID: "c1", BuyCostCents: 5000, PSASourcingFeeCents: 300, PurchaseDate: "2026-04-20"},
				Sale:     &inventory.Sale{PurchaseID: "p1", SalePriceCents: 8000, SaleFeeCents: 800, NetProfitCents: 1900, DaysToSell: 10, SaleDate: "2026-04-22", SaleChannel: inventory.SaleChannelEbay},
			},
		}, nil
	}
	repo.GetGlobalPNLByChannelFn = func(_ context.Context) ([]inventory.ChannelPNL, error) {
		return []inventory.ChannelPNL{{Channel: inventory.SaleChannelEbay, SaleCount: 1}}, nil
	}
	repo.GetCapitalRawDataFn = func(_ context.Context) (*inventory.CapitalRawData, error) {
		return &inventory.CapitalRawData{OutstandingCents: 5000, RecoveryRate30dCents: 1000}, nil
	}

	snap, err := svc.GetSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	if snap.Health == nil {
		t.Error("Health is nil")
	}
	if snap.Insights == nil {
		t.Error("Insights is nil")
	}
	if snap.WeeklyReview == nil {
		t.Error("WeeklyReview is nil")
	}
	if snap.Suggestions == nil {
		t.Error("Suggestions is nil")
	}
	if snap.CreditSummary == nil {
		t.Error("CreditSummary is nil")
	}
	if len(snap.WeeklyHistory) != 8 {
		t.Errorf("WeeklyHistory len = %d, want 8", len(snap.WeeklyHistory))
	}

	// Key invariant: GetAllPurchasesWithSales called exactly once
	if callCount != 1 {
		t.Errorf("GetAllPurchasesWithSales called %d times, want 1", callCount)
	}
}
