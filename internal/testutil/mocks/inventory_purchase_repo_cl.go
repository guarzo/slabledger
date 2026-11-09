package mocks

import "context"

func (m *PurchaseRepositoryMock) UpdatePurchaseCLError(ctx context.Context, id, reason, at string) error {
	if m.UpdatePurchaseCLErrorFn != nil {
		return m.UpdatePurchaseCLErrorFn(ctx, id, reason, at)
	}
	return nil
}
func (m *PurchaseRepositoryMock) UpdatePurchaseGemRateID(ctx context.Context, id, profile string) error {
	if m.UpdatePurchaseGemRateIDFn != nil {
		return m.UpdatePurchaseGemRateIDFn(ctx, id, profile)
	}
	return nil
}
func (m *PurchaseRepositoryMock) UpdatePurchaseCLCardMetadata(ctx context.Context, id, player, variation, category string) error {
	if m.UpdatePurchaseCLCardMetadataFn != nil {
		return m.UpdatePurchaseCLCardMetadataFn(ctx, id, player, variation, category)
	}
	return nil
}
func (m *PurchaseRepositoryMock) UpdatePurchaseSetName(ctx context.Context, id, set string) error {
	if m.UpdatePurchaseSetNameFn != nil {
		return m.UpdatePurchaseSetNameFn(ctx, id, set)
	}
	return nil
}
func (m *PurchaseRepositoryMock) UpdatePurchasePSASpecID(context.Context, string, int) error {
	return nil
}
