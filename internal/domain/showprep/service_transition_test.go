package showprep_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func transitionFixture() (sp.List, []sp.Item, map[string]sp.Purchase) {
	list := sp.List{ID: showListID, Name: "Show", WithdrawalGeneration: 3}
	a := scanMember()
	a.ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	a.PackedAt = ""
	b := scanMember()
	b.ID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	b.PurchaseID = "22222222-2222-4222-8222-222222222222"
	b.CertNumber = "87654321"
	p := scanPurchase()
	p.LocalPriceCents = 0
	q := scanPurchase()
	q.ID = b.PurchaseID
	q.CertNumber = b.CertNumber
	q.LocalPriceCents = 40000
	return list, []sp.Item{b, a}, map[string]sp.Purchase{p.ID: p, q.ID: q}
}
func transitionMock(list sp.List, items *[]sp.Item, ps map[string]sp.Purchase, receipts *[]sp.CommandReceipt) *mocks.ShowPrepStoreMock {
	return &mocks.ShowPrepStoreMock{
		GetListFn:       func(context.Context, string) (sp.List, error) { return list, nil },
		GetItemsFn:      func(context.Context, string) ([]sp.Item, error) { return slices.Clone(*items), nil },
		ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) { return ps, nil },
		ReadCertPurchaseIDsFn: func(_ context.Context, cert string) ([]string, error) {
			ids := []string{}
			for id, p := range ps {
				if p.CertNumber == cert {
					ids = append(ids, id)
				}
			}
			return ids, nil
		},
		SaveItemFn: func(_ context.Context, _ string, item sp.Item) error {
			for j := range *items {
				if (*items)[j].ID == item.ID {
					(*items)[j] = item
					return nil
				}
			}
			return sp.ErrNotFound
		},
		InsertCommandReceiptFn: func(_ context.Context, r sp.CommandReceipt) error { *receipts = append(*receipts, r); return nil },
	}
}
func transitionCommand(list sp.List, items []sp.Item) sp.ConfirmExistingCommand {
	ids := make([]string, 0, len(items))
	for _, i := range items {
		ids = append(ids, i.ID)
	}
	return sp.ConfirmExistingCommand{CommandID: commandID, ExpectedState: sp.ListStateFingerprint(list, items), ItemIDs: ids}
}

// Catches partial-cohort updates, new packing timestamps overwriting history, and price gating.
func TestConfirmExistingExactCohort(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{true: "empty", false: "all ready"}[empty], func(t *testing.T) {
			list, items, ps := transitionFixture()
			if empty {
				items = []sp.Item{}
			}
			before := slices.Clone(items)
			receipts := []sp.CommandReceipt{}
			m := transitionMock(list, &items, ps, &receipts)
			m.LockPurchasesFn = func(_ context.Context, ids []string) error {
				want := []string{}
				if !empty {
					want = []string{scanPurchaseID, "22222222-2222-4222-8222-222222222222"}
				}
				require.Equal(t, want, ids)
				return nil
			}
			cmd := transitionCommand(list, items)
			result, err := sp.NewService(m, nil, func() time.Time { return fixedScanNow }).ConfirmExisting(context.Background(), showListID, cmd)
			require.NoError(t, err)
			require.False(t, result.Replay)
			require.Equal(t, showListID, result.ListID)
			require.Equal(t, commandID, result.CommandID)
			want := []string{}
			if !empty {
				want = []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}
			}
			require.Equal(t, want, result.ConfirmedItemIDs)
			require.Len(t, receipts, 1)
			require.Equal(t, "confirm_existing", receipts[0].Kind)
			require.Equal(t, result, *receipts[0].Transition)
			for j, i := range items {
				require.Equal(t, before[j].AddedAt, i.AddedAt)
				require.Equal(t, before[j].Version+1, i.Version)
				require.NotEmpty(t, i.LastCommand)
				if before[j].PackedAt != "" {
					require.Equal(t, before[j].PackedAt, i.PackedAt)
				} else {
					require.Equal(t, "2026-10-09T12:00:00Z", i.PackedAt)
				}
				require.Equal(t, ps[i.PurchaseID].LocalPriceCents, i.AcknowledgedPriceCents)
				if i.PurchaseID == scanPurchaseID {
					require.Equal(t, sp.NoListedPrice, i.AcknowledgedStatus)
				} else {
					require.Equal(t, sp.NeedsReview, i.AcknowledgedStatus)
				}
			}
		})
	}
}

func TestConfirmExistingRejectsWholeCohort(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*sp.List, *[]sp.Item, map[string]sp.Purchase, *mocks.ShowPrepStoreMock, *sp.ConfirmExistingCommand)
	}{
		{"stale rename", func(l *sp.List, _ *[]sp.Item, _ map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			l.Name = "Renamed"
		}},
		{"stale version", func(_ *sp.List, i *[]sp.Item, _ map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			(*i)[0].Version++
		}},
		{"stale withdrawal", func(l *sp.List, _ *[]sp.Item, _ map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			l.WithdrawalGeneration++
		}},
		{"omitted member", func(_ *sp.List, _ *[]sp.Item, _ map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, c *sp.ConfirmExistingCommand) {
			c.ItemIDs = c.ItemIDs[:1]
		}},
		{"new member", func(_ *sp.List, i *[]sp.Item, _ map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := (*i)[0]
			x.ID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			*i = append(*i, x)
		}},
		{"sold", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.Sold = true
			p[x.ID] = x
		}},
		{"not received", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.Received = false
			p[x.ID] = x
		}},
		{"refunded", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.Refunded = true
			p[x.ID] = x
		}},
		{"closed", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.Phase = "closed"
			p[x.ID] = x
		}},
		{"unknown", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.Known = false
			p[x.ID] = x
		}},
		{"removed", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			delete(p, scanPurchaseID)
		}},
		{"cert changed", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.CertNumber = "different"
			p[x.ID] = x
		}},
		{"grader changed", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.Grader = "BGS"
			p[x.ID] = x
		}},
		{"grade changed", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.Grade = 9
			p[x.ID] = x
		}},
		{"duplicate live cert", func(_ *sp.List, _ *[]sp.Item, p map[string]sp.Purchase, _ *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			x := p[scanPurchaseID]
			x.ID = "competitor"
			p[x.ID] = x
		}},
		{"latched collision", func(_ *sp.List, _ *[]sp.Item, _ map[string]sp.Purchase, m *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			m.CertCollisionObservedFn = func(cert string) bool { return cert == "12345678" }
		}},
		{"late live collision", func(_ *sp.List, _ *[]sp.Item, _ map[string]sp.Purchase, m *mocks.ShowPrepStoreMock, _ *sp.ConfirmExistingCommand) {
			reads := 0
			old := m.ReadCertPurchaseIDsFn
			m.ReadCertPurchaseIDsFn = func(ctx context.Context, cert string) ([]string, error) {
				reads++
				ids, err := old(ctx, cert)
				if reads > 2 && cert == "12345678" {
					ids = append(ids, "competitor")
				}
				return ids, err
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			list, items, ps := transitionFixture()
			cmd := transitionCommand(list, items)
			receipts := []sp.CommandReceipt{}
			m := transitionMock(list, &items, ps, &receipts)
			tt.change(&list, &items, ps, m, &cmd)
			m.GetListFn = func(context.Context, string) (sp.List, error) { return list, nil }
			before := slices.Clone(items)
			m.SaveItemFn = func(context.Context, string, sp.Item) error {
				t.Fatal("validate the entire cohort before any save")
				return nil
			}
			_, err := sp.NewService(m, nil, nil).ConfirmExisting(context.Background(), showListID, cmd)
			require.ErrorIs(t, err, sp.ErrConflict)
			require.Empty(t, receipts)
			require.Equal(t, before, items)
		})
	}
}

func TestConfirmExistingReceiptFirstReplay(t *testing.T) {
	for _, change := range []string{"price and removal", "changed state payload", "changed cohort payload", "kind", "destination"} {
		t.Run(change, func(t *testing.T) {
			list, items, ps := transitionFixture()
			cmd := transitionCommand(list, items)
			receipts := []sp.CommandReceipt{}
			m := transitionMock(list, &items, ps, &receipts)
			svc := sp.NewService(m, nil, nil)
			first, err := svc.ConfirmExisting(context.Background(), showListID, cmd)
			require.NoError(t, err)
			r := receipts[0]
			items = nil
			m.GetCommandReceiptFn = func(context.Context, string) (sp.CommandReceipt, bool, error) { return r, true, nil }
			m.GetListFn = func(context.Context, string) (sp.List, error) {
				t.Fatal("receipt first")
				return sp.List{}, sp.ErrNotFound
			}
			m.SaveItemFn = func(context.Context, string, sp.Item) error { t.Fatal("replay cannot mutate"); return nil }
			m.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) {
				t.Fatal("replay cannot observe")
				return nil, nil
			}
			switch change {
			case "changed state payload":
				cmd.ExpectedState = "changed"
			case "changed cohort payload":
				cmd.ItemIDs = cmd.ItemIDs[:1]
			case "kind":
				r.Kind = "scan"
			case "destination":
				r.ListID = "other"
			default:
				slices.Reverse(cmd.ItemIDs)
			}
			replay, err := svc.ConfirmExisting(context.Background(), showListID, cmd)
			if change != "price and removal" {
				require.ErrorIs(t, err, sp.ErrConflict)
			} else {
				require.NoError(t, err)
				first.Replay = true
				require.Equal(t, first, replay)
			}
			require.Len(t, receipts, 1)
		})
	}
}

func TestConfirmExistingCohortValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		ids  []string
	}{
		{"nil cohort", nil}, {"non UUID member", []string{"bad"}},
		{"duplicate member", []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &mocks.ShowPrepStoreMock{WithinFn: func(context.Context, func(sp.Session) error) error {
				t.Fatal("invalid cohort cannot transact")
				return nil
			}}
			_, err := sp.NewService(m, nil, nil).ConfirmExisting(context.Background(), showListID, sp.ConfirmExistingCommand{CommandID: commandID, ExpectedState: "state", ItemIDs: tt.ids})
			require.ErrorIs(t, err, sp.ErrInvalid)
		})
	}
}

func TestConfirmExistingStorageFailures(t *testing.T) {
	failure := errors.New("storage unavailable")
	for _, tt := range []struct {
		name   string
		want   error
		change func(*mocks.ShowPrepStoreMock)
	}{
		{"receipt read", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetCommandReceiptFn = func(context.Context, string) (sp.CommandReceipt, bool, error) {
				return sp.CommandReceipt{}, false, failure
			}
		}},
		{"list read", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetListFn = func(context.Context, string) (sp.List, error) { return sp.List{}, failure }
		}},
		{"missing show", sp.ErrNotFound, func(m *mocks.ShowPrepStoreMock) {
			m.GetListFn = func(context.Context, string) (sp.List, error) { return sp.List{}, sp.ErrNotFound }
		}},
		{"retired show", sp.ErrRetired, func(m *mocks.ShowPrepStoreMock) {
			m.GetListFn = func(context.Context, string) (sp.List, error) { return sp.List{}, sp.ErrNotFound }
			m.GetRetiredListFn = func(context.Context, string) (sp.RetiredList, bool, error) {
				return sp.RetiredList{ListID: showListID}, true, nil
			}
		}},
		{"items read", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetItemsFn = func(context.Context, string) ([]sp.Item, error) { return nil, failure }
		}},
		{"contention", sp.ErrContended, func(m *mocks.ShowPrepStoreMock) {
			m.LockPurchasesFn = func(context.Context, []string) error { return sp.ErrContended }
		}},
		{"purchase", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) { return nil, failure }
		}},
		{"evidence", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ReadSnapshotsFn = func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) { return nil, failure }
		}},
		{"observation", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) { return nil, failure }
		}},
		{"cert", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) { return nil, failure }
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			list, items, ps := transitionFixture()
			receipts := []sp.CommandReceipt{}
			m := transitionMock(list, &items, ps, &receipts)
			tt.change(m)
			before := slices.Clone(items)
			_, err := sp.NewService(m, nil, nil).ConfirmExisting(context.Background(), showListID, transitionCommand(list, items))
			require.ErrorIs(t, err, tt.want)
			require.Empty(t, receipts)
			require.Equal(t, before, items)
		})
	}
}
