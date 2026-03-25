package pricecharting

import (
	"context"
	"testing"
)

func TestVerifyProductMatch(t *testing.T) {
	tests := []struct {
		name           string
		productName    string
		expectedNumber string
		want           bool
	}{
		{"empty number", "Charizard #4", "", true},
		{"exact hash match", "Pokemon Charizard #4/102", "4", true},
		{"number in token", "Pokemon Charizard 4/102", "4", true},
		{"number mismatch", "Pokemon Pikachu #25/25", "4", false},
		{"slash number match", "Charizard 4/102 Base Set", "4/102", true},
		{"leading zero match", "Pokemon Charizard #004", "004", true},
		{"letter-prefixed collector number", "Pokemon Scarlet Violet Promos #SVP001 Pikachu", "SVP001", true},
		{"gallery collector number", "Pokemon Crown Zenith Galarian Gallery #GG42 Zeraora VMAX", "GG42", true},
		{"SWSH prefix mismatch", "Pokemon Sword Shield Black Star Promos SWSH075", "075", false},
		{"SWSH prefix with hash", "Pokemon Sword Shield Black Star Promos #SWSH029", "029", false},
		{"SM prefix mismatch", "Sun Moon Promos SM162 Pikachu", "SM162", true},
		{"numeric digits differ", "Pokemon Pikachu #25", "75", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyProductMatch(context.Background(), tt.productName, tt.expectedNumber)
			if got != tt.want {
				t.Errorf("VerifyProductMatch(%q, %q) = %v, want %v", tt.productName, tt.expectedNumber, got, tt.want)
			}
		})
	}
}

func TestVerifySetOverlap(t *testing.T) {
	tests := []struct {
		name            string
		consoleName     string
		expectedSetName string
		want            bool
	}{
		{"empty console", "", "Base Set", false},
		{"empty expected", "Pokemon Cards", "", true},
		{"overlap", "Pokemon Expedition", "Expedition", true},
		{"no overlap", "Pokemon Base Set", "Expedition", false},
		{"partial word", "Pokemon Crown Zenith", "Crown Zenith", true},
		{"multi-word set single overlap", "Pokemon Modern Masters Collection", "Modern Masters", true},
		{"multi-word single token overlap fails", "Masters Collection", "Modern Masters", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifySetOverlap(context.Background(), tt.consoleName, tt.expectedSetName)
			if got != tt.want {
				t.Errorf("VerifySetOverlap(%q, %q) = %v, want %v", tt.consoleName, tt.expectedSetName, got, tt.want)
			}
		})
	}
}
