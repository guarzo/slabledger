package scoring

import "testing"

func TestApplySafetyFilters_ConfidenceClamping(t *testing.T) {
	tests := []struct {
		name        string
		confidence  float64
		verdict     Verdict
		wantVerdict Verdict
	}{
		{"low confidence strong buy clamped", 0.25, VerdictStrongBuy, VerdictLeanBuy},
		{"low confidence buy clamped", 0.25, VerdictBuy, VerdictLeanBuy},
		{"low confidence lean buy ok", 0.25, VerdictLeanBuy, VerdictLeanBuy},
		{"low confidence hold ok", 0.25, VerdictHold, VerdictHold},
		{"low confidence strong sell clamped", 0.25, VerdictStrongSell, VerdictLeanSell},
		{"medium confidence buy ok", 0.4, VerdictBuy, VerdictBuy},
		{"medium confidence strong buy clamped", 0.4, VerdictStrongBuy, VerdictBuy},
		{"high confidence strong buy ok", 0.6, VerdictStrongBuy, VerdictStrongBuy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := ScoreCard{Confidence: tt.confidence, Verdict: tt.verdict}
			result := ApplySafetyFilters(sc)
			if result.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %s, want %s", result.Verdict, tt.wantVerdict)
			}
		})
	}
}

func TestApplySafetyFilters_MixedSignals(t *testing.T) {
	sc := ScoreCard{Confidence: 0.8, Verdict: VerdictBuy, MixedSignals: true}
	result := ApplySafetyFilters(sc)
	allowed := map[Verdict]bool{VerdictLeanBuy: true, VerdictHold: true, VerdictLeanSell: true}
	if !allowed[result.Verdict] {
		t.Errorf("mixed signals: Verdict = %s, want lean_buy/hold/lean_sell", result.Verdict)
	}
}

func TestApplySafetyFilters_NoChangeWhenSafe(t *testing.T) {
	sc := ScoreCard{Confidence: 0.75, Verdict: VerdictBuy, MixedSignals: false}
	result := ApplySafetyFilters(sc)
	if result.Verdict != VerdictBuy {
		t.Errorf("Verdict = %s, want buy", result.Verdict)
	}
}

func TestValidateVerdictAdjustment(t *testing.T) {
	tests := []struct {
		name   string
		engine Verdict
		llm    Verdict
		valid  bool
	}{
		{"same", VerdictBuy, VerdictBuy, true},
		{"one step up", VerdictBuy, VerdictStrongBuy, true},
		{"one step down", VerdictBuy, VerdictLeanBuy, true},
		{"two steps", VerdictBuy, VerdictHold, false},
		{"three steps", VerdictBuy, VerdictLeanSell, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateVerdictAdjustment(tt.engine, tt.llm); got != tt.valid {
				t.Errorf("ValidateVerdictAdjustment(%s, %s) = %v, want %v", tt.engine, tt.llm, got, tt.valid)
			}
		})
	}
}
