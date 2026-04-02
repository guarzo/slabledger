package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
)

// Compile-time interface check
var _ intelligence.Repository = (*MockIntelligenceRepository)(nil)

// MockIntelligenceRepository implements intelligence.Repository for testing.
type MockIntelligenceRepository struct {
	mu    sync.Mutex
	store map[string]*intelligence.MarketIntelligence

	StoreFn         func(ctx context.Context, intel *intelligence.MarketIntelligence) error
	GetByCardFn     func(ctx context.Context, cardName, setName, cardNumber string) (*intelligence.MarketIntelligence, error)
	GetByCardsFn    func(ctx context.Context, keys []intelligence.CardKey) (map[intelligence.CardKey]*intelligence.MarketIntelligence, error)
	GetByDHCardIDFn func(ctx context.Context, dhCardID string) (*intelligence.MarketIntelligence, error)
	GetStaleFn      func(ctx context.Context, maxAge time.Duration, limit int) ([]intelligence.MarketIntelligence, error)
}

// NewMockIntelligenceRepository creates a new MockIntelligenceRepository with an initialised in-memory store.
func NewMockIntelligenceRepository() *MockIntelligenceRepository {
	return &MockIntelligenceRepository{
		store: make(map[string]*intelligence.MarketIntelligence),
	}
}

func (m *MockIntelligenceRepository) Store(ctx context.Context, intel *intelligence.MarketIntelligence) error {
	if m.StoreFn != nil {
		return m.StoreFn(ctx, intel)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := intel.CardName + "|" + intel.SetName + "|" + intel.CardNumber
	m.store[key] = intel
	return nil
}

func (m *MockIntelligenceRepository) GetByCard(ctx context.Context, cardName, setName, cardNumber string) (*intelligence.MarketIntelligence, error) {
	if m.GetByCardFn != nil {
		return m.GetByCardFn(ctx, cardName, setName, cardNumber)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := cardName + "|" + setName + "|" + cardNumber
	return m.store[key], nil
}

func (m *MockIntelligenceRepository) GetByCards(ctx context.Context, keys []intelligence.CardKey) (map[intelligence.CardKey]*intelligence.MarketIntelligence, error) {
	if m.GetByCardsFn != nil {
		return m.GetByCardsFn(ctx, keys)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[intelligence.CardKey]*intelligence.MarketIntelligence)
	for _, k := range keys {
		key := k.CardName + "|" + k.SetName + "|" + k.CardNumber
		if intel, ok := m.store[key]; ok {
			result[k] = intel
		}
	}
	return result, nil
}

func (m *MockIntelligenceRepository) GetByDHCardID(ctx context.Context, dhCardID string) (*intelligence.MarketIntelligence, error) {
	if m.GetByDHCardIDFn != nil {
		return m.GetByDHCardIDFn(ctx, dhCardID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, intel := range m.store {
		if intel.DHCardID == dhCardID {
			return intel, nil
		}
	}
	return nil, nil
}

func (m *MockIntelligenceRepository) GetStale(ctx context.Context, maxAge time.Duration, limit int) ([]intelligence.MarketIntelligence, error) {
	if m.GetStaleFn != nil {
		return m.GetStaleFn(ctx, maxAge, limit)
	}
	return []intelligence.MarketIntelligence{}, nil
}
