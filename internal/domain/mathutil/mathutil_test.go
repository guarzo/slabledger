package mathutil

import (
	"testing"
)

func TestRound2(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"positive", 3.14159, 3.14},
		{"negative", -2.7182, -2.72},
		{"zero", 0.0, 0.0},
		{"many decimal places", 1.999999, 2.0},
		{"already two decimals", 5.55, 5.55},
		{"rounds up", 1.555, 1.56},
		{"rounds down", 1.554, 1.55},
		{"whole number", 42.0, 42.0},
		{"single decimal", 3.1, 3.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Round2(tt.input)
			if got != tt.expected {
				t.Errorf("Round2(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestToCents(t *testing.T) {
	tests := []struct {
		input    float64
		expected int64
	}{
		{12.34, 1234},
		{0.005, 1},
		{0.0, 0},
		{-5.50, -550},
		{99.999, 10000},
	}
	for _, tt := range tests {
		got := ToCents(tt.input)
		if got != tt.expected {
			t.Errorf("ToCents(%v) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestToCentsInt(t *testing.T) {
	tests := []struct {
		input    float64
		expected int
	}{
		{12.34, 1234},
		{0.005, 1},
		{0.0, 0},
		{-5.50, -550},
		{99.999, 10000},
	}
	for _, tt := range tests {
		got := ToCentsInt(tt.input)
		if got != tt.expected {
			t.Errorf("ToCentsInt(%v) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestToDollars(t *testing.T) {
	tests := []struct {
		input    int64
		expected float64
	}{
		{1234, 12.34},
		{0, 0.0},
		{-550, -5.50},
		{1, 0.01},
	}
	for _, tt := range tests {
		got := ToDollars(tt.input)
		if got != tt.expected {
			t.Errorf("ToDollars(%d) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestConfidenceLabel(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "low"},
		{1, "low"},
		{4, "low"},
		{5, "medium"},
		{10, "medium"},
		{19, "medium"},
		{20, "high"},
		{100, "high"},
	}
	for _, tt := range tests {
		got := ConfidenceLabel(tt.n)
		if got != tt.want {
			t.Errorf("ConfidenceLabel(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestConfidenceScore(t *testing.T) {
	tests := []struct {
		n    int
		want float64
	}{
		{0, 0.3},
		{4, 0.3},
		{5, 0.6},
		{19, 0.6},
		{20, 1.0},
		{50, 1.0},
	}
	for _, tt := range tests {
		got := ConfidenceScore(tt.n)
		if got != tt.want {
			t.Errorf("ConfidenceScore(%d) = %v, want %v", tt.n, got, tt.want)
		}
	}
}

func TestFormatGrade(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  string
	}{
		{"integer 10", 10, "10"},
		{"integer 9", 9, "9"},
		{"integer 1", 1, "1"},
		{"integer 0", 0, "0"},
		{"half-grade 9.5", 9.5, "9.5"},
		{"half-grade 8.5", 8.5, "8.5"},
		{"half-grade 0.5", 0.5, "0.5"},
		{"precise decimal 9.15", 9.15, "9.15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGrade(tt.input)
			if got != tt.want {
				t.Errorf("FormatGrade(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
