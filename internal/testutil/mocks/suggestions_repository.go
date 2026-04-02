package mocks

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
)

// Compile-time interface check
var _ intelligence.SuggestionsRepository = (*MockSuggestionsRepository)(nil)

// MockSuggestionsRepository implements intelligence.SuggestionsRepository for testing.
type MockSuggestionsRepository struct {
	mu          sync.Mutex
	Suggestions []intelligence.Suggestion

	StoreSuggestionsFn   func(ctx context.Context, suggestions []intelligence.Suggestion) error
	GetByDateFn          func(ctx context.Context, date string) ([]intelligence.Suggestion, error)
	GetLatestFn          func(ctx context.Context) ([]intelligence.Suggestion, error)
	GetCardSuggestionsFn func(ctx context.Context, cardName, setName string) ([]intelligence.Suggestion, error)
}

// NewMockSuggestionsRepository creates a new MockSuggestionsRepository.
func NewMockSuggestionsRepository() *MockSuggestionsRepository {
	return &MockSuggestionsRepository{}
}

func (m *MockSuggestionsRepository) StoreSuggestions(ctx context.Context, suggestions []intelligence.Suggestion) error {
	if m.StoreSuggestionsFn != nil {
		return m.StoreSuggestionsFn(ctx, suggestions)
	}
	if len(suggestions) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Mirror real implementation: replace all suggestions for the date.
	// Precondition: all suggestions in the slice share the same SuggestionDate.
	date := suggestions[0].SuggestionDate
	n := 0
	for _, s := range m.Suggestions {
		if s.SuggestionDate != date {
			m.Suggestions[n] = s
			n++
		}
	}
	m.Suggestions = append(m.Suggestions[:n], suggestions...)
	return nil
}

func (m *MockSuggestionsRepository) GetByDate(ctx context.Context, date string) ([]intelligence.Suggestion, error) {
	if m.GetByDateFn != nil {
		return m.GetByDateFn(ctx, date)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []intelligence.Suggestion
	for _, s := range m.Suggestions {
		if s.SuggestionDate == date {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *MockSuggestionsRepository) GetLatest(ctx context.Context) ([]intelligence.Suggestion, error) {
	if m.GetLatestFn != nil {
		return m.GetLatestFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Suggestions) == 0 {
		return []intelligence.Suggestion{}, nil
	}
	return m.Suggestions, nil
}

func (m *MockSuggestionsRepository) GetCardSuggestions(ctx context.Context, cardName, setName string) ([]intelligence.Suggestion, error) {
	if m.GetCardSuggestionsFn != nil {
		return m.GetCardSuggestionsFn(ctx, cardName, setName)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []intelligence.Suggestion
	for _, s := range m.Suggestions {
		if s.CardName == cardName && s.SetName == setName {
			result = append(result, s)
		}
	}
	return result, nil
}
