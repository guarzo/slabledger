package scheduler

import "context"

// mockSyncStateStore implements SyncStateStore for testing.
// This mock lives here (not in testutil/mocks) because SyncStateStore is defined
// in the scheduler package and moving it would create a circular import.
type mockSyncStateStore struct {
	GetFn  func(ctx context.Context, key string) (string, error)
	SetFn  func(ctx context.Context, key, value string) error
	values map[string]string
}

func newMockSyncStateStore() *mockSyncStateStore {
	return &mockSyncStateStore{values: make(map[string]string)}
}

func (m *mockSyncStateStore) Get(ctx context.Context, key string) (string, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, key)
	}
	return m.values[key], nil
}

func (m *mockSyncStateStore) Set(ctx context.Context, key, value string) error {
	if m.SetFn != nil {
		return m.SetFn(ctx, key, value)
	}
	m.values[key] = value
	return nil
}
