package insights

import "testing"

func TestMapParameterToColumn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		param string
		want  string // "" means no column (drop)
	}{
		{"buy threshold", "buyTermsCLPct", "buyPct"},
		{"grade range maps to characters bucket (v1 approximation)", "gradeRange", "characters"},
		{"phase closes become kill", "phase", "buyPct"},
		{"daily spend cap", "dailySpendCap", "spendCap"},
		{"price range — no v1 column", "priceRange", ""},
		{"channel — no v1 column", "saleChannel", ""},
		{"unknown — no v1 column", "somethingElse", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapParameterToColumn(tc.param)
			if got != tc.want {
				t.Errorf("MapParameterToColumn(%q) = %q, want %q", tc.param, got, tc.want)
			}
		})
	}
}

func TestDeriveCellSeverity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		confidence int
		want       Severity
	}{
		{"high confidence = act", 20, SeverityAct},
		{"medium confidence = tune", 8, SeverityTune},
		{"low confidence = tune", 3, SeverityTune},
		{"zero confidence = tune", 0, SeverityTune},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveCellSeverity(tc.confidence)
			if got != tc.want {
				t.Errorf("DeriveCellSeverity(%d) = %q, want %q", tc.confidence, got, tc.want)
			}
		})
	}
}

func TestDeriveRowStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		cells map[string]TuningCell
		want  Status
	}{
		{"empty row = OK", map[string]TuningCell{}, StatusOK},
		{"any tune cell = Tune", map[string]TuningCell{"buyPct": {Severity: SeverityTune}}, StatusTune},
		{"any act cell = Act", map[string]TuningCell{"buyPct": {Severity: SeverityAct}}, StatusAct},
		{"act beats tune", map[string]TuningCell{"buyPct": {Severity: SeverityAct}, "years": {Severity: SeverityTune}}, StatusAct},
		{"retire recommendation = Kill", map[string]TuningCell{"buyPct": {Severity: SeverityAct, Recommendation: "Retire campaign (-12% ROI, 90d)"}}, StatusKill},
		{"close recommendation = Kill", map[string]TuningCell{"buyPct": {Severity: SeverityAct, Recommendation: "Close campaign"}}, StatusKill},
		{"retire lowercase still kills", map[string]TuningCell{"buyPct": {Severity: SeverityAct, Recommendation: "retire this campaign"}}, StatusKill},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveRowStatus(tc.cells)
			if got != tc.want {
				t.Errorf("DeriveRowStatus = %q, want %q", got, tc.want)
			}
		})
	}
}
