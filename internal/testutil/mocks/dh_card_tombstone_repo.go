package mocks

import "context"

// DHCardTombstoneRepoMock implements pricing.DHCardTombstoneRepo using the
// Fn-field pattern. Set any *Fn field to override behavior; unset methods
// return safe zero defaults.
type DHCardTombstoneRepoMock struct {
	IsTombstonedFn  func(ctx context.Context, cardID int) (bool, error)
	RecordFailureFn func(ctx context.Context, cardID int, errMsg string) (int, error)
	ClearFn         func(ctx context.Context, cardID int) error
	ClearAllFn      func(ctx context.Context) (int, error)
	CountFn         func(ctx context.Context) (int, error)
}

func (m *DHCardTombstoneRepoMock) IsTombstoned(ctx context.Context, cardID int) (bool, error) {
	if m.IsTombstonedFn != nil {
		return m.IsTombstonedFn(ctx, cardID)
	}
	return false, nil
}

func (m *DHCardTombstoneRepoMock) RecordFailure(ctx context.Context, cardID int, errMsg string) (int, error) {
	if m.RecordFailureFn != nil {
		return m.RecordFailureFn(ctx, cardID, errMsg)
	}
	return 1, nil
}

func (m *DHCardTombstoneRepoMock) Clear(ctx context.Context, cardID int) error {
	if m.ClearFn != nil {
		return m.ClearFn(ctx, cardID)
	}
	return nil
}

func (m *DHCardTombstoneRepoMock) ClearAll(ctx context.Context) (int, error) {
	if m.ClearAllFn != nil {
		return m.ClearAllFn(ctx)
	}
	return 0, nil
}

func (m *DHCardTombstoneRepoMock) Count(ctx context.Context) (int, error) {
	if m.CountFn != nil {
		return m.CountFn(ctx)
	}
	return 0, nil
}
