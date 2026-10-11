package mocks

import (
	"context"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

// ShowPrepTransitionServiceMock is the narrow operational command service double.
type ShowPrepTransitionServiceMock struct {
	ConfirmExistingFn func(context.Context, string, sp.ConfirmExistingCommand) (sp.ConfirmExistingResult, error)
	DeleteListFn      func(context.Context, string, sp.DeleteListCommand) (sp.DeleteListResult, error)
}

func (m *ShowPrepTransitionServiceMock) ConfirmExisting(ctx context.Context, id string, c sp.ConfirmExistingCommand) (sp.ConfirmExistingResult, error) {
	return m.ConfirmExistingFn(ctx, id, c)
}
func (m *ShowPrepTransitionServiceMock) DeleteList(ctx context.Context, id string, c sp.DeleteListCommand) (sp.DeleteListResult, error) {
	return m.DeleteListFn(ctx, id, c)
}
