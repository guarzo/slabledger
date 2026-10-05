package mocks

import "context"

type DHInventorySnapshotFetcherMock struct {
	FetchAllInventoryFn func(context.Context) (map[int]string, error)
}

func (m *DHInventorySnapshotFetcherMock) FetchAllInventory(c context.Context) (map[int]string, error) {
	if m.FetchAllInventoryFn != nil {
		return m.FetchAllInventoryFn(c)
	}
	return map[int]string{}, nil
}
