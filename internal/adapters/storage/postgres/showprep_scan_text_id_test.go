package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestShowPrepScanReceivedTextPurchaseID(t *testing.T) {
	for _, id := range []string{"legacy-card-1", showPurchase} {
		t.Run(id, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase) VALUES('text-id-campaign','TEXT identity','active')`)
			require.NoError(t, err)
			received := "2026-09-02"
			purchase := &inventory.Purchase{ID: id, CampaignID: "text-id-campaign", CardName: "Legacy card", CertNumber: "00123AB", Grader: "PSA", GradeValue: 10, PurchaseDate: "2026-09-01", ReceivedAt: &received, CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), ReviewedPriceCents: 12345}
			require.NoError(t, NewPurchaseStore(db.DB, observability.NewNoopLogger()).CreatePurchase(ctx, purchase))
			store := NewShowPrepStore(db.DB)
			svc := sp.NewService(store, nil, nil)
			_, err = svc.CreateList(ctx, showList, "TEXT identity")
			require.NoError(t, err)
			cmd := sp.ScanCommand{CommandID: showScanCommand, CertNumber: purchase.CertNumber}
			result, err := svc.ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.Equal(t, "confirmed", result.Original.Outcome)
			require.Equal(t, id, result.Original.PurchaseID)
			require.Equal(t, id, result.Current.Evaluation.PurchaseID)
			require.Equal(t, 12345, result.Original.AcknowledgedPriceCents)
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			replay, err := sp.NewService(NewShowPrepStore(db.DB), nil, nil).ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.True(t, replay.Replay)
			require.Equal(t, result.Original, replay.Original)
			require.Equal(t, "present", replay.Current.State)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
