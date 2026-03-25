package cards

import (
	"context"
	"testing"
)

func TestParseQuery_NameOnly(t *testing.T) {
	pq := parseSearchQuery("Charizard")
	if pq.CardName != "Charizard" {
		t.Errorf("expected CardName %q, got %q", "Charizard", pq.CardName)
	}
	if pq.SetName != "" {
		t.Errorf("expected empty SetName, got %q", pq.SetName)
	}
	if pq.CardNumber != "" {
		t.Errorf("expected empty CardNumber, got %q", pq.CardNumber)
	}
	if len(pq.Tokens) != 1 || pq.Tokens[0] != "charizard" {
		t.Errorf("expected tokens [charizard], got %v", pq.Tokens)
	}
}

func TestParseQuery_NameAndSet(t *testing.T) {
	pq := parseSearchQuery("Charizard base set")
	if pq.CardName != "Charizard" {
		t.Errorf("expected CardName %q, got %q", "Charizard", pq.CardName)
	}
	if pq.SetName != "base set" {
		t.Errorf("expected SetName %q, got %q", "base set", pq.SetName)
	}
	if pq.CardNumber != "" {
		t.Errorf("expected empty CardNumber, got %q", pq.CardNumber)
	}
}

func TestParseQuery_NameAndNumber(t *testing.T) {
	pq := parseSearchQuery("Charizard #4")
	if pq.CardName != "Charizard" {
		t.Errorf("expected CardName %q, got %q", "Charizard", pq.CardName)
	}
	if pq.CardNumber != "4" {
		t.Errorf("expected CardNumber %q, got %q", "4", pq.CardNumber)
	}
}

func TestParseQuery_NumberWithSlash(t *testing.T) {
	pq := parseSearchQuery("4/102")
	if pq.CardNumber != "4/102" {
		t.Errorf("expected CardNumber %q, got %q", "4/102", pq.CardNumber)
	}
}

func TestParseQuery_JustNumber(t *testing.T) {
	pq := parseSearchQuery("25")
	if pq.CardNumber != "25" {
		t.Errorf("expected CardNumber %q, got %q", "25", pq.CardNumber)
	}
}

func TestParseQuery_SetBoundaryCheck(t *testing.T) {
	// "neo" should not match inside "neon"
	pq := parseSearchQuery("Neon Card")
	if pq.SetName != "" {
		t.Errorf("expected empty SetName (neo should not match inside neon), got %q", pq.SetName)
	}
}

func TestScoring_ExactNameMatch(t *testing.T) {
	card := Card{Name: "Charizard", ImageURL: "http://img"}
	pq := parsedQuery{CardName: "Charizard", Tokens: []string{"charizard"}}
	score := scoreCard(card, pq)
	// 80 (exact) + 5 (image) = 85
	if score != 85.0 {
		t.Errorf("expected score 85, got %f", score)
	}
}

func TestScoring_PrefixMatch(t *testing.T) {
	card := Card{Name: "Charizard EX"}
	pq := parsedQuery{CardName: "Charizard", Tokens: []string{"charizard"}}
	score := scoreCard(card, pq)
	// 60 (prefix) + 0 (no image)
	if score != 60.0 {
		t.Errorf("expected score 60, got %f", score)
	}
}

func TestScoring_ContainsMatch(t *testing.T) {
	card := Card{Name: "Dark Charizard"}
	pq := parsedQuery{CardName: "Charizard", Tokens: []string{"charizard"}}
	score := scoreCard(card, pq)
	// 40 (contains)
	if score != 40.0 {
		t.Errorf("expected score 40, got %f", score)
	}
}

func TestScoring_TokenMatch(t *testing.T) {
	card := Card{Name: "Shining Charizard VMAX"}
	pq := parsedQuery{CardName: "Charizard VMAX", Tokens: []string{"charizard", "vmax"}}
	// Contains "charizard vmax" as substring? "shining charizard vmax" contains "charizard vmax" -> yes
	score := scoreCard(card, pq)
	// 40 (contains)
	if score != 40.0 {
		t.Errorf("expected score 40, got %f", score)
	}
}

func TestScoring_NumberExactMatch(t *testing.T) {
	card := Card{Name: "Charizard", Number: "4", ImageURL: "http://img"}
	pq := parsedQuery{CardNumber: "4", CardName: "Charizard", Tokens: []string{"charizard"}}
	score := scoreCard(card, pq)
	// 100 (number exact) + 80 (name exact) + 5 (image) = 185
	if score != 185.0 {
		t.Errorf("expected score 185, got %f", score)
	}
}

func TestScoring_NumberMismatchReturnsZero(t *testing.T) {
	card := Card{Name: "Charizard", Number: "5"}
	pq := parsedQuery{CardNumber: "4", CardName: "Charizard", Tokens: []string{"charizard"}}
	score := scoreCard(card, pq)
	if score != 0 {
		t.Errorf("expected score 0 for number mismatch, got %f", score)
	}
}

func TestScoring_SetBonus(t *testing.T) {
	card := Card{Name: "Charizard", SetName: "Base Set"}
	pq := parsedQuery{CardName: "Charizard", SetName: "base set", Tokens: []string{"charizard"}}
	score := scoreCard(card, pq)
	// 80 (exact name) + 20 (set match) = 100
	if score != 100.0 {
		t.Errorf("expected score 100, got %f", score)
	}
}

func TestScoring_NoMatchReturnsZero(t *testing.T) {
	card := Card{Name: "Pikachu"}
	pq := parsedQuery{CardName: "Charizard", Tokens: []string{"charizard"}}
	score := scoreCard(card, pq)
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
}

// mockCardProvider is a minimal mock for testing SearchService.
type mockCardProvider struct {
	cards      []Card
	totalCount int
	err        error
}

func (m *mockCardProvider) GetCards(_ context.Context, _ string) ([]Card, error) {
	return nil, nil
}
func (m *mockCardProvider) GetSet(_ context.Context, _ string) (*Set, error) {
	return nil, nil
}
func (m *mockCardProvider) ListAllSets(_ context.Context) ([]Set, error) {
	return nil, nil
}
func (m *mockCardProvider) Available() bool { return true }
func (m *mockCardProvider) SearchCards(_ context.Context, _ SearchCriteria) ([]Card, int, error) {
	return m.cards, m.totalCount, m.err
}

func TestSearchService_Search(t *testing.T) {
	provider := &mockCardProvider{
		cards: []Card{
			{ID: "1", Name: "Charizard", Number: "4", SetName: "Base Set", ImageURL: "http://img"},
			{ID: "2", Name: "Dark Charizard", Number: "21", SetName: "Team Rocket"},
			{ID: "3", Name: "Pikachu", Number: "25", SetName: "Base Set"},
		},
		totalCount: 3,
	}

	svc := NewSearchService(provider)
	results, total, err := svc.Search(context.Background(), "Charizard", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	// Pikachu should be filtered out (score 0)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Charizard (exact) should rank higher than Dark Charizard (contains)
	if results[0].Card.ID != "1" {
		t.Errorf("expected first result to be Charizard (id=1), got id=%s", results[0].Card.ID)
	}
	if results[1].Card.ID != "2" {
		t.Errorf("expected second result to be Dark Charizard (id=2), got id=%s", results[1].Card.ID)
	}
}

func TestSearchService_LimitResults(t *testing.T) {
	provider := &mockCardProvider{
		cards: []Card{
			{ID: "1", Name: "Charizard"},
			{ID: "2", Name: "Charizard EX"},
			{ID: "3", Name: "Charizard VMAX"},
		},
		totalCount: 3,
	}

	svc := NewSearchService(provider)
	results, _, err := svc.Search(context.Background(), "Charizard", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results (limited), got %d", len(results))
	}
}
