package picks

import (
	"encoding/json"
	"fmt"
	"strings"
)

// candidateRequest is the JSON schema for LLM candidate generation output.
type candidateRequest struct {
	CardName  string `json:"card_name"`
	SetName   string `json:"set_name"`
	Grade     string `json:"grade"`
	Rationale string `json:"rationale"`
}

// scoredPick is the JSON schema for LLM scoring output.
type scoredPick struct {
	CardName          string `json:"card_name"`
	SetName           string `json:"set_name"`
	Grade             string `json:"grade"`
	Direction         string `json:"direction"`
	Confidence        string `json:"confidence"`
	BuyThesis         string `json:"buy_thesis"`
	TargetBuyPrice    int    `json:"target_buy_price_cents"`
	ExpectedSellPrice int    `json:"expected_sell_price_cents"`
	Rank              int    `json:"rank"`
	Signals           []struct {
		Factor    string `json:"factor"`
		Direction string `json:"direction"`
		Title     string `json:"title"`
		Detail    string `json:"detail"`
	} `json:"signals"`
}

// BuildCandidatePrompt constructs the system prompt for candidate generation.
func BuildCandidatePrompt(profile ProfitabilityProfile, watchlist []WatchlistItem, heldCards []string) string {
	var sb strings.Builder

	sb.WriteString("You are an expert Pokemon TCG card market analyst. Your job is to recommend PSA-graded cards worth acquiring.\n\n")

	sb.WriteString("## Portfolio Profitability Profile\n")
	if len(profile.TopEras) > 0 {
		fmt.Fprintf(&sb, "Most profitable eras: %s\n", strings.Join(profile.TopEras, ", "))
	}
	for _, g := range profile.ProfitableGrades {
		fmt.Fprintf(&sb, "- %s: avg ROI %.0f%%, avg margin $%.2f, %d sales\n", g.Grade, g.AvgROI, float64(g.AvgMargin)/100, g.Count)
	}
	for _, t := range profile.ProfitablePriceTiers {
		fmt.Fprintf(&sb, "- Buy range $%.2f-$%.2f: avg ROI %.0f%%, %d sales\n", float64(t.MinPrice)/100, float64(t.MaxPrice)/100, t.AvgROI, t.Count)
	}
	if profile.AvgDaysToSell > 0 {
		fmt.Fprintf(&sb, "Average days to sell: %d\n", profile.AvgDaysToSell)
	}
	if len(profile.TopChannels) > 0 {
		fmt.Fprintf(&sb, "Top sales channels: %s\n", strings.Join(profile.TopChannels, ", "))
	}

	if len(watchlist) > 0 {
		sb.WriteString("\n## Cards to Re-evaluate\nInclude these in your candidate list:\n")
		for _, w := range watchlist {
			fmt.Fprintf(&sb, "- %s | %s | %s\n", w.CardName, w.SetName, w.Grade)
		}
	}

	if len(heldCards) > 0 {
		sb.WriteString("\n## Already Held (exclude from recommendations)\n")
		for _, c := range heldCards {
			fmt.Fprintf(&sb, "- %s\n", c)
		}
	}

	sb.WriteString("\n## Instructions\n")
	sb.WriteString("Generate 20-30 candidate PSA-graded Pokemon cards worth acquiring. Focus on cards that align with the profitability profile above.\n")
	sb.WriteString("Consider: market trends, population dynamics, upcoming releases, collector demand, and grading ROI potential.\n")
	sb.WriteString("Do NOT recommend cards already held.\n")
	sb.WriteString("Return ONLY a JSON array, no markdown fences:\n")
	sb.WriteString(`[{"card_name": "...", "set_name": "...", "grade": "PSA 10", "rationale": "..."}]`)

	return sb.String()
}

// BuildScoringPrompt constructs the system prompt for scoring & ranking.
func BuildScoringPrompt(candidates []candidateRequest, profile ProfitabilityProfile) string {
	var sb strings.Builder

	sb.WriteString("You are an expert Pokemon TCG card market analyst. Score and rank these acquisition candidates.\n\n")

	sb.WriteString("## Profitability Context\n")
	if len(profile.TopEras) > 0 {
		fmt.Fprintf(&sb, "Profitable eras: %s\n", strings.Join(profile.TopEras, ", "))
	}
	if profile.AvgDaysToSell > 0 {
		fmt.Fprintf(&sb, "Avg days to sell: %d\n", profile.AvgDaysToSell)
	}

	sb.WriteString("\n## Candidates\n")
	candidatesJSON, _ := json.Marshal(candidates)
	sb.WriteString(string(candidatesJSON))

	sb.WriteString("\n\n## Instructions\n")
	sb.WriteString("For each candidate, evaluate market opportunity and alignment with the profitability profile.\n")
	sb.WriteString("Select the top 10 and rank them. For each:\n")
	sb.WriteString("- direction: buy, watch, or avoid\n")
	sb.WriteString("- confidence: high, medium, or low\n")
	sb.WriteString("- buy_thesis: 1-2 sentence explanation\n")
	sb.WriteString("- target_buy_price_cents and expected_sell_price_cents in USD cents\n")
	sb.WriteString("- 2-3 signals with factor, direction (bullish/bearish/neutral), title (3-5 words), detail (1 sentence with numbers)\n")
	sb.WriteString("- rank: 1 (best) to 10\n\n")
	sb.WriteString("Return ONLY a JSON array, no markdown fences:\n")
	sb.WriteString(`[{"card_name": "...", "set_name": "...", "grade": "...", "direction": "buy", "confidence": "high", "buy_thesis": "...", "target_buy_price_cents": 15000, "expected_sell_price_cents": 22500, "rank": 1, "signals": [{"factor": "population", "direction": "bullish", "title": "Low PSA 10 pop", "detail": "Only 847 graded"}]}]`)

	return sb.String()
}

// ParseCandidates parses the LLM's candidate generation response.
func ParseCandidates(raw string) ([]candidateRequest, error) {
	cleaned := cleanJSONResponse(raw)
	var candidates []candidateRequest
	if err := json.Unmarshal([]byte(cleaned), &candidates); err != nil {
		return nil, fmt.Errorf("parse candidates: %w", err)
	}
	return candidates, nil
}

// ParseScoredPicks parses the LLM's scoring response into domain Pick objects.
func ParseScoredPicks(raw string) ([]Pick, error) {
	cleaned := cleanJSONResponse(raw)
	var scored []scoredPick
	if err := json.Unmarshal([]byte(cleaned), &scored); err != nil {
		return nil, fmt.Errorf("parse scored picks: %w", err)
	}

	result := make([]Pick, 0, len(scored))
	for _, s := range scored {
		signals := make([]Signal, 0, len(s.Signals))
		for _, sig := range s.Signals {
			signals = append(signals, Signal{
				Factor:    sig.Factor,
				Direction: SignalDirection(sig.Direction),
				Title:     sig.Title,
				Detail:    sig.Detail,
			})
		}
		result = append(result, Pick{
			CardName:          s.CardName,
			SetName:           s.SetName,
			Grade:             s.Grade,
			Direction:         Direction(s.Direction),
			Confidence:        Confidence(s.Confidence),
			BuyThesis:         s.BuyThesis,
			TargetBuyPrice:    s.TargetBuyPrice,
			ExpectedSellPrice: s.ExpectedSellPrice,
			Rank:              s.Rank,
			Signals:           signals,
			Source:            SourceAI,
		})
	}
	return result, nil
}

// cleanJSONResponse strips markdown code fences from LLM responses.
func cleanJSONResponse(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
