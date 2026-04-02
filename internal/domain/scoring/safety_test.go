package scoring

import "testing"

func TestApplySafetyFilters(t *testing.T) {
	tests := []struct {
		name         string
		confidence   float64
		verdict      Verdict
		mixedSignals bool
		wantVerdict  Verdict
		wantOneOf    []Verdict // if set, result must be one of these
	}{
		{"low confidence strong buy clamped", 0.25, VerdictStrongBuy, false, VerdictLeanBuy, nil},
		{"low confidence buy clamped", 0.25, VerdictBuy, false, VerdictLeanBuy, nil},
		{"low confidence lean buy ok", 0.25, VerdictLeanBuy, false, VerdictLeanBuy, nil},
		{"low confidence hold ok", 0.25, VerdictHold, false, VerdictHold, nil},
		{"low confidence strong sell clamped", 0.25, VerdictStrongSell, false, VerdictLeanSell, nil},
		{"medium confidence buy ok", 0.4, VerdictBuy, false, VerdictBuy, nil},
		{"medium confidence strong buy clamped", 0.4, VerdictStrongBuy, false, VerdictBuy, nil},
		{"high confidence strong buy ok", 0.6, VerdictStrongBuy, false, VerdictStrongBuy, nil},
		{"mixed signals clamps buy", 0.8, VerdictBuy, true, "", []Verdict{VerdictLeanBuy, VerdictHold, VerdictLeanSell}},
		{"no change when safe", 0.75, VerdictBuy, false, VerdictBuy, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := ScoreCard{Confidence: tt.confidence, Verdict: tt.verdict, MixedSignals: tt.mixedSignals}
			result := ApplySafetyFilters(sc)
			if tt.wantOneOf != nil {
				found := false
				for _, v := range tt.wantOneOf {
					if result.Verdict == v {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Verdict = %s, want one of %v", result.Verdict, tt.wantOneOf)
				}
			} else if result.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %s, want %s", result.Verdict, tt.wantVerdict)
			}
		})
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
