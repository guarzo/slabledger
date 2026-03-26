package social

import (
	"strings"
	"testing"
)

func TestDetectCardTheme(t *testing.T) {
	tests := []struct {
		name    string
		cards   []PostCardDetail
		wantHas string
	}{
		{name: "fire type", cards: []PostCardDetail{{CardName: "Charizard VMAX"}}, wantHas: "volcanic"},
		{name: "water type", cards: []PostCardDetail{{CardName: "Blastoise EX"}}, wantHas: "ocean"},
		{name: "electric type", cards: []PostCardDetail{{CardName: "Pikachu VMAX"}}, wantHas: "electric"},
		{name: "dark type", cards: []PostCardDetail{{CardName: "Umbreon VMAX"}}, wantHas: "cosmic"},
		{name: "grass type", cards: []PostCardDetail{{CardName: "Venusaur EX"}}, wantHas: "forest"},
		{name: "mixed cards", cards: []PostCardDetail{{CardName: "Mimikyu"}, {CardName: "Cubone"}}, wantHas: "abstract energy"},
		{name: "empty cards", cards: nil, wantHas: "abstract energy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectCardTheme(tt.cards)
			if !strings.Contains(got, tt.wantHas) {
				t.Errorf("detectCardTheme() = %q, want substring %q", got, tt.wantHas)
			}
		})
	}
}

func TestBuildBackgroundPrompt(t *testing.T) {
	cards := []PostCardDetail{{CardName: "Charizard", SetName: "Base Set"}}
	prompt := buildBackgroundPrompt(PostTypeHotDeals, cards)
	if !strings.Contains(prompt, "Energetic") {
		t.Error("expected hot_deals mood in prompt")
	}
	if !strings.Contains(prompt, "volcanic") {
		t.Error("expected fire theme for Charizard")
	}
	if !strings.Contains(prompt, "No text") {
		t.Error("expected constraints in prompt")
	}
}

func TestBuildCardBackgroundPrompt(t *testing.T) {
	card := PostCardDetail{CardName: "Dark Gyarados", SetName: "Team Rocket"}
	prompt := buildCardBackgroundPrompt(PostTypeHotDeals, card)
	if !strings.Contains(prompt, "Dark Gyarados") {
		t.Error("expected card name in card prompt")
	}
	if !strings.Contains(prompt, "Team Rocket") {
		t.Error("expected set name in card prompt")
	}
}
