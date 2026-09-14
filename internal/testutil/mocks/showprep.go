package mocks

import (
	"context"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"time"
)

type ShowPrepSourceMock struct {
	FetchFn func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error)
}

func (m *ShowPrepSourceMock) Fetch(c context.Context, i sp.Identity, t time.Time) (sp.Snapshot, error) {
	return m.FetchFn(c, i, t)
}

type ShowPrepStoreMock struct {
	ReadPurchasesFn            func(context.Context, []string) (map[string]sp.Purchase, error)
	ObservePriceAssociationsFn func(context.Context, []string) (map[string]bool, error)
	ReadSnapshotsFn            func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error)
	BeginAttemptFn             func(context.Context, sp.Identity, time.Time) (int64, error)
	FinishAttemptFn            func(context.Context, sp.Identity, int64, sp.Snapshot) error
	ListsFn                    func(context.Context) ([]sp.List, error)
	CreateListFn               func(context.Context, string, string) (sp.List, error)
	RenameListFn               func(context.Context, string, string) (sp.List, error)
	GetListFn                  func(context.Context, string) (sp.List, error)
	GetItemsFn                 func(context.Context, string) ([]sp.Item, error)
	RemoveItemFn               func(context.Context, string, string) error
	WithinFn                   func(context.Context, func(sp.Session) error) error
	LockPurchasesFn            func(context.Context, []string) error
	InsertItemFn               func(context.Context, string, sp.Item) error
	SaveItemFn                 func(context.Context, string, sp.Item) error
}

var _ sp.Store = (*ShowPrepStoreMock)(nil)
var _ sp.Session = (*ShowPrepStoreMock)(nil)

func (m *ShowPrepStoreMock) ReadPurchases(c context.Context, ids []string) (map[string]sp.Purchase, error) {
	if m.ReadPurchasesFn != nil {
		return m.ReadPurchasesFn(c, ids)
	}
	return map[string]sp.Purchase{}, nil
}
func (m *ShowPrepStoreMock) ObservePriceAssociations(c context.Context, ids []string) (map[string]bool, error) {
	if m.ObservePriceAssociationsFn != nil {
		return m.ObservePriceAssociationsFn(c, ids)
	}
	return map[string]bool{}, nil
}
func (m *ShowPrepStoreMock) ReadSnapshots(c context.Context, i []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
	if m.ReadSnapshotsFn != nil {
		return m.ReadSnapshotsFn(c, i)
	}
	return map[sp.Identity]*sp.Snapshot{}, nil
}
func (m *ShowPrepStoreMock) BeginAttempt(c context.Context, i sp.Identity, t time.Time) (int64, error) {
	if m.BeginAttemptFn != nil {
		return m.BeginAttemptFn(c, i, t)
	}
	return 1, nil
}
func (m *ShowPrepStoreMock) FinishAttempt(c context.Context, i sp.Identity, n int64, s sp.Snapshot) error {
	if m.FinishAttemptFn != nil {
		return m.FinishAttemptFn(c, i, n, s)
	}
	return nil
}
func (m *ShowPrepStoreMock) Lists(c context.Context) ([]sp.List, error) {
	if m.ListsFn != nil {
		return m.ListsFn(c)
	}
	return []sp.List{}, nil
}
func (m *ShowPrepStoreMock) CreateList(c context.Context, i, n string) (sp.List, error) {
	if m.CreateListFn != nil {
		return m.CreateListFn(c, i, n)
	}
	return sp.List{}, nil
}
func (m *ShowPrepStoreMock) RenameList(c context.Context, i, n string) (sp.List, error) {
	if m.RenameListFn != nil {
		return m.RenameListFn(c, i, n)
	}
	return sp.List{}, nil
}
func (m *ShowPrepStoreMock) GetList(c context.Context, i string) (sp.List, error) {
	if m.GetListFn != nil {
		return m.GetListFn(c, i)
	}
	return sp.List{ID: i}, nil
}
func (m *ShowPrepStoreMock) GetItems(c context.Context, i string) ([]sp.Item, error) {
	if m.GetItemsFn != nil {
		return m.GetItemsFn(c, i)
	}
	return []sp.Item{}, nil
}
func (m *ShowPrepStoreMock) RemoveItem(c context.Context, l, i string) error {
	if m.RemoveItemFn != nil {
		return m.RemoveItemFn(c, l, i)
	}
	return nil
}
func (m *ShowPrepStoreMock) Within(c context.Context, fn func(sp.Session) error) error {
	if m.WithinFn != nil {
		return m.WithinFn(c, fn)
	}
	return fn(m)
}
func (m *ShowPrepStoreMock) LockPurchases(c context.Context, ids []string) error {
	if m.LockPurchasesFn != nil {
		return m.LockPurchasesFn(c, ids)
	}
	return nil
}
func (m *ShowPrepStoreMock) InsertItem(c context.Context, l string, i sp.Item) error {
	if m.InsertItemFn != nil {
		return m.InsertItemFn(c, l, i)
	}
	return nil
}
func (m *ShowPrepStoreMock) SaveItem(c context.Context, l string, i sp.Item) error {
	if m.SaveItemFn != nil {
		return m.SaveItemFn(c, l, i)
	}
	return nil
}
