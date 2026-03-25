package mocks

import (
	"context"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/cards"
)

// MockCardProvider implements a mock cards provider for testing
type MockCardProvider struct {
	behavior MockBehavior
	sets     []cards.Set
	cards    map[string][]cards.Card
}

// Ensure MockCardProvider implements the cards.CardProvider interface

// NewMockCardProvider creates a new mock card provider with optional behavior configuration
func NewMockCardProvider(opts ...MockOption) *MockCardProvider {
	m := &MockCardProvider{
		sets:  generateMockSets(),
		cards: make(map[string][]cards.Card),
	}

	// Apply options
	for _, opt := range opts {
		opt(&m.behavior)
	}

	// Pre-generate cards for default sets
	for _, set := range m.sets {
		m.cards[set.ID] = generateMockCards(set)
	}

	return m
}

// SetMockSets allows customization of the mock sets
func (m *MockCardProvider) SetMockSets(sets []cards.Set) {
	m.sets = sets
}

// SetMockCards allows customization of the mock cards for a specific set
func (m *MockCardProvider) SetMockCards(setID string, cardList []cards.Card) {
	m.cards[setID] = cardList
}

// Available returns true if the mock provider is available
func (m *MockCardProvider) Available() bool {
	return !m.behavior.ShouldError
}

// GetCards fetches all cards from a set (domain interface method)
func (m *MockCardProvider) GetCards(ctx context.Context, setID string) ([]cards.Card, error) {
	if err := m.behavior.checkBehavior(); err != nil {
		return nil, err
	}

	if m.behavior.ReturnEmptyData {
		return []cards.Card{}, nil
	}

	// Get cards - they're already domain cards
	domainCards, exists := m.cards[setID]
	if !exists {
		// Generate cards on demand if set ID not found
		domainCards = generateMockCardsForUnknownSet(setID)
		m.cards[setID] = domainCards
	}

	return domainCards, nil
}

// GetSet fetches set metadata (domain interface method)
func (m *MockCardProvider) GetSet(ctx context.Context, setID string) (*cards.Set, error) {
	if err := m.behavior.checkBehavior(); err != nil {
		return nil, err
	}

	// Find the set by ID
	for _, s := range m.sets {
		if s.ID == setID {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("set not found: %s", setID)
}

// ListAllSets fetches all available sets (domain interface method)
func (m *MockCardProvider) ListAllSets(ctx context.Context) ([]cards.Set, error) {
	if err := m.behavior.checkBehavior(); err != nil {
		return nil, err
	}

	if m.behavior.ReturnEmptyData {
		return []cards.Set{}, nil
	}

	// Return domain sets directly
	return m.sets, nil
}

// SearchCards searches for cards matching the given criteria (domain interface method)
func (m *MockCardProvider) SearchCards(ctx context.Context, criteria cards.SearchCriteria) ([]cards.Card, int, error) {
	if err := m.behavior.checkBehavior(); err != nil {
		return nil, 0, err
	}

	if m.behavior.ReturnEmptyData {
		return []cards.Card{}, 0, nil
	}

	// If ReturnAllCards is set, return all cards without filtering (except Limit)
	if m.behavior.ReturnAllCards {
		var results []cards.Card
		for _, cardList := range m.cards {
			results = append(results, cardList...)
		}

		totalCount := len(results)

		// Apply limit
		if criteria.Limit > 0 && len(results) > criteria.Limit {
			results = results[:criteria.Limit]
		}

		return results, totalCount, nil
	}

	// Normal behavior: search through all cards with filtering
	var results []cards.Card
	for _, cardList := range m.cards {
		for _, card := range cardList {
			// Simple matching logic for testing
			match := false

			if criteria.CardName != "" && containsIgnoreCase(card.Name, criteria.CardName) {
				match = true
			}
			if criteria.SetName != "" && containsIgnoreCase(card.SetName, criteria.SetName) {
				match = true
			}
			if criteria.CardNumber != "" && card.Number == criteria.CardNumber {
				match = true
			}
			if criteria.Query != "" && (containsIgnoreCase(card.Name, criteria.Query) || containsIgnoreCase(card.SetName, criteria.Query)) {
				match = true
			}

			if match {
				results = append(results, card)
			}
		}
	}

	totalCount := len(results)

	// Apply limit
	if criteria.Limit > 0 && len(results) > criteria.Limit {
		results = results[:criteria.Limit]
	}

	return results, totalCount, nil
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// generateMockSets creates a default list of mock sets for testing
func generateMockSets() []cards.Set {
	return []cards.Set{
		{
			ID:          "base",
			Name:        "Base Set",
			Series:      "Base",
			TotalCards:  102,
			ReleaseDate: "1999-01-09",
		},
		{
			ID:          "sv3pt5",
			Name:        "Paldean Fates",
			Series:      "Scarlet & Violet",
			TotalCards:  250,
			ReleaseDate: "2024-01-26",
		},
		{
			ID:          "sv7",
			Name:        "Stellar Crown",
			Series:      "Scarlet & Violet",
			TotalCards:  175,
			ReleaseDate: "2024-09-13",
		},
		{
			ID:          "sv8",
			Name:        "Surging Sparks",
			Series:      "Scarlet & Violet",
			TotalCards:  200,
			ReleaseDate: "2024-11-08",
		},
		{
			ID:          "test-set",
			Name:        "Test Set",
			Series:      "Test",
			TotalCards:  10,
			ReleaseDate: "2024-01-01",
		},
	}
}

// generateMockCards creates mock cards for a given set
func generateMockCards(set cards.Set) []cards.Card {
	// Generate deterministic cards based on set ID
	hash := simpleHash(set.ID)
	numCards := 5 + (hash % 10) // 5-15 cards per set

	cardList := make([]cards.Card, numCards)
	for i := 0; i < numCards; i++ {
		cardNum := fmt.Sprintf("%03d", i+1)
		cardName := fmt.Sprintf("Test Card %d", i+1)

		// Vary card types
		rarity := "Common"
		if i%5 == 0 {
			rarity = "Rare"
		} else if i%3 == 0 {
			rarity = "Uncommon"
		}

		// Add some price data for realism
		basePrice := float64(1 + (i * 2))
		market := basePrice * 1.2

		marketPriceCents := int64(market * 100)

		cardList[i] = cards.Card{
			ID:       fmt.Sprintf("%s-%s", set.ID, cardNum),
			Name:     cardName,
			Number:   cardNum,
			Set:      set.ID,
			SetName:  set.Name,
			Rarity:   rarity,
			Language: "English",
			ImageURL: "",
			MarketPrice: cards.Money{
				Cents:    marketPriceCents,
				Currency: "USD",
			},
		}
	}

	return cardList
}

// generateMockCardsForUnknownSet creates cards for a set ID that wasn't pre-configured
func generateMockCardsForUnknownSet(setID string) []cards.Card {
	return generateMockCards(cards.Set{
		ID:          setID,
		Name:        fmt.Sprintf("Mock Set %s", setID),
		Series:      "Mock",
		TotalCards:  10,
		ReleaseDate: "2024-01-01",
	})
}
