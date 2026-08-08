package inventory

import (
	"testing"

	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// TestExtractGrade_AgreesWithExtractGraderAndGrade pins the decision made in SLA-43:
// every grader+grade regex derives from cardutil.GradePattern, so the PSA-only and
// multi-grader extractors must never disagree about what counts as a grade.
//
// Before SLA-43 the two regexes had drifted — ExtractGrade accepted a multi-digit
// fraction (\.\d+) while ExtractGraderAndGrade accepted only a single one (\.\d) —
// so "PSA 9.55" yielded 9.55 on the PSA CSV path and 9 on the Shopify path. The
// explicit decision is that both ACCEPT the multi-digit form and let the shared
// 1-10 range check be the thing that rejects nonsense.
func TestExtractGrade_AgreesWithExtractGraderAndGrade(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  float64
	}{
		{name: "whole grade", title: "2022 POKEMON CHARIZARD PSA 9", want: 9},
		{name: "top grade", title: "2022 POKEMON CHARIZARD PSA 10", want: 10},
		{name: "half grade", title: "2022 POKEMON CHARIZARD PSA 9.5", want: 9.5},
		{name: "no space before grade", title: "Charizard PSA9.5", want: 9.5},
		{name: "lowercase grader", title: "Charizard psa 8.5", want: 8.5},
		// The divergence SLA-43 closed. No real PSA/BGS/CGC/SGC scale emits two
		// fractional digits, so this input is malformed either way — what matters
		// is that both paths now read the whole number rather than one truncating
		// it to 9 and silently disagreeing with the other.
		{name: "multi-digit fraction is read in full", title: "Charizard PSA 9.55", want: 9.55},
		{name: "above range rejected", title: "Charizard PSA 11", want: 0},
		{name: "below range rejected", title: "Charizard PSA 0", want: 0},
		{name: "no grade at all", title: "Charizard Holo Rare", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGrade := ExtractGrade(tt.title)
			if gotGrade != tt.want {
				t.Errorf("ExtractGrade(%q) = %v, want %v", tt.title, gotGrade, tt.want)
			}

			gotGrader, gotPaired := ExtractGraderAndGrade(tt.title)
			if gotPaired != tt.want {
				t.Errorf("ExtractGraderAndGrade(%q) grade = %v, want %v (must agree with ExtractGrade)",
					tt.title, gotPaired, tt.want)
			}
			if tt.want != 0 && gotGrader != "PSA" {
				t.Errorf("ExtractGraderAndGrade(%q) grader = %q, want %q", tt.title, gotGrader, "PSA")
			}
		})
	}
}

// TestExtractGraderAndGrade_NonPSAGraders covers the graders ExtractGrade
// deliberately does not handle, so the shared grader fragment stays honest.
func TestExtractGraderAndGrade_NonPSAGraders(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		wantGrader string
		wantGrade  float64
	}{
		{name: "BGS half grade", title: "Charizard BGS 9.5", wantGrader: "BGS", wantGrade: 9.5},
		{name: "CGC whole grade", title: "Charizard CGC 10", wantGrader: "CGC", wantGrade: 10},
		{name: "SGC lowercase", title: "Charizard sgc 8", wantGrader: "SGC", wantGrade: 8},
		{name: "unknown grader ignored", title: "Charizard ACE 9", wantGrader: "", wantGrade: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGrader, gotGrade := ExtractGraderAndGrade(tt.title)
			if gotGrader != tt.wantGrader || gotGrade != tt.wantGrade {
				t.Errorf("ExtractGraderAndGrade(%q) = (%q, %v), want (%q, %v)",
					tt.title, gotGrader, gotGrade, tt.wantGrader, tt.wantGrade)
			}
			// ExtractGrade is PSA-only by contract.
			if got := ExtractGrade(tt.title); got != 0 {
				t.Errorf("ExtractGrade(%q) = %v, want 0 (PSA-only)", tt.title, got)
			}
		})
	}
}

// TestGradeStrippingUsesSharedPattern covers the two stripping call sites that
// derive from the same fragment. A truncated match here leaves a stray fractional
// tail in the card name, which is the visible cost of the pre-SLA-43 drift.
func TestGradeStrippingUsesSharedPattern(t *testing.T) {
	t.Run("ExtractCardNameFromTitle", func(t *testing.T) {
		tests := []struct {
			name  string
			title string
			want  string
		}{
			{name: "whole grade", title: "Charizard VMAX PSA 10", want: "Charizard VMAX"},
			{name: "half grade", title: "Charizard VMAX BGS 9.5", want: "Charizard VMAX"},
			{name: "multi-digit fraction leaves no tail", title: "Charizard VMAX PSA 9.55", want: "Charizard VMAX"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := ExtractCardNameFromTitle(tt.title); got != tt.want {
					t.Errorf("ExtractCardNameFromTitle(%q) = %q, want %q", tt.title, got, tt.want)
				}
			})
		}
	})

	t.Run("PSAGradeSuffixRegex", func(t *testing.T) {
		tests := []struct {
			name string
			in   string
			want string
		}{
			{name: "whole grade", in: "Charizard VMAX PSA 10", want: "Charizard VMAX"},
			{name: "half grade", in: "Charizard VMAX CGC 9.5", want: "Charizard VMAX"},
			{name: "multi-digit fraction leaves no tail", in: "Charizard VMAX PSA 9.55", want: "Charizard VMAX"},
			{name: "no suffix untouched", in: "Charizard VMAX", want: "Charizard VMAX"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := cardutil.PSAGradeSuffixRegex.ReplaceAllString(tt.in, ""); got != tt.want {
					t.Errorf("PSAGradeSuffixRegex strip of %q = %q, want %q", tt.in, got, tt.want)
				}
			})
		}
	})
}
