package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// Compile-time interface checks
var _ pricing.PriceRepository = (*MockPriceRepository)(nil)
var _ pricing.APITracker = (*MockPriceRepository)(nil)
var _ pricing.AccessTracker = (*MockPriceRepository)(nil)
var _ pricing.HealthChecker = (*MockPriceRepository)(nil)

// MockPriceRepository implements pricing.PriceRepository for testing
type MockPriceRepository struct {
	mu            sync.Mutex
	StalePrices   []pricing.StalePrice
	BlockedUntil  time.Time
	APIUsage      *pricing.APIUsageStats
	RecordedCalls []pricing.APICallRecord

	// Configurable DeletePricesByCard behavior
	DeletePricesByCardFn func(context.Context, string, string, string) (int64, error)
	DeletePricesCount    int64
	DeletePricesErr      error

	// Configurable GetLatestPricesBySource behavior
	GetLatestPricesBySourceFn func(ctx context.Context, cardName, setName, cardNumber, source string, maxAge time.Duration) (map[string]pricing.PriceEntry, error)
}

func (m *MockPriceRepository) GetStalePrices(_ context.Context, _ string, _ int) ([]pricing.StalePrice, error) {
	return m.StalePrices, nil
}

func (m *MockPriceRepository) IsProviderBlocked(_ context.Context, _ string) (bool, time.Time, error) {
	if m.BlockedUntil.After(time.Now()) {
		return true, m.BlockedUntil, nil
	}
	return false, time.Time{}, nil
}

func (m *MockPriceRepository) GetAPIUsage(_ context.Context, provider string) (*pricing.APIUsageStats, error) {
	if m.APIUsage != nil {
		return m.APIUsage, nil
	}
	return &pricing.APIUsageStats{
		Provider:      provider,
		CallsLastHour: 0,
	}, nil
}

func (m *MockPriceRepository) RecordAPICall(_ context.Context, call *pricing.APICallRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RecordedCalls = append(m.RecordedCalls, *call)
	return nil
}

func (m *MockPriceRepository) StorePrice(_ context.Context, _ *pricing.PriceEntry) error {
	return nil
}

func (m *MockPriceRepository) GetLatestPrice(_ context.Context, _ pricing.Card, _ string, _ string) (*pricing.PriceEntry, error) {
	return nil, nil
}

func (m *MockPriceRepository) UpdateRateLimit(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (m *MockPriceRepository) RecordCardAccess(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *MockPriceRepository) CleanupOldAccessLogs(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

func (m *MockPriceRepository) GetLatestPricesBySource(ctx context.Context, cardName, setName, cardNumber, source string, maxAge time.Duration) (map[string]pricing.PriceEntry, error) {
	if m.GetLatestPricesBySourceFn != nil {
		return m.GetLatestPricesBySourceFn(ctx, cardName, setName, cardNumber, source, maxAge)
	}
	return nil, nil
}

func (m *MockPriceRepository) DeletePricesByCard(ctx context.Context, cardName, setName, cardNumber string) (int64, error) {
	if m.DeletePricesByCardFn != nil {
		return m.DeletePricesByCardFn(ctx, cardName, setName, cardNumber)
	}
	return m.DeletePricesCount, m.DeletePricesErr
}

func (m *MockPriceRepository) Ping(_ context.Context) error {
	return nil
}

// MockSimplePriceProvider implements pricing.PriceProvider with call tracking for testing
type MockSimplePriceProvider struct {
	mu        sync.Mutex
	callCount int
	available bool
}

// NewMockSimplePriceProvider creates a new mock provider with the given availability
func NewMockSimplePriceProvider(available bool) *MockSimplePriceProvider {
	return &MockSimplePriceProvider{available: available}
}

func (m *MockSimplePriceProvider) GetPrice(_ context.Context, _ pricing.Card) (*pricing.Price, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	return &pricing.Price{}, nil
}

func (m *MockSimplePriceProvider) Available() bool {
	return m.available
}

func (m *MockSimplePriceProvider) Name() string {
	return "mock"
}

func (m *MockSimplePriceProvider) Close() error {
	return nil
}

func (m *MockSimplePriceProvider) LookupCard(_ context.Context, _ string, _ cards.Card) (*pricing.Price, error) {
	return &pricing.Price{}, nil
}

func (m *MockSimplePriceProvider) GetStats(_ context.Context) *pricing.ProviderStats {
	return nil
}

// CallCount returns the number of GetPrice calls made
func (m *MockSimplePriceProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}
