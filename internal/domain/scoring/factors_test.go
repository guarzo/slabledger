package scoring

import (
	"math"
	"testing"
)

func assertFactor(t *testing.T, f Factor, wantName string, wantMin, wantMax float64) {
	t.Helper()
	if f.Name != wantName {
		t.Errorf("Name = %s, want %s", f.Name, wantName)
	}
	if f.Value < wantMin || f.Value > wantMax {
		t.Errorf("%s Value = %f, want [%f, %f]", wantName, f.Value, wantMin, wantMax)
	}
}

func TestComputeMarketTrend(t *testing.T) {
	tests := []struct {
		name           string
		priceChangePct float64
		wantValue      float64
	}{
		{"strong positive", 20.0, 1.0},
		{"moderate positive", 10.0, 0.5},
		{"flat", 0.0, 0.0},
		{"moderate negative", -10.0, -0.5},
		{"clamped at -1", -30.0, -1.0},
		{"clamped at +1", 40.0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeMarketTrend(tt.priceChangePct, 0.9, "test")
			if math.Abs(f.Value-tt.wantValue) > 0.01 {
				t.Errorf("Value = %f, want %f", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeLiquidity(t *testing.T) {
	tests := []struct {
		name    string
		sales   float64
		wantMin float64
		wantMax float64
	}{
		{"high velocity", 15.0, 0.9, 1.0},
		{"good velocity", 10.0, 0.9, 1.0},
		{"moderate", 5.0, 0.4, 0.6},
		{"low", 2.0, -0.1, 0.1},
		{"very low", 0.5, -0.6, -0.4},
		{"none", 0.0, -1.0, -1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeLiquidity(tt.sales, 1.0, "test")
			assertFactor(t, f, FactorLiquidity, tt.wantMin, tt.wantMax)
		})
	}
}

func TestComputeROIPotential(t *testing.T) {
	tests := []struct {
		name      string
		roiPct    float64
		wantValue float64
	}{
		{"strong positive", 50.0, 1.0},
		{"moderate", 25.0, 0.5},
		{"breakeven", 0.0, 0.0},
		{"loss", -25.0, -0.5},
		{"clamped", -60.0, -1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeROIPotential(tt.roiPct, 1.0, "test")
			if math.Abs(f.Value-tt.wantValue) > 0.01 {
				t.Errorf("Value = %f, want %f", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeScarcity(t *testing.T) {
	tests := []struct {
		name      string
		psa10Pop  int
		wantValue float64
	}{
		{"extremely rare", 100, 0.8},
		{"rare", 500, 0.6},
		{"uncommon", 1500, 0.4},
		{"moderate", 3000, 0.2},
		{"common", 6000, 0.0},
		{"high pop", 8000, -0.15},
		{"very high pop", 15000, -0.3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeScarcity(tt.psa10Pop, 1.0, "test")
			if math.Abs(f.Value-tt.wantValue) > 0.01 {
				t.Errorf("Value = %f, want %f", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeMarketAlignment(t *testing.T) {
	tests := []struct {
		name      string
		trend30d  float64
		wantValue float64
	}{
		{"strong up", 10.0, 1.0},
		{"moderate up", 5.0, 0.5},
		{"flat", 0.0, 0.0},
		{"moderate down", -5.0, -0.5},
		{"clamped down", -15.0, -1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeMarketAlignment(tt.trend30d, 1.0, "test")
			if math.Abs(f.Value-tt.wantValue) > 0.01 {
				t.Errorf("Value = %f, want %f", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputePortfolioFit(t *testing.T) {
	tests := []struct {
		name      string
		concRisk  string
		wantValue float64
	}{
		{"low risk", "low", 0.8},
		{"medium risk", "medium", 0.0},
		{"high risk", "high", -0.8},
		{"unknown defaults to medium", "unknown", 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputePortfolioFit(tt.concRisk, 1.0, "test")
			if math.Abs(f.Value-tt.wantValue) > 0.01 {
				t.Errorf("Value = %f, want %f", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeCapitalPressure(t *testing.T) {
	tests := []struct {
		name         string
		weeksToCover float64
		wantValue    float64
	}{
		{"low exposure", 5.0, 0.0},
		{"moderate", 8.0, -0.3},
		{"high", 15.0, -0.6},
		{"critical", 22.0, -1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeCapitalPressure(tt.weeksToCover, 1.0, "test")
			if math.Abs(f.Value-tt.wantValue) > 0.01 {
				t.Errorf("Value = %f, want %f", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeCarryingCost(t *testing.T) {
	tests := []struct {
		name    string
		days    int
		wantMin float64
		wantMax float64
	}{
		{"fresh", 0, -0.01, 0.01},
		{"90 days", 90, 0.45, 0.55},
		{"180 days", 180, 0.95, 1.0},
		{"capped at 1", 365, 0.95, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeCarryingCost(tt.days, 1.0, "test")
			assertFactor(t, f, FactorCarryingCost, tt.wantMin, tt.wantMax)
		})
	}
}

func TestComputeSellThrough(t *testing.T) {
	tests := []struct {
		name      string
		pct       float64
		wantValue float64
	}{
		{"100%", 100.0, 1.0},
		{"50%", 50.0, 0.0},
		{"0%", 0.0, -1.0},
		{"75%", 75.0, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeSellThrough(tt.pct, 1.0, "test")
			if math.Abs(f.Value-tt.wantValue) > 0.01 {
				t.Errorf("Value = %f, want %f", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeGradeFit(t *testing.T) {
	tests := []struct {
		name           string
		gradeROI       float64
		campaignAvgROI float64
		confidence     float64
		source         string
		wantValue      float64
		wantFactor     string
	}{
		{
			name:     "grade outperforms campaign by 30 pct — value 1.0",
			gradeROI: 30.0, campaignAvgROI: 0.0, confidence: 0.8, source: "test",
			wantValue: 1.0, wantFactor: FactorGradeFit,
		},
		{
			name:     "grade underperforms campaign by 30 pct — value -1.0",
			gradeROI: 0.0, campaignAvgROI: 30.0, confidence: 0.8, source: "test",
			wantValue: -1.0, wantFactor: FactorGradeFit,
		},
		{
			name:     "equal roi — value 0.0",
			gradeROI: 15.0, campaignAvgROI: 15.0, confidence: 1.0, source: "test",
			wantValue: 0.0, wantFactor: FactorGradeFit,
		},
		{
			name:     "clamped positive — outperforms by 60 pct caps at 1.0",
			gradeROI: 60.0, campaignAvgROI: 0.0, confidence: 0.5, source: "test",
			wantValue: 1.0, wantFactor: FactorGradeFit,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeGradeFit(tt.gradeROI, tt.campaignAvgROI, tt.confidence, tt.source)
			if f.Name != tt.wantFactor {
				t.Errorf("Name = %q, want %q", f.Name, tt.wantFactor)
			}
			if f.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeCrackAdvantage(t *testing.T) {
	tests := []struct {
		name       string
		crackROI   float64
		gradedROI  float64
		confidence float64
		source     string
		wantValue  float64
		wantFactor string
	}{
		{
			name:     "crack significantly better — clamps to 1.0",
			crackROI: 100.0, gradedROI: 0.0, confidence: 0.9, source: "test",
			wantValue: 1.0, wantFactor: FactorCrackAdvantage,
		},
		{
			name:     "crack significantly worse — clamps to -1.0",
			crackROI: 0.0, gradedROI: 100.0, confidence: 0.9, source: "test",
			wantValue: -1.0, wantFactor: FactorCrackAdvantage,
		},
		{
			name:     "equal — value 0.0",
			crackROI: 50.0, gradedROI: 50.0, confidence: 0.5, source: "test",
			wantValue: 0.0, wantFactor: FactorCrackAdvantage,
		},
		{
			name:     "crack better by 25 — value 0.5",
			crackROI: 75.0, gradedROI: 50.0, confidence: 0.7, source: "test",
			wantValue: 0.5, wantFactor: FactorCrackAdvantage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeCrackAdvantage(tt.crackROI, tt.gradedROI, tt.confidence, tt.source)
			if f.Name != tt.wantFactor {
				t.Errorf("Name = %q, want %q", f.Name, tt.wantFactor)
			}
			if f.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeSpendEfficiency(t *testing.T) {
	tests := []struct {
		name        string
		fillRatePct float64
		roiPct      float64
		confidence  float64
		source      string
		wantValue   float64
		wantFactor  string
	}{
		{
			name:        "low fill rate — negative signal",
			fillRatePct: 10.0, roiPct: 5.0, confidence: 0.8, source: "test",
			wantValue: -0.6, wantFactor: FactorSpendEfficiency,
		},
		{
			name:        "high fill rate and good roi — positive signal",
			fillRatePct: 97.0, roiPct: 15.0, confidence: 0.9, source: "test",
			wantValue: 0.6, wantFactor: FactorSpendEfficiency,
		},
		{
			name:        "mid-range fill rate — moderate signal",
			fillRatePct: 70.0, roiPct: 5.0, confidence: 0.7, source: "test",
			wantValue: 0.3, wantFactor: FactorSpendEfficiency,
		},
		{
			name:        "default case — neutral",
			fillRatePct: 50.0, roiPct: 5.0, confidence: 0.5, source: "test",
			wantValue: 0.0, wantFactor: FactorSpendEfficiency,
		},
		{
			name:        "high fill rate but low roi — not positive",
			fillRatePct: 97.0, roiPct: 5.0, confidence: 0.5, source: "test",
			wantValue: 0.0, wantFactor: FactorSpendEfficiency,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeSpendEfficiency(tt.fillRatePct, tt.roiPct, tt.confidence, tt.source)
			if f.Name != tt.wantFactor {
				t.Errorf("Name = %q, want %q", f.Name, tt.wantFactor)
			}
			if f.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", f.Value, tt.wantValue)
			}
		})
	}
}

func TestComputeCoverageImpact(t *testing.T) {
	tests := []struct {
		name         string
		fillsGap     bool
		overlapCount int
		confidence   float64
		source       string
		wantValue    float64
		wantFactor   string
	}{
		{
			name:     "fills a gap — strong positive signal",
			fillsGap: true, overlapCount: 0, confidence: 0.9, source: "test",
			wantValue: 0.8, wantFactor: FactorCoverageImpact,
		},
		{
			name:     "overlaps existing — negative signal",
			fillsGap: false, overlapCount: 3, confidence: 0.7, source: "test",
			wantValue: -0.3, wantFactor: FactorCoverageImpact,
		},
		{
			name:     "no gap and no overlap — neutral",
			fillsGap: false, overlapCount: 0, confidence: 0.5, source: "test",
			wantValue: 0.0, wantFactor: FactorCoverageImpact,
		},
		{
			name:     "fills gap takes precedence over overlap",
			fillsGap: true, overlapCount: 5, confidence: 0.6, source: "test",
			wantValue: 0.8, wantFactor: FactorCoverageImpact,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeCoverageImpact(tt.fillsGap, tt.overlapCount, tt.confidence, tt.source)
			if f.Name != tt.wantFactor {
				t.Errorf("Name = %q, want %q", f.Name, tt.wantFactor)
			}
			if f.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", f.Value, tt.wantValue)
			}
		})
	}
}
