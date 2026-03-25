package psa

import (
	"testing"
)

func TestParseGrade(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"GEM MT 10", 10},
		{"MINT 9", 9},
		{"NM-MT 8", 8},
		{"NM-MT 8.5", 8.5},
		{"NM 7", 7},
		{"EX-MT 6", 6},
		{"VG-EX 4", 4},
		{"PR 1", 1},
		{"", 0},
		{"AUTHENTIC", 0},
	}

	for _, tt := range tests {
		got := ParseGrade(tt.input)
		if got != tt.expected {
			t.Errorf("ParseGrade(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestBuildCardName(t *testing.T) {
	tests := []struct {
		name     string
		info     *CertInfo
		expected string
	}{
		{
			name:     "subject with variety appended",
			info:     &CertInfo{Year: "2022", Brand: "POKEMON", Category: "CELEBRATIONS", Subject: "CHARIZARD", Variety: "V"},
			expected: "CHARIZARD V",
		},
		{
			name:     "1st edition variety",
			info:     &CertInfo{Subject: "DARK GYARADOS-HOLO", Variety: "1ST EDITION"},
			expected: "DARK GYARADOS-HOLO 1ST EDITION",
		},
		{
			name:     "shadowless variety",
			info:     &CertInfo{Subject: "BLASTOISE-HOLO", Variety: "SHADOWLESS"},
			expected: "BLASTOISE-HOLO SHADOWLESS",
		},
		{
			name:     "empty subject falls back to category",
			info:     &CertInfo{Category: "CELEBRATIONS"},
			expected: "CELEBRATIONS",
		},
		{
			name:     "only subject no variety",
			info:     &CertInfo{Subject: "PIKACHU"},
			expected: "PIKACHU",
		},
		{
			name:     "empty variety not appended",
			info:     &CertInfo{Subject: "MEWTWO-REV.FOIL", Variety: ""},
			expected: "MEWTWO-REV.FOIL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCardName(tt.info)
			if got != tt.expected {
				t.Errorf("BuildCardName = %q, want %q", got, tt.expected)
			}
		})
	}
}
