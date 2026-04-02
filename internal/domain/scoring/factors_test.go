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

func TestComputeCreditPressure(t *testing.T) {
	tests := []struct {
		name           string
		utilizationPct float64
		wantValue      float64
	}{
		{"low utilization", 30.0, 0.0},
		{"moderate", 70.0, -0.3},
		{"high", 85.0, -0.6},
		{"critical", 96.0, -1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ComputeCreditPressure(tt.utilizationPct, 1.0, "test")
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
