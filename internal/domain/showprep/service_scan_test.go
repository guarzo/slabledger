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

const showListID = "33333333-3333-4333-8333-333333333333"
const commandID = "44444444-4444-4444-8444-444444444444"
const scanPurchaseID = "11111111-1111-4111-8111-111111111111"

var fixedScanNow = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func scanPurchase() sp.Purchase {
	return sp.Purchase{ID: scanPurchaseID, CampaignID: "campaign", CardName: "Card", CertNumber: "12345678", Grader: "PSA", Grade: 10, ProfileID: "profile", Known: true, Exists: true, CampaignExists: true, Phase: "active", Received: true, LocalPriceCents: 30000}
}
func scanMember() sp.Item {
	return sp.Item{ID: "item", PurchaseID: scanPurchaseID, CardName: "Card", CertNumber: "12345678", Grader: "PSA", Grade: 10, AddedAt: "2026-10-01T12:00:00Z", PackedAt: "2026-10-02T12:00:00Z", Version: 4, AcknowledgedPriceCents: 30000, AcknowledgedStatus: sp.NeedsReview, LastCommand: "prior"}
}
func scanCommand() sp.ScanCommand {
	return sp.ScanCommand{CommandID: commandID, CertNumber: "12345678", WithdrawalGeneration: 3}
}
func scanStore(p sp.Purchase, items *[]sp.Item, receipts *[]sp.CommandReceipt) *mocks.ShowPrepStoreMock {
	return &mocks.ShowPrepStoreMock{
		GetListFn: func(context.Context, string) (sp.List, error) {
			return sp.List{ID: showListID, WithdrawalGeneration: 3}, nil
		},
		GetItemsFn:            func(context.Context, string) ([]sp.Item, error) { return *items, nil },
		ReadCertPurchaseIDsFn: func(context.Context, string) ([]string, error) { return []string{p.ID}, nil },
		ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
			return map[string]sp.Purchase{p.ID: p}, nil
		},
		InsertItemFn:           func(_ context.Context, _ string, item sp.Item) error { *items = append(*items, item); return nil },
		SaveItemFn:             func(_ context.Context, _ string, item sp.Item) error { (*items)[0] = item; return nil },
		InsertCommandReceiptFn: func(_ context.Context, r sp.CommandReceipt) error { *receipts = append(*receipts, r); return nil },
	}
}

// Catches missing atomic confirmation, accidental price requirements, and needless baseline/version writes.
func TestScanConfirmOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name, outcome string
		member        *sp.Item
		price         int
		status        sp.Status
		version       int64
		packed        string
	}{
		{"new", "confirmed", nil, 30000, sp.NeedsReview, 1, "2026-10-09T12:00:00Z"},
		{"no asking or comps", "confirmed", nil, 0, sp.NoListedPrice, 1, "2026-10-09T12:00:00Z"},
		{"legacy", "confirmed", func() *sp.Item { i := scanMember(); i.PackedAt = ""; return &i }(), 30000, sp.NeedsReview, 5, "2026-10-09T12:00:00Z"},
		{"already confirmed", "already_confirmed", func() *sp.Item { i := scanMember(); return &i }(), 30000, sp.NeedsReview, 4, "2026-10-02T12:00:00Z"},
		{"price recheck", "rechecked", func() *sp.Item { i := scanMember(); return &i }(), 25000, sp.NeedsReview, 5, "2026-10-02T12:00:00Z"},
		{"support recheck", "rechecked", func() *sp.Item { i := scanMember(); i.AcknowledgedStatus = sp.Supported; return &i }(), 30000, sp.NeedsReview, 5, "2026-10-02T12:00:00Z"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			p := scanPurchase()
			p.LocalPriceCents = tt.price
			items := []sp.Item{}
			if tt.member != nil {
				items = append(items, *tt.member)
			}
			receipts := []sp.CommandReceipt{}
			store := scanStore(p, &items, &receipts)
			inserts, saves, locks := 0, 0, 0
			insert, save := store.InsertItemFn, store.SaveItemFn
			store.InsertItemFn = func(c context.Context, l string, i sp.Item) error { inserts++; return insert(c, l, i) }
			store.SaveItemFn = func(c context.Context, l string, i sp.Item) error { saves++; return save(c, l, i) }
			store.LockPurchasesFn = func(_ context.Context, ids []string) error {
				locks++
				require.Equal(t, []string{scanPurchaseID}, ids)
				return nil
			}
			source := &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
				t.Fatal("scan must not call provider")
				return sp.Snapshot{}, nil
			}}
			result, err := sp.NewService(store, source, func() time.Time { return fixedScanNow }).ScanConfirm(ctx, showListID, scanCommand())
			require.NoError(t, err)
			require.Equal(t, tt.outcome, result.Original.Outcome)
			require.Empty(t, result.Original.Code)
			require.False(t, result.Replay)
			require.Len(t, items, 1)
			require.Equal(t, tt.version, items[0].Version)
			require.Equal(t, tt.packed, items[0].PackedAt)
			require.Equal(t, tt.price, items[0].AcknowledgedPriceCents)
			require.Equal(t, tt.status, items[0].AcknowledgedStatus)
			require.Equal(t, items[0].ID, result.Original.ItemID)
			require.Equal(t, tt.packed, result.Original.ConfirmedAt)
			require.Equal(t, scanPurchaseID, result.Original.PurchaseID)
			require.Equal(t, "Card", result.Original.CardName)
			require.Equal(t, tt.price, result.Original.AcknowledgedPriceCents)
			require.Equal(t, tt.status, result.Original.AcknowledgedStatus)
			require.Equal(t, "present", result.Current.State)
			require.NotNil(t, result.Current.Evaluation)
			require.Len(t, receipts, 1)
			require.Equal(t, result.Original, *receipts[0].Scan)
			require.Equal(t, "scan", receipts[0].Kind)
			require.Equal(t, 1, locks)
			if tt.member == nil {
				require.Equal(t, 1, inserts)
				require.Zero(t, saves)
			} else {
				require.Zero(t, inserts)
				if tt.outcome == "already_confirmed" {
					require.Zero(t, saves)
					require.Equal(t, "prior", items[0].LastCommand)
				} else {
					require.Equal(t, 1, saves)
				}
				require.Equal(t, tt.member.AddedAt, items[0].AddedAt)
			}
		})
	}
}

func TestScanConfirmRejections(t *testing.T) {
	for _, tt := range []struct {
		name, code string
		change     func(*sp.Purchase, *[]sp.Item, *mocks.ShowPrepStoreMock, *sp.ScanCommand)
	}{
		{"generation", "contents_changed", func(_ *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, c *sp.ScanCommand) {
			c.WithdrawalGeneration = 2
		}},
		{"no match", "no_match", func(_ *sp.Purchase, i *[]sp.Item, m *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			*i = nil
			m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) { return nil, nil }
		}},
		{"duplicate including unavailable", "ambiguous_cert", func(_ *sp.Purchase, _ *[]sp.Item, m *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) { return []string{scanPurchaseID, "other"}, nil }
		}},
		{"cert changed during resolution", "identity_changed", func(p *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			p.CertNumber = "87654321"
		}},
		{"saved grader contradiction", "identity_changed", func(_ *sp.Purchase, i *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			(*i)[0].Grader = "BGS"
		}},
		{"saved grade contradiction", "identity_changed", func(_ *sp.Purchase, i *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) { (*i)[0].Grade = 9 }},
		{"saved cert contradiction", "identity_changed", func(_ *sp.Purchase, i *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			(*i)[0].CertNumber = "old"
		}},
		{"saved owner contradiction", "identity_changed", func(_ *sp.Purchase, i *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			(*i)[0].PurchaseID = "other"
		}},
		{"not received", "not_received", func(p *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) { p.Received = false }},
		{"sold", "sold", func(p *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) { p.Sold = true }},
		{"refunded", "refunded", func(p *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) { p.Refunded = true }},
		{"campaign closed", "campaign_closed", func(p *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) { p.Phase = "closed" }},
		{"removed", "removed", func(p *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) { p.Exists = false }},
		{"unknown availability", "unknown_availability", func(p *sp.Purchase, _ *[]sp.Item, _ *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) { p.Known = false }},
		{"saved removed", "removed", func(_ *sp.Purchase, _ *[]sp.Item, m *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) { return nil, nil }
			m.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) { return nil, nil }
		}},
		{"candidate set changed", "identity_changed", func(_ *sp.Purchase, _ *[]sp.Item, m *mocks.ShowPrepStoreMock, _ *sp.ScanCommand) {
			n := 0
			m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) {
				n++
				if n == 1 {
					return []string{scanPurchaseID}, nil
				}
				return nil, nil
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := scanPurchase()
			items := []sp.Item{scanMember()}
			before := items[0]
			receipts := []sp.CommandReceipt{}
			cmd := scanCommand()
			store := scanStore(p, &items, &receipts)
			tt.change(&p, &items, store, &cmd)
			if tt.name != "saved removed" {
				store.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
					return map[string]sp.Purchase{p.ID: p}, nil
				}
			}
			if len(items) > 0 {
				before = items[0]
			}
			store.InsertItemFn = func(context.Context, string, sp.Item) error { t.Fatal("rejection cannot insert"); return nil }
			store.SaveItemFn = func(context.Context, string, sp.Item) error { t.Fatal("rejection cannot save"); return nil }
			if tt.name == "generation" {
				store.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) {
					t.Fatal("stale scan must not resolve")
					return nil, nil
				}
			}
			result, err := sp.NewService(store, nil, func() time.Time { return fixedScanNow }).ScanConfirm(context.Background(), showListID, cmd)
			require.NoError(t, err)
			require.Equal(t, "rejected", result.Original.Outcome)
			require.Equal(t, tt.code, result.Original.Code)
			require.Len(t, receipts, 1)
			require.Equal(t, result.Original, *receipts[0].Scan)
			if len(items) > 0 {
				require.Equal(t, before, items[0])
			}
		})
	}
}

func TestScanConfirmTransientFailures(t *testing.T) {
	failure := errors.New("storage unavailable")
	for _, tt := range []struct {
		name   string
		want   error
		change func(*mocks.ShowPrepStoreMock)
	}{
		{"contention", sp.ErrContended, func(m *mocks.ShowPrepStoreMock) {
			m.LockPurchasesFn = func(context.Context, []string) error { return sp.ErrContended }
		}},
		{"list", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetListFn = func(context.Context, string) (sp.List, error) { return sp.List{}, failure }
		}},
		{"resolve", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) { return nil, failure }
		}},
		{"purchase", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) { return nil, failure }
		}},
		{"hold", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) { return nil, failure }
		}},
		{"evidence", failure, func(m *mocks.ShowPrepStoreMock) {
			m.ReadSnapshotsFn = func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) { return nil, failure }
		}},
		{"receipt read", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetCommandReceiptFn = func(context.Context, string) (sp.CommandReceipt, bool, error) {
				return sp.CommandReceipt{}, false, failure
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := []sp.Item{}
			receipts := []sp.CommandReceipt{}
			store := scanStore(scanPurchase(), &items, &receipts)
			tt.change(store)
			_, err := sp.NewService(store, nil, nil).ScanConfirm(context.Background(), showListID, scanCommand())
			require.ErrorIs(t, err, tt.want)
			require.Empty(t, receipts)
			require.Empty(t, items)
		})
	}
}
