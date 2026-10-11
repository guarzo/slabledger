package showprep_test

import (
	"context"
	"slices"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Catches a read presenting saved confirmation as ready after live identity correction.
func TestListDetailConfirmedIdentity(t *testing.T) {
	for _, tt := range []struct {
		name                          string
		change                        func(*sp.Purchase, *sp.Item)
		changed                       bool
		confirmed, value, legacyValue int
	}{
		{"unchanged", func(_ *sp.Purchase, _ *sp.Item) {}, false, 1, 30000, 30000},
		{"grade correction", func(p *sp.Purchase, _ *sp.Item) { p.Grade = 9 }, true, 0, 0, 30000},
		{"grader correction", func(p *sp.Purchase, _ *sp.Item) { p.Grader = "BGS" }, true, 0, 0, 30000},
		{"cert correction", func(p *sp.Purchase, _ *sp.Item) { p.CertNumber = "87654321" }, true, 0, 0, 30000},
		{"cert is exact", func(p *sp.Purchase, i *sp.Item) { p.CertNumber = "00123ab"; i.CertNumber = "00123AB" }, true, 0, 0, 30000},
		{"grader trim and case", func(p *sp.Purchase, i *sp.Item) { p.Grader = " psa "; i.Grader = " PSA " }, false, 1, 30000, 30000},
		{"name is not identity", func(p *sp.Purchase, _ *sp.Item) { p.CardName = "Corrected name" }, false, 1, 30000, 30000},
		{"removed identity unknown", func(p *sp.Purchase, _ *sp.Item) { p.Exists = false }, false, 0, 0, 0},
		{"unknown identity", func(p *sp.Purchase, _ *sp.Item) { p.Known = false }, false, 0, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			p, item := scanPurchase(), scanMember()
			tt.change(&p, &item)
			store := &mocks.ShowPrepStoreMock{
				GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return slices.Clone([]sp.Item{item}), nil },
				ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
					return map[string]sp.Purchase{item.PurchaseID: p}, nil
				},
			}
			service := sp.NewService(store, nil, func() time.Time { return fixedScanNow })
			detail, err := service.ListDetail(ctx, showListID)
			require.NoError(t, err)
			require.Len(t, detail.Items, 1)
			require.Equal(t, tt.confirmed, detail.Summary.ConfirmedReadyCount)
			require.Equal(t, tt.value, detail.Summary.ConfirmedKnownValueCents)
			require.Equal(t, tt.changed, detail.Items[0].IdentityChanged)
			require.Equal(t, tt.legacyValue, detail.Summary.KnownValueCents)
			require.Equal(t, 1, detail.Summary.PackedCount)
			require.Equal(t, item.PackedAt, detail.Items[0].PackedAt)
			require.Equal(t, item.Grade, detail.Items[0].Grade)
			require.Equal(t, item.CertNumber, detail.Items[0].CertNumber)
			require.Equal(t, item.Grader, detail.Items[0].Grader)
			require.Equal(t, item.Version, detail.Items[0].Version)
			require.Equal(t, item.AcknowledgedPriceCents, detail.Items[0].AcknowledgedPriceCents)
			require.Equal(t, item.AcknowledgedStatus, detail.Items[0].AcknowledgedStatus)
			require.Equal(t, sp.ListStateFingerprint(detail.List, []sp.Item{item}), detail.StateFingerprint)
		})
	}
}
