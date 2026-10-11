package showprep_test

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Preserves lifecycle coverage formerly exercised through the now-retired HTTP
// Pack/Acknowledge endpoint. These helpers still serve domain/storage fixtures.
func TestLegacyPackingRetriesAndStaleAcknowledgment(t *testing.T) {
	ctx := context.Background()
	p := sp.Purchase{ID: scanPurchaseID, CardName: "Card", CertNumber: "123", Grader: "PSA", Grade: 10, Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "pending", LocalPriceCents: 30000}
	item := sp.Item{ID: "item", PurchaseID: scanPurchaseID, Version: 1, AcknowledgedPriceCents: 30000, AcknowledgedStatus: sp.NeedsReview}
	writes := 0
	store := &mocks.ShowPrepStoreMock{
		ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
			return map[string]sp.Purchase{scanPurchaseID: p}, nil
		},
		GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return []sp.Item{item}, nil },
		SaveItemFn: func(_ context.Context, _ string, saved sp.Item) error { item = saved; writes++; return nil },
	}
	svc := sp.NewService(store, nil, func() time.Time { return fixedScanNow })
	es, err := svc.Evaluate(ctx, []string{scanPurchaseID})
	require.NoError(t, err)
	yes, no := true, false
	pack := sp.UpdateItem{Version: 1, EvaluationVersion: es[0].Version, Packed: &yes}
	for _, tt := range []struct {
		name          string
		cmd           sp.UpdateItem
		price, writes int
		want          error
	}{
		{"pack", pack, 30000, 1, nil},
		{"transport replay", pack, 30000, 1, nil},
		{"stale conflicting intent", sp.UpdateItem{Version: 1, Packed: &no}, 30000, 1, sp.ErrConflict},
		{"stale acknowledgment after price sync", sp.UpdateItem{Version: 2, EvaluationVersion: es[0].Version, Acknowledge: true}, 40000, 1, sp.ErrConflict},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p.LocalPriceCents = tt.price
			result, err := svc.UpdateItem(ctx, showListID, item.ID, tt.cmd)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			} else {
				require.NoError(t, err)
				require.Len(t, result.Items, 1)
				require.Equal(t, 1, result.Summary.PackedCount)
				require.Equal(t, int64(2), result.Items[0].Version)
			}
			require.Equal(t, tt.writes, writes)
			require.Equal(t, 30000, item.AcknowledgedPriceCents)
			require.Equal(t, "2026-10-09T12:00:00Z", item.PackedAt)
		})
	}
}
