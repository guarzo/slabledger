package mocks

import (
	"context"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

func (m *ShowPrepStoreMock) ReadCertPurchaseIDs(ctx context.Context, cert string) ([]string, error) {
	if m.ReadCertPurchaseIDsFn != nil {
		return m.ReadCertPurchaseIDsFn(ctx, cert)
	}
	return []string{}, nil
}
func (m *ShowPrepStoreMock) CertCollisionObserved(cert string) bool {
	return m.CertCollisionObservedFn != nil && m.CertCollisionObservedFn(cert)
}
func (m *ShowPrepStoreMock) GetCommandReceipt(ctx context.Context, id string) (sp.CommandReceipt, bool, error) {
	if m.GetCommandReceiptFn != nil {
		return m.GetCommandReceiptFn(ctx, id)
	}
	return sp.CommandReceipt{}, false, nil
}
func (m *ShowPrepStoreMock) InsertCommandReceipt(ctx context.Context, r sp.CommandReceipt) error {
	if m.InsertCommandReceiptFn != nil {
		return m.InsertCommandReceiptFn(ctx, r)
	}
	return nil
}
func (m *ShowPrepStoreMock) GetRetiredList(ctx context.Context, id string) (sp.RetiredList, bool, error) {
	if m.GetRetiredListFn != nil {
		return m.GetRetiredListFn(ctx, id)
	}
	return sp.RetiredList{}, false, nil
}
func (m *ShowPrepStoreMock) RetireList(ctx context.Context, r sp.RetiredList) error {
	if m.RetireListFn != nil {
		return m.RetireListFn(ctx, r)
	}
	return nil
}
