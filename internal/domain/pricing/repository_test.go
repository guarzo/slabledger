package pricing

import (
	"context"
	"time"
)

// Test that interfaces can be implemented (compile-time checks)
type mockRepository struct{}

func (m *mockRepository) StorePrice(ctx context.Context, entry *PriceEntry) error {
	return nil
}

func (m *mockRepository) GetLatestPrice(ctx context.Context, card Card, grade string, source string) (*PriceEntry, error) {
	return nil, nil
}

func (m *mockRepository) GetStalePrices(ctx context.Context, source string, limit int) ([]StalePrice, error) {
	return nil, nil
}

func (m *mockRepository) GetLatestPricesBySource(_ context.Context, _, _, _, _ string, _ time.Duration) (map[string]PriceEntry, error) {
	return nil, nil
}

func (m *mockRepository) DeletePricesByCard(_ context.Context, _, _, _ string) (int64, error) {
	return 0, nil
}

func (m *mockRepository) RecordAPICall(ctx context.Context, call *APICallRecord) error {
	return nil
}

func (m *mockRepository) GetAPIUsage(ctx context.Context, provider string) (*APIUsageStats, error) {
	return nil, nil
}

func (m *mockRepository) UpdateRateLimit(ctx context.Context, provider string, blockedUntil time.Time) error {
	return nil
}

func (m *mockRepository) IsProviderBlocked(ctx context.Context, provider string) (bool, time.Time, error) {
	return false, time.Time{}, nil
}

func (m *mockRepository) RecordCardAccess(ctx context.Context, cardName, setName, accessType string) error {
	return nil
}

func (m *mockRepository) CleanupOldAccessLogs(ctx context.Context, retentionDays int) (int64, error) {
	return 0, nil
}

func (m *mockRepository) Ping(ctx context.Context) error {
	return nil
}

// Compile-time interface checks
var _ PriceRepository = (*mockRepository)(nil)
var _ APITracker = (*mockRepository)(nil)
var _ AccessTracker = (*mockRepository)(nil)
var _ HealthChecker = (*mockRepository)(nil)
