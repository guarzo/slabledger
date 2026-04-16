package scoring

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestCampaignData(t *testing.T) {
	tests := []struct {
		name       string
		tuningSvc  *mocks.MockTuningService
		invSvc     *mocks.MockInventoryService
		campaignID string
		check      func(t *testing.T, data *advisor.CampaignFactorData)
	}{
		{
			name: "with tuning service",
			invSvc: &mocks.MockInventoryService{
				GetCampaignPNLFn: func(_ context.Context, _ string) (*inventory.CampaignPNL, error) {
					return &inventory.CampaignPNL{
						ROI:            0.22,
						SellThroughPct: 0.65,
						TotalPurchases: 30,
					}, nil
				},
				GetInventoryAgingFn: func(_ context.Context, _ string) (*inventory.InventoryResult, error) {
					return &inventory.InventoryResult{Items: []inventory.AgingItem{
						{
							Signal:        &inventory.MarketSignal{DeltaPct: 0.10},
							CurrentMarket: &inventory.MarketSnapshot{MonthlyVelocity: 12},
						},
						{
							Signal:        &inventory.MarketSignal{DeltaPct: -0.05},
							CurrentMarket: &inventory.MarketSnapshot{MonthlyVelocity: 8},
						},
					}}, nil
				},
			},
			tuningSvc: &mocks.MockTuningService{
				GetCampaignTuningFn: func(_ context.Context, _ string) (*inventory.TuningResponse, error) {
					return &inventory.TuningResponse{
						MarketAlignment: &inventory.MarketAlignment{AvgTrend30d: -0.03},
					}, nil
				},
			},
			campaignID: "camp-1",
			check: func(t *testing.T, data *advisor.CampaignFactorData) {
				assertPtrFloat(t, "ROIPct", data.ROIPct, 22.0)
				assertPtrFloat(t, "SellThroughPct", data.SellThroughPct, 0.65)
				if data.PriceConfidence != 1.0 {
					t.Errorf("PriceConfidence: got %v, want 1.0", data.PriceConfidence)
				}
				assertPtrFloat(t, "Trend30dPct", data.Trend30dPct, -3.0)
				assertPtrFloat(t, "SalesPerMonth", data.SalesPerMonth, 10.0)
				assertPtrFloat(t, "PriceChangePct", data.PriceChangePct, 2.5)
				assertPtrFloat(t, "CampaignROI", data.CampaignROI, 22.0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []ProviderOption{}
			if tt.tuningSvc != nil {
				opts = append(opts, WithTuningService(tt.tuningSvc))
			}
			p := NewProvider(tt.invSvc, opts...)
			data, err := p.CampaignData(context.Background(), tt.campaignID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, data)
		})
	}
}

func assertPtrFloat(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: got nil, want %v", name, want)
	}
	if diff := *got - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("%s: got %v, want %v", name, *got, want)
	}
}
