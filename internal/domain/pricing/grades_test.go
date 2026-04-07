package pricing

import "testing"

func TestGradePredicates(t *testing.T) {
	tests := []struct {
		grade Grade
		psa10 bool
		psa9  bool
		psa8  bool
		isRaw bool
	}{
		{GradePSA10, true, false, false, false},
		{GradePSA9, false, true, false, false},
		{GradePSA8, false, false, true, false},
		{GradeRaw, false, false, false, true},
		{GradePSA95, false, false, false, false},
		{GradeBGS10, false, false, false, false},
		{GradeUnknown, false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.grade), func(t *testing.T) {
			if got := tt.grade.IsPSA10(); got != tt.psa10 {
				t.Errorf("Grade(%q).IsPSA10() = %v, want %v", tt.grade, got, tt.psa10)
			}
			if got := tt.grade.IsPSA9(); got != tt.psa9 {
				t.Errorf("Grade(%q).IsPSA9() = %v, want %v", tt.grade, got, tt.psa9)
			}
			if got := tt.grade.IsPSA8(); got != tt.psa8 {
				t.Errorf("Grade(%q).IsPSA8() = %v, want %v", tt.grade, got, tt.psa8)
			}
			if got := tt.grade.IsRaw(); got != tt.isRaw {
				t.Errorf("Grade(%q).IsRaw() = %v, want %v", tt.grade, got, tt.isRaw)
			}
		})
	}
}

func TestGradeString(t *testing.T) {
	if got := GradePSA10.String(); got != "psa10" {
		t.Errorf("GradePSA10.String() = %q, want %q", got, "psa10")
	}
	if got := GradeRaw.String(); got != "raw" {
		t.Errorf("GradeRaw.String() = %q, want %q", got, "raw")
	}
}

func TestDisplayLabel(t *testing.T) {
	tests := []struct {
		grade Grade
		want  string
	}{
		{GradePSA10, "PSA 10"},
		{GradePSA9, "PSA 9"},
		{GradePSA8, "PSA 8"},
		{GradePSA7, "PSA 7"},
		{GradePSA6, "PSA 6"},
		{GradePSA5, "PSA 5"},
		{GradePSA1, "PSA 1"},
		{GradeRaw, "Raw"},
		{GradePSA95, "CGC 9.5"},
		{GradeBGS10, "BGS 10"},
		{GradeUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.grade.DisplayLabel(); got != tt.want {
				t.Errorf("Grade(%q).DisplayLabel() = %q, want %q", tt.grade, got, tt.want)
			}
		})
	}
}

func TestSetGetGradePrice(t *testing.T) {
	var grades GradedPrices
	SetGradePrice(&grades, GradePSA10, 1000)
	SetGradePrice(&grades, GradePSA9, 500)
	SetGradePrice(&grades, GradePSA8, 300)
	SetGradePrice(&grades, GradeRaw, 100)
	SetGradePrice(&grades, GradePSA95, 750)
	SetGradePrice(&grades, GradeBGS10, 2000)

	if got := GetGradePrice(grades, GradePSA10); got != 1000 {
		t.Errorf("GetGradePrice(PSA10) = %d, want 1000", got)
	}
	if got := GetGradePrice(grades, GradePSA9); got != 500 {
		t.Errorf("GetGradePrice(PSA9) = %d, want 500", got)
	}
	if got := GetGradePrice(grades, GradePSA8); got != 300 {
		t.Errorf("GetGradePrice(PSA8) = %d, want 300", got)
	}
	if got := GetGradePrice(grades, GradeRaw); got != 100 {
		t.Errorf("GetGradePrice(Raw) = %d, want 100", got)
	}
	if got := GetGradePrice(grades, GradeUnknown); got != 0 {
		t.Errorf("GetGradePrice(Unknown) = %d, want 0", got)
	}

	SetGradePrice(&grades, GradePSA7, 200)
	SetGradePrice(&grades, GradePSA6, 150)

	if got := GetGradePrice(grades, GradePSA7); got != 200 {
		t.Errorf("GetGradePrice(PSA7) = %d, want 200", got)
	}
	if got := GetGradePrice(grades, GradePSA6); got != 150 {
		t.Errorf("GetGradePrice(PSA6) = %d, want 150", got)
	}
}

func TestCoreGradesAreSubsetOfAll(t *testing.T) {
	allSet := make(map[Grade]bool)
	for _, g := range AllDisplayGrades {
		allSet[g] = true
	}
	for _, g := range CoreGrades {
		if !allSet[g] {
			t.Errorf("CoreGrade %q not in AllDisplayGrades", g)
		}
	}
}
