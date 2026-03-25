package mocks

import (
	"context"
	"fmt"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// MockPriceProvider implements a mock price provider for testing
type MockPriceProvider struct {
	behavior MockBehavior
	matches  map[string]*pricing.Price // Cache of mock matches
}

// NewMockPriceProvider creates a new mock price provider with optional behavior configuration
func NewMockPriceProvider(opts ...MockOption) *MockPriceProvider {
	m := &MockPriceProvider{
		matches: make(map[string]*pricing.Price),
	}

	// Apply options
	for _, opt := range opts {
		opt(&m.behavior)
	}

	return m
}

// Available returns true if the mock provider is available
func (m *MockPriceProvider) Available() bool {
	return !m.behavior.ShouldError
}

// Name returns the provider name
func (m *MockPriceProvider) Name() string {
	return "MockPriceProvider"
}

// GetPrice returns mock price for a card (domain interface method)
func (m *MockPriceProvider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
	if err := m.behavior.checkBehavior(); err != nil {
		return nil, err
	}

	if m.behavior.ReturnEmptyData {
		return nil, fmt.Errorf("no price found")
	}

	// Generate mock price
	hash := simpleHash(card.Name + card.Number)
	basePrice := int64(1000 + (hash % 10000)) // $10-$110 in cents

	return &pricing.Price{
		Amount:   basePrice,
		Currency: "USD",
		Source:   pricing.SourcePriceCharting,
	}, nil
}

// Close is a no-op for the mock provider
func (m *MockPriceProvider) Close() error {
	return nil
}

// LookupCard returns mock price data for a card.
// The context parameter enables request cancellation and timeout propagation.
func (m *MockPriceProvider) LookupCard(ctx context.Context, setName string, card domainCards.Card) (*pricing.Price, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := m.behavior.checkBehavior(); err != nil {
		return nil, err
	}

	if m.behavior.ReturnEmptyData {
		return nil, fmt.Errorf("no match found")
	}

	// Check if we have a pre-configured match
	key := fmt.Sprintf("%s:%s:%s", setName, card.Name, card.Number)
	if match, exists := m.matches[key]; exists {
		return match, nil
	}

	// Generate deterministic mock data
	return generateMockPrice(setName, card), nil
}

// GetStats returns mock provider statistics
func (m *MockPriceProvider) GetStats(_ context.Context) *pricing.ProviderStats {
	return &pricing.ProviderStats{}
}

// SetMockMatch allows customization of mock matches for specific cards
func (m *MockPriceProvider) SetMockMatch(setName, cardName, number string, match *pricing.Price) {
	key := fmt.Sprintf("%s:%s:%s", setName, cardName, number)
	m.matches[key] = match
}

// generateMockPrice creates a mock Price with realistic price data
func generateMockPrice(setName string, card domainCards.Card) *pricing.Price {
	// Generate deterministic but varied prices based on card name
	hash := simpleHash(card.Name + card.Number)
	basePrice := int64(10 + (hash % 100)) // Base price between $10-$110

	// Convert to cents
	rawPrice := basePrice * 100
	psa10Price := rawPrice * 5 // PSA 10 is 5x raw price
	psa9Price := rawPrice * 3  // PSA 9 is 3x raw price
	bgs95Price := rawPrice * 4 // BGS 9.5 is 4x raw price

	return &pricing.Price{
		ID:          fmt.Sprintf("mock-%d", hash),
		ProductName: fmt.Sprintf("Pokemon %s %s #%s", setName, card.Name, card.Number),
		Amount:      psa10Price,
		Currency:    "USD",
		Source:      pricing.SourcePriceCharting,

		// Price fields in cents
		Grades: pricing.GradedPrices{
			RawCents:     rawPrice,
			PSA10Cents:   psa10Price,
			PSA9Cents:    psa9Price,
			Grade95Cents: bgs95Price,
			BGS10Cents:   psa10Price + 1000, // BGS 10 slightly higher
		},
	}
}
