package showprep_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestScanConfirmReplay(t *testing.T) {
	for _, tt := range []struct {
		name, state string
		change      func(*mocks.ShowPrepStoreMock, *[]sp.Item, *sp.Purchase)
	}{
		{"changed price", "present", func(_ *mocks.ShowPrepStoreMock, _ *[]sp.Item, p *sp.Purchase) { p.LocalPriceCents = 50000 }},
		{"padded member grader", "present", func(_ *mocks.ShowPrepStoreMock, i *[]sp.Item, _ *sp.Purchase) { (*i)[0].Grader = " PSA " }},
		{"removed", "removed", func(_ *mocks.ShowPrepStoreMock, i *[]sp.Item, _ *sp.Purchase) { *i = nil }},
		{"readded", "removed", func(_ *mocks.ShowPrepStoreMock, i *[]sp.Item, _ *sp.Purchase) { (*i)[0].ID = "new-member" }},
		{"sold", "unavailable", func(_ *mocks.ShowPrepStoreMock, _ *[]sp.Item, p *sp.Purchase) { p.Sold = true }},
		{"identity changed", "unavailable", func(_ *mocks.ShowPrepStoreMock, _ *[]sp.Item, p *sp.Purchase) { p.Grader = "BGS" }},
		{"missing purchase", "unavailable", func(m *mocks.ShowPrepStoreMock, _ *[]sp.Item, _ *sp.Purchase) {
			m.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) { return nil, nil }
		}},
		{"unreadable purchase", "unknown", func(m *mocks.ShowPrepStoreMock, _ *[]sp.Item, _ *sp.Purchase) {
			m.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) { return nil, errors.New("read failed") }
		}},
		{"unreadable items", "unknown", func(m *mocks.ShowPrepStoreMock, _ *[]sp.Item, _ *sp.Purchase) {
			m.GetItemsFn = func(context.Context, string) ([]sp.Item, error) { return nil, errors.New("read failed") }
		}},
		{"unknown availability", "unknown", func(_ *mocks.ShowPrepStoreMock, _ *[]sp.Item, p *sp.Purchase) { p.Known = false }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := []sp.Item{scanMember()}
			receipts := []sp.CommandReceipt{}
			p := scanPurchase()
			store := scanStore(p, &items, &receipts)
			original, err := sp.NewService(store, nil, func() time.Time { return fixedScanNow }).ScanConfirm(context.Background(), showListID, scanCommand())
			require.NoError(t, err)
			store.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
				return map[string]sp.Purchase{p.ID: p}, nil
			}
			tt.change(store, &items, &p)
			store.GetCommandReceiptFn = func(context.Context, string) (sp.CommandReceipt, bool, error) { return receipts[0], true, nil }
			store.GetListFn = func(context.Context, string) (sp.List, error) {
				t.Fatal("replay must precede missing list/generation checks")
				return sp.List{}, sp.ErrNotFound
			}
			store.LockPurchasesFn = func(context.Context, []string) error { t.Fatal("replay cannot acquire purchase locks"); return nil }
			store.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) {
				t.Fatal("replay cannot resolve a new candidate")
				return nil, nil
			}
			store.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) {
				t.Fatal("replay cannot write holds")
				return nil, nil
			}
			store.ReadSnapshotsFn = func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				t.Fatal("replay does not need evidence")
				return nil, nil
			}
			store.InsertItemFn = func(context.Context, string, sp.Item) error { t.Fatal("replay cannot insert"); return nil }
			store.SaveItemFn = func(context.Context, string, sp.Item) error { t.Fatal("replay cannot save"); return nil }
			store.InsertCommandReceiptFn = func(context.Context, sp.CommandReceipt) error { t.Fatal("replay cannot insert receipt"); return nil }
			result, err := sp.NewService(store, nil, nil).ScanConfirm(context.Background(), showListID, scanCommand())
			require.NoError(t, err)
			require.True(t, result.Replay)
			require.Equal(t, original.Original, result.Original)
			require.Equal(t, tt.state, result.Current.State)
			require.Len(t, receipts, 1)
		})
	}
}

func TestScanConfirmPayloadMismatch(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*sp.CommandReceipt)
	}{
		{"payload", func(r *sp.CommandReceipt) { r.PayloadHash = "other" }},
		{"destination", func(r *sp.CommandReceipt) { r.ListID = "other" }},
		{"kind", func(r *sp.CommandReceipt) { r.Kind = "confirm_existing" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := []sp.Item{}
			receipts := []sp.CommandReceipt{}
			store := scanStore(scanPurchase(), &items, &receipts)
			_, err := sp.NewService(store, nil, nil).ScanConfirm(context.Background(), showListID, scanCommand())
			require.NoError(t, err)
			r := receipts[0]
			tt.change(&r)
			store.GetCommandReceiptFn = func(context.Context, string) (sp.CommandReceipt, bool, error) { return r, true, nil }
			store.GetListFn = func(context.Context, string) (sp.List, error) {
				t.Fatal("mismatch must precede list read")
				return sp.List{}, nil
			}
			_, err = sp.NewService(store, nil, nil).ScanConfirm(context.Background(), showListID, scanCommand())
			require.ErrorIs(t, err, sp.ErrConflict)
			require.Len(t, receipts, 1)
		})
	}
}

func TestScanConfirmValidation(t *testing.T) {
	for _, tt := range []struct {
		name, list string
		cmd        sp.ScanCommand
	}{
		{"list UUID", "invalid", scanCommand()},
		{"command UUID", showListID, sp.ScanCommand{CommandID: "invalid", CertNumber: "12345678"}},
		{"empty cert", showListID, sp.ScanCommand{CommandID: commandID}},
		{"padded cert", showListID, sp.ScanCommand{CommandID: commandID, CertNumber: " 12345678 "}},
		{"negative generation", showListID, sp.ScanCommand{CommandID: commandID, CertNumber: "12345678", WithdrawalGeneration: -1}},
		{"unsafe generation", showListID, sp.ScanCommand{CommandID: commandID, CertNumber: "12345678", WithdrawalGeneration: 9007199254740992}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.ShowPrepStoreMock{WithinFn: func(context.Context, func(sp.Session) error) error {
				t.Fatal("invalid input must not enter transaction")
				return nil
			}}
			_, err := sp.NewService(store, nil, nil).ScanConfirm(context.Background(), tt.list, tt.cmd)
			require.ErrorIs(t, err, sp.ErrInvalid)
		})
	}
}
