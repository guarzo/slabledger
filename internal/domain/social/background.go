package social

import (
	"fmt"
	"strings"
)

var postTypeMoods = map[PostType]string{
	PostTypeHotDeals:    "Energetic, warm, urgency — think glowing embers, molten energy, heat haze",
	PostTypeNewArrivals: "Fresh, premium, discovery — think crystalline light, clean gradients, subtle sparkle",
	PostTypePriceMovers: "Dynamic, momentum, velocity — think data streams, motion blur, shifting light",
}

type pokemonTheme struct {
	keywords []string
	theme    string
}

var pokemonThemes = []pokemonTheme{
	{keywords: []string{"charizard", "arcanine", "flareon", "blaziken", "infernape", "typhlosion", "magmar", "moltres", "entei", "reshiram", "ho-oh"},
		theme: "volcanic, flame accents, warm oranges and reds"},
	{keywords: []string{"blastoise", "gyarados", "vaporeon", "lapras", "suicune", "kyogre", "greninja", "feraligatr", "milotic", "lugia"},
		theme: "ocean depths, aquatic blues, flowing water"},
	{keywords: []string{"pikachu", "raichu", "jolteon", "electabuzz", "zapdos", "raikou", "luxray", "zeraora"},
		theme: "electric arcs, lightning, bright yellows"},
	{keywords: []string{"umbreon", "mewtwo", "gengar", "darkrai", "absol", "tyranitar", "espeon", "alakazam", "gardevoir", "lunala"},
		theme: "cosmic, nebula, deep purples and blacks"},
	{keywords: []string{"venusaur", "leafeon", "sceptile", "celebi", "shaymin", "torterra", "decidueye"},
		theme: "lush forest, emerald tones, natural light"},
}

const defaultTheme = "abstract energy, prismatic light, rich dark tones"

func detectCardTheme(cards []PostCardDetail) string {
	if len(cards) == 0 {
		return defaultTheme
	}
	type themeScore struct {
		theme string
		count int
	}
	scores := make(map[string]*themeScore)
	for _, card := range cards {
		nameLower := strings.ToLower(card.CardName + " " + card.SetName)
		for _, pt := range pokemonThemes {
			for _, kw := range pt.keywords {
				if strings.Contains(nameLower, kw) {
					if s, ok := scores[pt.theme]; ok {
						s.count++
					} else {
						scores[pt.theme] = &themeScore{theme: pt.theme, count: 1}
					}
					break
				}
			}
		}
	}
	if len(scores) == 0 {
		return defaultTheme
	}
	var best *themeScore
	for _, s := range scores {
		if best == nil || s.count > best.count {
			best = s
		}
	}
	return best.theme
}

func buildBackgroundPrompt(postType PostType, cards []PostCardDetail) string {
	mood := postTypeMoods[postType]
	if mood == "" {
		mood = postTypeMoods[PostTypeNewArrivals]
	}
	theme := detectCardTheme(cards)
	return fmt.Sprintf(`Generate a 1024x1024 background image for a social media post about collectible graded Pokemon cards.

Mood: %s
Theme: %s
Style: Abstract, atmospheric, No text, no cards, no characters, no logos. Dark base tones suitable for white text overlay. Rich but not overwhelming. Leave the center relatively clear — cards will be composited on top.`, mood, theme)
}

func buildCardBackgroundPrompt(postType PostType, card PostCardDetail) string {
	mood := postTypeMoods[postType]
	if mood == "" {
		mood = postTypeMoods[PostTypeNewArrivals]
	}
	theme := detectCardTheme([]PostCardDetail{card})
	return fmt.Sprintf(`Generate a 1024x1024 background image for a social media slide featuring %s from %s.

Mood: %s
Theme: %s
Style: Abstract, atmospheric, no text, no cards, no characters, no logos. Dark base tones suitable for white text overlay. Subtle and immersive — the card slab will be composited in the center.`, card.CardName, card.SetName, mood, theme)
}
