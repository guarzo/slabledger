package scheduler

import "context"

// mockSyncStateStore implements SyncStateStore for testing.
type mockSyncStateStore struct {
	values map[string]string
	getErr error
	setErr error
}

func newMockSyncStateStore() *mockSyncStateStore {
	return &mockSyncStateStore{values: make(map[string]string)}
}

func (m *mockSyncStateStore) Get(_ context.Context, key string) (string, error) {
	return m.values[key], m.getErr
}

func (m *mockSyncStateStore) Set(_ context.Context, key, value string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.values[key] = value
	return nil
}
