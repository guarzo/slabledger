package showprep_test

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestListStateFingerprintStoredStateOnly(t *testing.T) {
	list := sp.List{ID: "list", Name: "Show", CreatedAt: "created", UpdatedAt: "updated", WithdrawalGeneration: 2}
	items := []sp.Item{{ID: "a", PurchaseID: "p1", CardName: "Card", CertNumber: "123", Grader: "PSA", Grade: 10, AddedAt: "added", PackedAt: "packed", Version: 1, AcknowledgedPriceCents: 12345, AcknowledgedStatus: sp.Supported, LastCommand: "command"}, {ID: "b", PurchaseID: "p2", Version: 2}}
	original := sp.ListStateFingerprint(list, items)
	for _, tt := range []struct {
		name    string
		change  func(*sp.List, []sp.Item)
		changed bool
	}{
		{"list identity", func(l *sp.List, _ []sp.Item) { l.ID = "other" }, true},
		{"list name", func(l *sp.List, _ []sp.Item) { l.Name = "Other" }, true},
		{"created", func(l *sp.List, _ []sp.Item) { l.CreatedAt = "new" }, true},
		{"updated", func(l *sp.List, _ []sp.Item) { l.UpdatedAt = "new" }, true},
		{"generation", func(l *sp.List, _ []sp.Item) { l.WithdrawalGeneration++ }, true},
		{"member identity", func(_ *sp.List, i []sp.Item) { i[0].ID = "new" }, true},
		{"purchase identity", func(_ *sp.List, i []sp.Item) { i[0].PurchaseID = "new" }, true},
		{"saved cert", func(_ *sp.List, i []sp.Item) { i[0].CertNumber = "new" }, true},
		{"saved name", func(_ *sp.List, i []sp.Item) { i[0].CardName = "new" }, true},
		{"grader", func(_ *sp.List, i []sp.Item) { i[0].Grader = "BGS" }, true},
		{"grade", func(_ *sp.List, i []sp.Item) { i[0].Grade = 9 }, true},
		{"added", func(_ *sp.List, i []sp.Item) { i[0].AddedAt = "new" }, true},
		{"version", func(_ *sp.List, i []sp.Item) { i[0].Version++ }, true},
		{"confirmation", func(_ *sp.List, i []sp.Item) { i[0].PackedAt = "" }, true},
		{"baseline price", func(_ *sp.List, i []sp.Item) { i[0].AcknowledgedPriceCents++ }, true},
		{"baseline support", func(_ *sp.List, i []sp.Item) { i[0].AcknowledgedStatus = sp.ThinEvidence }, true},
		{"command", func(_ *sp.List, i []sp.Item) { i[0].LastCommand = "new" }, true},
		{"evaluation not stored", func(_ *sp.List, i []sp.Item) {
			i[0].Evaluation = sp.Evaluation{LocalPriceCents: 99999, Availability: sp.Sold}
			i[0].PriceChanged = true
			i[0].SupportChanged = true
		}, false},
		{"member order", func(_ *sp.List, i []sp.Item) { i[0], i[1] = i[1], i[0] }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l := list
			i := append([]sp.Item(nil), items...)
			tt.change(&l, i)
			got := sp.ListStateFingerprint(l, i)
			require.Equal(t, tt.changed, got != original)
		})
	}
	require.Equal(t, "a", items[0].ID, "fingerprinting must not sort caller's slice")
	require.NotEqual(t, original, sp.ListStateFingerprint(list, items[:1]))
	require.Equal(t, sp.ListStateFingerprint(list, nil), sp.ListStateFingerprint(list, []sp.Item{}))
}

func TestConfirmedReadySummaryPreservesLegacyCounts(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		packed, received, sold bool
		price, count, value    int
	}{
		{"confirmed ready", true, true, false, 30000, 1, 30000},
		{"legacy unconfirmed", false, true, false, 30000, 0, 0},
		{"confirmed missing asking", true, true, false, 0, 1, 0},
		{"confirmed negative asking", true, true, false, -1, 1, 0},
		{"not received", true, false, false, 30000, 0, 0},
		{"sold", true, true, true, 30000, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			item := sp.Item{ID: "member", PurchaseID: "p", Version: 1}
			if tt.packed {
				item.PackedAt = "2026-10-09T12:00:00Z"
			}
			list := sp.List{ID: "list", Name: "Show"}
			store := &mocks.ShowPrepStoreMock{GetListFn: func(context.Context, string) (sp.List, error) { return list, nil }, GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return []sp.Item{item}, nil }, ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
				return map[string]sp.Purchase{"p": {ID: "p", Known: true, Exists: true, CampaignExists: true, Phase: "active", Received: tt.received, Sold: tt.sold, LocalPriceCents: tt.price}}, nil
			}}
			detail, err := sp.NewService(store, nil, time.Now).ListDetail(context.Background(), "list")
			require.NoError(t, err)
			require.Equal(t, tt.count, detail.Summary.ConfirmedReadyCount)
			require.Equal(t, tt.value, detail.Summary.ConfirmedKnownValueCents)
			require.Equal(t, 1, detail.Summary.TotalCount)
			if tt.packed {
				require.Equal(t, 1, detail.Summary.PackedCount)
			} else {
				require.Zero(t, detail.Summary.PackedCount)
			}
			require.Equal(t, sp.ListStateFingerprint(list, []sp.Item{item}), detail.StateFingerprint)
		})
	}
}
