package pricing

import "testing"

func TestNormalizeGrade(t *testing.T) {
	tests := []struct {
		input    string
		expected Grade
	}{
		// PSA 10 variants
		{"PSA 10", GradePSA10},
		{"psa10", GradePSA10},
		{"PSA-10", GradePSA10},
		{"10", GradePSA10},
		{"Gem Mint 10", GradePSA10},

		// PSA 9 variants
		{"PSA 9", GradePSA9},
		{"psa9", GradePSA9},
		{"PSA-9", GradePSA9},
		{"9", GradePSA9},

		// PSA 8 variants
		{"PSA 8", GradePSA8},
		{"psa8", GradePSA8},
		{"8", GradePSA8},

		// PSA 7 variants
		{"PSA 7", GradePSA7},
		{"psa7", GradePSA7},
		{"7", GradePSA7},

		// PSA 6 variants
		{"PSA 6", GradePSA6},
		{"psa6", GradePSA6},
		{"6", GradePSA6},

		// PSA 5 variants
		{"PSA 5", GradePSA5},
		{"psa5", GradePSA5},
		{"5", GradePSA5},

		// PSA 4 variants
		{"PSA 4", GradePSA4},
		{"psa4", GradePSA4},
		{"4", GradePSA4},

		// PSA 3 variants
		{"PSA 3", GradePSA3},
		{"psa3", GradePSA3},
		{"3", GradePSA3},

		// PSA 2 variants
		{"PSA 2", GradePSA2},
		{"psa2", GradePSA2},
		{"2", GradePSA2},

		// PSA 1 variants
		{"PSA 1", GradePSA1},
		{"psa1", GradePSA1},
		{"1", GradePSA1},

		// 9.5 / CGC / BGS 9.5
		{"BGS 9.5", GradePSA95},
		{"CGC 9.5", GradePSA95},
		{"9.5", GradePSA95},
		{"cgc95", GradePSA95},
		{"bgs95", GradePSA95},

		// BGS 10 Black Label
		{"BGS 10", GradeBGS10},
		{"bgs10", GradeBGS10},

		// Raw / ungraded
		{"", GradeRaw},
		{"raw", GradeRaw},
		{"ungraded", GradeRaw},
		{"nm", GradeRaw},
		{"Near Mint", GradeRaw},
		{"NM", GradeRaw},

		// Unknown
		{"something else", GradeUnknown},
		{"excellent", GradeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeGrade(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeGrade(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

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

func TestGradeFromDisplay(t *testing.T) {
	tests := []struct {
		display string
		want    Grade
	}{
		{"PSA 10", GradePSA10},
		{"PSA 9", GradePSA9},
		{"PSA 8", GradePSA8},
		{"Raw", GradeRaw},
		{"CGC 9.5", GradePSA95},
		{"BGS 9.5", GradePSA95},
		{"BGS 10", GradeBGS10},
		{"PSA 1", GradePSA1},
		{"Unknown", GradeUnknown},
		{"", GradeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.display, func(t *testing.T) {
			if got := GradeFromDisplay(tt.display); got != tt.want {
				t.Errorf("GradeFromDisplay(%q) = %q, want %q", tt.display, got, tt.want)
			}
		})
	}
}

func TestIsKnownDisplayGrade(t *testing.T) {
	// All AllDisplayGrades should be known
	for _, g := range AllDisplayGrades {
		label := g.DisplayLabel()
		if !IsKnownDisplayGrade(label) {
			t.Errorf("IsKnownDisplayGrade(%q) = false, want true", label)
		}
	}
	// Unknown should not be known
	if IsKnownDisplayGrade("Unknown") {
		t.Error("IsKnownDisplayGrade(\"Unknown\") = true, want false")
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
