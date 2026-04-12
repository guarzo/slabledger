package mmutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMMSearchTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  MMSearchTitleFields
	}{
		{
			name:  "full title with SIR variation",
			title: "Cynthia's Garchomp ex 2025 Scarlet & Violet: Destined Rivals Special Illustration Rare #232/182 PSA 10",
			want: MMSearchTitleFields{
				PlayerName: "Cynthia's Garchomp ex",
				Year:       "2025",
				Set:        "Scarlet & Violet: Destined Rivals",
				Variation:  "Special Illustration Rare",
				CardNumber: "#232/182",
			},
		},
		{
			name:  "holo variation with EX set prefix",
			title: "Entei Star 2005 EX: Unseen Forces Holo #113/115 PSA 8",
			want: MMSearchTitleFields{
				PlayerName: "Entei Star",
				Year:       "2005",
				Set:        "EX: Unseen Forces",
				Variation:  "Holo",
				CardNumber: "#113/115",
			},
		},
		{
			name:  "art rare variation",
			title: "Mewtwo 2023 Japanese Scarlet & Violet: 151 Art Rare #183/165 PSA 10",
			want: MMSearchTitleFields{
				PlayerName: "Mewtwo",
				Year:       "2023",
				Set:        "Japanese Scarlet & Violet: 151",
				Variation:  "Art Rare",
				CardNumber: "#183/165",
			},
		},
		{
			name:  "super rare variation",
			title: "Mewtwo V 2022 Japanese Sword & Shield: Pokemon GO Super Rare #74/71 PSA 9",
			want: MMSearchTitleFields{
				PlayerName: "Mewtwo V",
				Year:       "2022",
				Set:        "Japanese Sword & Shield: Pokemon GO",
				Variation:  "Super Rare",
				CardNumber: "#74/71",
			},
		},
		{
			name:  "BGS grader with half grade",
			title: "Charizard 1999 Base Set Holo #4/102 BGS 9.5",
			want: MMSearchTitleFields{
				PlayerName: "Charizard",
				Year:       "1999",
				Set:        "Base Set",
				Variation:  "Holo",
				CardNumber: "#4/102",
			},
		},
		{
			name:  "Raw TCG grader",
			title: "Pikachu ex 2026 Mega Evolution: Ascended Heroes Special Illustration Rare #276/217 Raw TCG (Near Mint)",
			want: MMSearchTitleFields{
				PlayerName: "Pikachu ex",
				Year:       "2026",
				Set:        "Mega Evolution: Ascended Heroes",
				Variation:  "Special Illustration Rare",
				CardNumber: "#276/217",
			},
		},
		{
			name:  "alphanumeric card number",
			title: "Kanazawa's Pikachu 2020 Japanese S-P Promo Pokemon Center Kanazawa Opening #144/S-P PSA 10",
			want: MMSearchTitleFields{
				PlayerName: "Kanazawa's Pikachu",
				Year:       "2020",
				Set:        "Japanese S-P Promo Pokemon Center Kanazawa Opening",
				Variation:  "",
				CardNumber: "#144/S-P",
			},
		},
		{
			name:  "1st Edition variation",
			title: "Dark Charizard 2000 Team Rocket 1st Edition Holo #4/82 PSA 9",
			want: MMSearchTitleFields{
				PlayerName: "Dark Charizard",
				Year:       "2000",
				Set:        "Team Rocket",
				Variation:  "1st Edition Holo",
				CardNumber: "#4/82",
			},
		},
		{
			name:  "empty title",
			title: "",
			want:  MMSearchTitleFields{},
		},
		{
			name:  "no year — everything in player name",
			title: "Charizard Base Set Holo PSA 10",
			want: MMSearchTitleFields{
				PlayerName: "Charizard Base Set Holo",
			},
		},
		{
			name:  "no card number",
			title: "Pikachu 2023 Crown Zenith Holo PSA 10",
			want: MMSearchTitleFields{
				PlayerName: "Pikachu",
				Year:       "2023",
				Set:        "Crown Zenith",
				Variation:  "Holo",
			},
		},
		{
			name:  "no variation — set only",
			title: "Nidoking 2003 Aquapolis #150/147 PSA 9",
			want: MMSearchTitleFields{
				PlayerName: "Nidoking",
				Year:       "2003",
				Set:        "Aquapolis",
				CardNumber: "#150/147",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMMSearchTitle(tt.title)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSplitSetVariation(t *testing.T) {
	tests := []struct {
		name    string
		middle  string
		wantSet string
		wantVar string
	}{
		{
			name:    "known variation at end",
			middle:  "Scarlet & Violet: Destined Rivals Special Illustration Rare",
			wantSet: "Scarlet & Violet: Destined Rivals",
			wantVar: "Special Illustration Rare",
		},
		{
			name:    "holo at end",
			middle:  "Base Set Holo",
			wantSet: "Base Set",
			wantVar: "Holo",
		},
		{
			name:    "no known variation",
			middle:  "Japanese S-P Promo Pokemon Center Kanazawa Opening",
			wantSet: "Japanese S-P Promo Pokemon Center Kanazawa Opening",
			wantVar: "",
		},
		{
			name:    "empty middle",
			middle:  "",
			wantSet: "",
			wantVar: "",
		},
		{
			name:    "art rare",
			middle:  "Japanese Scarlet & Violet: 151 Art Rare",
			wantSet: "Japanese Scarlet & Violet: 151",
			wantVar: "Art Rare",
		},
		{
			name:    "full art",
			middle:  "XY Evolutions Full Art",
			wantSet: "XY Evolutions",
			wantVar: "Full Art",
		},
		{
			name:    "1st edition holo",
			middle:  "Team Rocket 1st Edition Holo",
			wantSet: "Team Rocket",
			wantVar: "1st Edition Holo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSet, gotVar := splitSetVariation(tt.middle)
			assert.Equal(t, tt.wantSet, gotSet, "set")
			assert.Equal(t, tt.wantVar, gotVar, "variation")
		})
	}
}
