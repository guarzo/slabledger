package cardutil

import "testing"

func TestGradeFormRoundTrip(t *testing.T) {
	tests := []struct {
		gForm   string
		display string
	}{
		{"g8", "PSA 8"},
		{"g10", "PSA 10"},
		{"g8_5", "PSA 8.5"},
	}
	for _, tc := range tests {
		t.Run(tc.gForm, func(t *testing.T) {
			gotDisplay := ConditionToAPIFormat(tc.gForm)
			if gotDisplay != tc.display {
				t.Fatalf("ConditionToAPIFormat(%q) = %q, want %q", tc.gForm, gotDisplay, tc.display)
			}
			gotG := DisplayConditionToGFormat(gotDisplay)
			if gotG != tc.gForm {
				t.Fatalf("DisplayConditionToGFormat(%q) = %q, want %q", gotDisplay, gotG, tc.gForm)
			}
		})
	}
}
