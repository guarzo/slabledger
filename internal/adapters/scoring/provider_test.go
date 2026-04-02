package scoring

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestPurchaseData(t *testing.T) {
	svc := &mocks.MockCampaignService{
		EvaluatePurchaseFn: func(_ context.Context, _ string, _ string, _ float64, _ int) (*campaigns.ExpectedValue, error) {
			return &campaigns.ExpectedValue{
				EVPerDollar:     0.35,
				LiquidityFactor: 1.2,
				TrendAdjustment: 0.05,
				Confidence:      "high",
			}, nil
		},
		GetCampaignTuningFn: func(_ context.Context, _ string) (*campaigns.TuningResponse, error) {
			return &campaigns.TuningResponse{
				ByGrade: []campaigns.GradePerformance{
					{Grade: 10, ROI: 0.25, PurchaseCount: 10},
					{Grade: 9, ROI: 0.15, PurchaseCount: 5},
				},
				MarketAlignment: &campaigns.MarketAlignment{
					AvgTrend30d: 0.08,
				},
			}, nil
		},
		GetPortfolioInsightsFn: func(_ context.Context) (*campaigns.PortfolioInsights, error) {
			return &campaigns.PortfolioInsights{
				ByCharacter: []campaigns.SegmentPerformance{
					{Label: "Charizard", PurchaseCount: 50},
					{Label: "Pikachu", PurchaseCount: 30},
					{Label: "Blastoise", PurchaseCount: 20},
				},
			}, nil
		},
	}

	p := NewProvider(svc)
	req := advisor.PurchaseAssessmentRequest{
		CampaignID:   "camp-1",
		CardName:     "Charizard VMAX",
		Grade:        "PSA 10",
		BuyCostCents: 5000,
	}

	data, err := p.PurchaseData(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
}

func TestPurchaseData_GracefulDegradation(t *testing.T) {
	// All service methods return defaults (nil pointers / empty slices).
	svc := &mocks.MockCampaignService{}
	p := NewProvider(svc)

	req := advisor.PurchaseAssessmentRequest{
		CampaignID:   "camp-1",
		CardName:     "Pikachu V",
		Grade:        "PSA 9",
		BuyCostCents: 1000,
	}

	data, err := p.PurchaseData(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With empty defaults the EvaluatePurchase returns a zero ExpectedValue,
	// so ROIPct and SalesPerMonth will be set to 0 (pointer to 0).
	if data.ROIPct == nil {
		t.Error("ROIPct should not be nil even with zero EV")
	}
	// Tuning returns empty ByGrade -> GradeROI nil
	if data.GradeROI != nil {
		t.Error("GradeROI should be nil with no grade data")
	}
	// Concentration with empty segments -> "low"
	if data.ConcentrationRisk != "low" {
		t.Errorf("ConcentrationRisk: got %q, want %q", data.ConcentrationRisk, "low")
	}
}

func TestCampaignData(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignPNLFn: func(_ context.Context, _ string) (*campaigns.CampaignPNL, error) {
			return &campaigns.CampaignPNL{
				ROI:            0.22,
				SellThroughPct: 0.65,
				TotalPurchases: 30,
			}, nil
		},
		GetCampaignTuningFn: func(_ context.Context, _ string) (*campaigns.TuningResponse, error) {
			return &campaigns.TuningResponse{
				MarketAlignment: &campaigns.MarketAlignment{
					AvgTrend30d: -0.03,
				},
			}, nil
		},
		GetInventoryAgingFn: func(_ context.Context, _ string) ([]campaigns.AgingItem, error) {
			return []campaigns.AgingItem{
				{
					Signal: &campaigns.MarketSignal{DeltaPct: 0.10},
					CurrentMarket: &campaigns.MarketSnapshot{
						MonthlyVelocity: 12,
					},
				},
				{
					Signal: &campaigns.MarketSignal{DeltaPct: -0.05},
					CurrentMarket: &campaigns.MarketSnapshot{
						MonthlyVelocity: 8,
					},
				},
			}, nil
		},
	}

	p := NewProvider(svc)
	data, err := p.CampaignData(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
}

func TestStubMethods_ReturnNil(t *testing.T) {
	p := NewProvider(&mocks.MockCampaignService{})

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
	}
	for _, tt := range tests {
		got := parseGrade(tt.input)
		if got != tt.want {
			t.Errorf("parseGrade(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestComputeConcentration(t *testing.T) {
	segments := []campaigns.SegmentPerformance{
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
