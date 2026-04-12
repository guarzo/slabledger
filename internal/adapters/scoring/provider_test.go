package scoring

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- tests ---

func TestPurchaseData(t *testing.T) {
	tests := []struct {
		name      string
		arbSvc    *mocks.MockArbitrageService
		tuningSvc *mocks.MockTuningService
		portSvc   *mocks.MockPortfolioService
		req       advisor.PurchaseAssessmentRequest
		check     func(t *testing.T, data *advisor.PurchaseFactorData)
	}{
		{
			name: "full services",
			arbSvc: &mocks.MockArbitrageService{
				EvaluatePurchaseFn: func(_ context.Context, _ string, _ string, _ float64, _ int) (*arbitrage.ExpectedValue, error) {
					return &arbitrage.ExpectedValue{
						EVPerDollar:     0.35,
						LiquidityFactor: 1.2,
						TrendAdjustment: 0.05,
						Confidence:      "high",
					}, nil
				},
			},
			tuningSvc: &mocks.MockTuningService{
				GetCampaignTuningFn: func(_ context.Context, _ string) (*inventory.TuningResponse, error) {
					return &inventory.TuningResponse{
						ByGrade: []inventory.GradePerformance{
							{Grade: 10, ROI: 0.25, PurchaseCount: 10},
							{Grade: 9, ROI: 0.15, PurchaseCount: 5},
						},
						MarketAlignment: &inventory.MarketAlignment{AvgTrend30d: 0.08},
					}, nil
				},
			},
			portSvc: &mocks.MockPortfolioService{
				GetPortfolioInsightsFn: func(_ context.Context) (*inventory.PortfolioInsights, error) {
					return &inventory.PortfolioInsights{
						ByCharacter: []inventory.SegmentPerformance{
							{Label: "Charizard", PurchaseCount: 50},
							{Label: "Pikachu", PurchaseCount: 30},
							{Label: "Blastoise", PurchaseCount: 20},
						},
					}, nil
				},
			},
			req: advisor.PurchaseAssessmentRequest{
				CampaignID:   "camp-1",
				CardName:     "Charizard VMAX",
				Grade:        "PSA 10",
				BuyCostCents: 5000,
			},
			check: func(t *testing.T, data *advisor.PurchaseFactorData) {
				// ROI = EVPerDollar * 100 = 35
				assertPtrFloat(t, "ROIPct", data.ROIPct, 35.0)
				// SalesPerMonth = LiquidityFactor * 5 = 6
				assertPtrFloat(t, "SalesPerMonth", data.SalesPerMonth, 6.0)
				// PriceChangePct = TrendAdjustment * 100 = 5
				assertPtrFloat(t, "PriceChangePct", data.PriceChangePct, 5.0)
				// PriceConfidence = high -> 1.0
				if data.PriceConfidence != 1.0 {
					t.Errorf("PriceConfidence: got %v, want 1.0", data.PriceConfidence)
				}
				// GradeROI for PSA 10 = 0.25 * 100 = 25
				assertPtrFloat(t, "GradeROI", data.GradeROI, 25.0)
				// CampaignAvgROI = weighted avg = (0.25*10 + 0.15*5) / 15 * 100 = 21.666...
				if data.CampaignAvgROI == nil {
					t.Fatal("CampaignAvgROI is nil")
				}
				if *data.CampaignAvgROI < 21.6 || *data.CampaignAvgROI > 21.7 {
					t.Errorf("CampaignAvgROI: got %v, want ~21.67", *data.CampaignAvgROI)
				}
				// Trend30dPct = 0.08 * 100 = 8
				assertPtrFloat(t, "Trend30dPct", data.Trend30dPct, 8.0)
				// Charizard has 50 of 100 total = 50% > 40% -> "high"
				if data.ConcentrationRisk != "high" {
					t.Errorf("ConcentrationRisk: got %q, want %q", data.ConcentrationRisk, "high")
				}
			},
		},
		{
			name: "graceful degradation — no optional services",
			req: advisor.PurchaseAssessmentRequest{
				CampaignID:   "camp-1",
				CardName:     "Pikachu V",
				Grade:        "PSA 9",
				BuyCostCents: 1000,
			},
			check: func(t *testing.T, data *advisor.PurchaseFactorData) {
				// No arbSvc injected → ROIPct is nil
				if data.ROIPct != nil {
					t.Error("ROIPct should be nil when no arbSvc injected")
				}
				// No tuningSvc injected → GradeROI nil
				if data.GradeROI != nil {
					t.Error("GradeROI should be nil with no tuningSvc injected")
				}
				// No portSvc injected → ConcentrationRisk is ""
				if data.ConcentrationRisk != "" {
					t.Errorf("ConcentrationRisk: got %q, want empty string (no portSvc)", data.ConcentrationRisk)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockInventoryService{}
			opts := []ProviderOption{}
			if tt.arbSvc != nil {
				opts = append(opts, WithArbitrageService(tt.arbSvc))
			}
			if tt.tuningSvc != nil {
				opts = append(opts, WithTuningService(tt.tuningSvc))
			}
			if tt.portSvc != nil {
				opts = append(opts, WithPortfolioService(tt.portSvc))
			}
			p := NewProvider(svc, opts...)
			data, err := p.PurchaseData(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, data)
		})
	}
}

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
				// ROI = 0.22 * 100 = 22
				assertPtrFloat(t, "ROIPct", data.ROIPct, 22.0)
				// SellThrough = 0.65
				assertPtrFloat(t, "SellThroughPct", data.SellThroughPct, 0.65)
				// PriceConfidence = 30 purchases -> 1.0 (>=20)
				if data.PriceConfidence != 1.0 {
					t.Errorf("PriceConfidence: got %v, want 1.0", data.PriceConfidence)
				}
				// Trend30dPct = -0.03 * 100 = -3
				assertPtrFloat(t, "Trend30dPct", data.Trend30dPct, -3.0)
				// SalesPerMonth = avg(12, 8) = 10
				assertPtrFloat(t, "SalesPerMonth", data.SalesPerMonth, 10.0)
				// PriceChangePct = avg(0.10*100, -0.05*100) = avg(10, -5) = 2.5
				assertPtrFloat(t, "PriceChangePct", data.PriceChangePct, 2.5)
				// CampaignROI = same as ROIPct = 22
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

func TestStubMethods_ReturnNil(t *testing.T) {
	p := NewProvider(&mocks.MockInventoryService{})

	t.Run("LiquidationData", func(t *testing.T) {
		data, err := p.LiquidationData(context.Background(), "p-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if data != nil {
			t.Error("expected nil, got non-nil LiquidationFactorData")
		}
	})

	t.Run("SuggestionData", func(t *testing.T) {
		data, err := p.SuggestionData(context.Background(), "segment-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if data != nil {
			t.Error("expected nil, got non-nil SuggestionFactorData")
		}
	})
}

func TestParseGrade(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"PSA 10", 10.0},
		{"PSA 9.5", 9.5},
		{"BGS 8", 8.0},
		{"10", 10.0},
		{"", 0},
		// Edge cases
		{"PSA NM-MT", 0},          // no numeric suffix returns 0
		{"PSA 8 abc", 8},          // rightmost numeric wins (abc fails, 8 is second from right)
		{"   9   ", 9},            // whitespace-padded grade
		{"PSA 8.5", 8.5},          // half-grade extracted
		{"BGS 9.5 Q", 9.5},        // half-grade with trailing non-numeric token
		{"CGC 10.0 Pristine", 10}, // grade with trailing label
	}
	for _, tt := range tests {
		got := parseGrade(tt.input)
		if got != tt.want {
			t.Errorf("parseGrade(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestComputeConcentration(t *testing.T) {
	segments := []inventory.SegmentPerformance{
		{Label: "Charizard", PurchaseCount: 50},
		{Label: "Pikachu", PurchaseCount: 30},
		{Label: "Blastoise", PurchaseCount: 20},
	}

	tests := []struct {
		cardName string
		want     string
	}{
		{"Charizard VMAX", "high"}, // 50/100 = 50% > 40%
		{"Pikachu V", "medium"},    // 30/100 = 30%
		{"Mewtwo GX", "low"},       // 0/100 = 0% < 15%
		{"Blastoise EX", "medium"}, // 20/100 = 20%
	}

	for _, tt := range tests {
		got := computeConcentration(segments, tt.cardName)
		if got != tt.want {
			t.Errorf("computeConcentration(%q) = %q, want %q", tt.cardName, got, tt.want)
		}
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
