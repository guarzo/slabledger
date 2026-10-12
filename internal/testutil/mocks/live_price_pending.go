package mocks

import "context"

func (m *PurchaseRepositoryMock) ListPendingDHLivePrices(ctx context.Context) ([]string, error) {
	if m.ListPendingDHLivePricesFn != nil {
		return m.ListPendingDHLivePricesFn(ctx)
	}
	return nil, nil
}
