package mocks

import "context"

type DHInventoryRecoveryMock struct {
	RecoverFn func(context.Context, string, string, string) error
}

func (m *DHInventoryRecoveryMock) RecoverDHInventory(ctx context.Context, id, action, actor string) error {
	return m.RecoverFn(ctx, id, action, actor)
}
