package pokemonprice

import (
	"context"
	"testing"
)

func TestMatchByCardNumber(t *testing.T) {
	tests := []struct {
		name         string
		data         []CardPriceData
		collectorNum string
		originalSet  string
		droppedSet   bool
		wantNil      bool
		wantSetName  string // expected SetName of matched card
	}{
		{
			name:         "no candidates",
			data:         []CardPriceData{{CardNumber: "100", SetName: "Base Set"}},
			collectorNum: "161",
			originalSet:  "Promo",
			droppedSet:   false,
			wantNil:      true,
		},
		{
			name:         "single match with set filter active",
			data:         []CardPriceData{{CardNumber: "161", SetName: "Pokemon Promo"}},
			collectorNum: "161",
			originalSet:  "Promo",
			droppedSet:   false,
			wantNil:      false,
			wantSetName:  "Pokemon Promo",
		},
		{
			name: "multiple matches with set filter active returns first",
			data: []CardPriceData{
				{CardNumber: "161", SetName: "Pokemon Promo"},
				{CardNumber: "161", SetName: "Prismatic Evolutions"},
			},
			collectorNum: "161",
			originalSet:  "Promo",
			droppedSet:   false,
			wantNil:      false,
			wantSetName:  "Pokemon Promo",
		},
		{
			name: "dropped set - disambiguate by set overlap",
			data: []CardPriceData{
				{CardNumber: "161/131", SetName: "Prismatic Evolutions"},
				{CardNumber: "161", SetName: "SVP Pokemon Promo"},
			},
			collectorNum: "161",
			originalSet:  "pokemon promo",
			droppedSet:   true,
			wantNil:      false,
			wantSetName:  "SVP Pokemon Promo",
		},
		{
			name: "dropped set - no overlap returns nil",
			data: []CardPriceData{
				{CardNumber: "161/131", SetName: "Prismatic Evolutions"},
				{CardNumber: "161", SetName: "Some Other Set"},
			},
			collectorNum: "161",
			originalSet:  "pokemon promo",
			droppedSet:   true,
			wantNil:      true,
		},
		{
			name: "dropped set - empty original set returns nil (ambiguous)",
			data: []CardPriceData{
				{CardNumber: "161", SetName: "Pokemon Promo"},
			},
			collectorNum: "161",
			originalSet:  "",
			droppedSet:   true,
			wantNil:      true,
		},
		{
			name:         "dropped set - single match with overlap",
			data:         []CardPriceData{{CardNumber: "161", SetName: "SVP Pokemon Promo"}},
			collectorNum: "161",
			originalSet:  "pokemon promo",
			droppedSet:   true,
			wantNil:      false,
			wantSetName:  "SVP Pokemon Promo",
		},
		{
			name: "card number normalization - slash format",
			data: []CardPriceData{
				{CardNumber: "161/131", SetName: "Prismatic Evolutions"},
			},
			collectorNum: "161",
			originalSet:  "Prismatic Evolutions",
			droppedSet:   false,
			wantNil:      false,
			wantSetName:  "Prismatic Evolutions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchByCardNumber(context.Background(), tt.data, tt.collectorNum, tt.originalSet, tt.droppedSet)
			if tt.wantNil {
				if result != nil {
					t.Errorf("matchByCardNumber() = %+v, want nil", result)
				}
				return
			}
			if result == nil {
				t.Fatal("matchByCardNumber() = nil, want non-nil")
			}
			if result.SetName != tt.wantSetName {
				t.Errorf("matchByCardNumber().SetName = %q, want %q", result.SetName, tt.wantSetName)
			}
		})
	}
}

func TestSetOverlaps(t *testing.T) {
	tests := []struct {
		name          string
		resultSet     string
		normalizedSet string
		want          bool
	}{
		{"exact match", "Expedition", "expedition", true},
		{"partial match", "SVP Pokemon Promo", "pokemon promo", true},
		{"no match", "Prismatic Evolutions", "pokemon promo", false},
		{"short words ignored", "XY Set", "xy", false},
		{"multi-word overlap", "Scarlet and Violet Promo", "scarlet violet promo", true},
		{"case insensitive", "EXPEDITION BASE", "Expedition", true},
		{"empty result set", "", "pokemon promo", false},
		// Era abbreviation expansion
		{"SV era expansion", "Scarlet & Violet Promos", "SV BLACK STAR PROMO", true},
		{"SWSH era expansion", "Sword & Shield Promos", "SWSH BLACK STAR PROMO", true},
		{"SV abbrev both sides", "SV Promos", "SV BLACK STAR PROMO", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setOverlaps(context.Background(), tt.resultSet, tt.normalizedSet); got != tt.want {
				t.Errorf("setOverlaps(%q, %q) = %v, want %v", tt.resultSet, tt.normalizedSet, got, tt.want)
			}
		})
	}
}

func TestIsGenericSet(t *testing.T) {
	tests := []struct {
		set  string
		want bool
	}{
		{"TCG Cards", true},
		{"tcg cards", true},
		{"Pokemon Cards", true},
		{"Cards", true},
		{"Pokemon", true},
		{"Trading Cards", true},
		{"Prismatic Evolutions", false},
		{"Pokemon Promo", false},
		{"Expedition", false},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.set, func(t *testing.T) {
			if got := isGenericSet(context.Background(), tt.set); got != tt.want {
				t.Errorf("isGenericSet(%q) = %v, want %v", tt.set, got, tt.want)
			}
		})
	}
}
