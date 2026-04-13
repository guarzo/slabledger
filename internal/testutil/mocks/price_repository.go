package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// Compile-time interface checks
var _ pricing.APITracker = (*MockDBTracker)(nil)
var _ pricing.AccessTracker = (*MockDBTracker)(nil)
var _ pricing.HealthChecker = (*MockDBTracker)(nil)

// MockDBTracker implements pricing.APITracker, pricing.AccessTracker,
// and pricing.HealthChecker for testing.
type MockDBTracker struct {
	mu            sync.Mutex
	BlockedUntil  time.Time
	APIUsage      *pricing.APIUsageStats
	RecordedCalls []pricing.APICallRecord
	PingError     error
}

func (m *MockDBTracker) IsProviderBlocked(_ context.Context, _ string) (bool, time.Time, error) {
	if m.BlockedUntil.After(time.Now()) {
		return true, m.BlockedUntil, nil
	}
	return false, time.Time{}, nil
}

func (m *MockDBTracker) GetAPIUsage(_ context.Context, provider string) (*pricing.APIUsageStats, error) {
	if m.APIUsage != nil {
		return m.APIUsage, nil
	}
	return &pricing.APIUsageStats{
		Provider:      provider,
		CallsLastHour: 0,
	}, nil
}

func (m *MockDBTracker) RecordAPICall(_ context.Context, call *pricing.APICallRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RecordedCalls = append(m.RecordedCalls, *call)
	return nil
}

func (m *MockDBTracker) UpdateRateLimit(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (m *MockDBTracker) RecordCardAccess(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *MockDBTracker) CleanupOldAccessLogs(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

func (m *MockDBTracker) Ping(_ context.Context) error {
	return m.PingError
}

// MockRefreshCandidateProvider implements pricing.RefreshCandidateProvider for testing.
type MockRefreshCandidateProvider struct {
	Candidates []pricing.RefreshCandidate
	Err        error
}

func (m *MockRefreshCandidateProvider) GetRefreshCandidates(_ context.Context, limit int) ([]pricing.RefreshCandidate, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.Candidates) <= limit {
		return m.Candidates, nil
	}
	return m.Candidates[:limit], nil
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

func (m *MockSimplePriceProvider) LookupCard(_ context.Context, _ string, _ pricing.CardLookup) (*pricing.Price, error) {
	return &pricing.Price{}, nil
}

// CallCount returns the number of GetPrice calls made
func (m *MockSimplePriceProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}
