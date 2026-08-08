package inventory

import "testing"

func TestParseCLConfidenceMin(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"2.5-4", 2, true},
		{"3-5", 3, true},
		{"4", 4, true},
		{"2.9", 2, true},
		{"", 0, false},
		{"abc", 0, false},
		{"-", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseCLConfidenceMin(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParseCLConfidenceMin(%q)=(%d,%v) want (%d,%v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
