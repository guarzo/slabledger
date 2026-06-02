package scheduler

import "testing"

func TestGradeMatchesTitle(t *testing.T) {
	tests := []struct {
		name        string
		grader      string
		gradeValue  float64
		searchTitle string
		want        bool
	}{
		{"PSA 10 matches PSA 10", "PSA", 10, "Charizard 1999 Base Set Holo #4/102 PSA 10", true},
		{"PSA 7 rejects PSA 10 title", "PSA", 7, "Umbreon EX Gold Star PSA 10", false},
		{"PSA 10 rejects PSA 9 title", "PSA", 10, "Pikachu Promo PSA 9", false},
		{"PSA 9 matches PSA 9", "PSA", 9, "Charizard Base Set Holo PSA 9", true},
		{"PSA 8 rejects PSA 8.5 title", "PSA", 8, "Mew Promo PSA 8.5", false},
		{"BGS 9.5 matches BGS 9.5", "BGS", 9.5, "Lugia Neo Genesis Holo BGS 9.5", true},
		{"BGS 9.5 rejects BGS 9 title", "BGS", 9.5, "Lugia Neo Genesis BGS 9", false},
		{"empty grader defaults to PSA", "", 7, "Card PSA 7", true},
		{"empty grader PSA 7 rejects PSA 10", "", 7, "Card PSA 10", false},
		{"no grade token in title — reject (cannot verify)", "PSA", 7, "Card With No Grade Mentioned", false},
		{"PSA 10 in middle of title", "PSA", 10, "PSA 10 Charizard 1999", true},
		{"case insensitive", "psa", 9, "Charizard PSA 9", true},
		{"GradeValue 0 rejects PSA 10 title", "PSA", 0, "Charizard PSA 10", false},
		{"GradeValue 0 empty grader rejects PSA 10 title", "", 0, "Charizard PSA 10", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := gradeMatchesTitle(tc.grader, tc.gradeValue, tc.searchTitle)
			if got != tc.want {
				t.Errorf("gradeMatchesTitle(%q, %v, %q) = %v, want %v",
					tc.grader, tc.gradeValue, tc.searchTitle, got, tc.want)
			}
		})
	}
}
