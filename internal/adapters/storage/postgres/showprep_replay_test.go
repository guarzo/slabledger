package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

// A transport replay recognizes a completed intent, not a new request to pack
// or acknowledge the purchase's current price and availability.
func TestShowPrepAppliedReplaySurvivesInventoryChanges(t *testing.T) {
	for _, operation := range []string{"pack", "acknowledge"} {
		for _, change := range []struct {
			name         string
			availability sp.Availability
			status       sp.Status
		}{
			{"price", sp.Ready, sp.BelowTarget},
			{"refunded", sp.Refunded, sp.Supported},
			{"removed", sp.Removed, sp.NoListedPrice},
			{"purchase locked", sp.Ready, sp.Supported},
			{"collision", sp.Ready, sp.NeedsReview},
		} {
			t.Run(operation+"/"+change.name, func(t *testing.T) {
				db := setupShowPrepTestDB(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				seedShowPurchase(t, db, showPurchase, "camp-show", "cert", "PSA")
				store := NewShowPrepStore(db.DB)
				seedShowEvidence(t, store)
				svc := sp.NewService(store, nil, time.Now)
				_, err := svc.CreateList(ctx, showList, "Replay")
				require.NoError(t, err)
				es, err := svc.Evaluate(ctx, []string{showPurchase})
				require.NoError(t, err)
				detail, err := svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
				require.NoError(t, err)
				item := detail.Items[0]
				cmd := sp.UpdateItem{Version: item.Version, EvaluationVersion: item.Evaluation.Version, Acknowledge: operation == "acknowledge"}
				if operation == "pack" {
					yes := true
					cmd.Packed = &yes
				}
				_, err = svc.UpdateItem(ctx, showList, item.ID, cmd)
				require.NoError(t, err)
				stored, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				require.EqualValues(t, 2, stored[0].Version)
				require.Equal(t, 30000, stored[0].AcknowledgedPriceCents)
				require.Equal(t, sp.Supported, stored[0].AcknowledgedStatus)

				switch change.name {
				case "price":
					_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_listing_price_cents=40000 WHERE id=$1`, showPurchase)
				case "refunded":
					_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`, showPurchase)
				case "removed":
					_, err = db.ExecContext(ctx, `DELETE FROM campaign_purchases WHERE id=$1`, showPurchase)
				case "purchase locked":
					writer, beginErr := db.BeginTx(ctx, nil)
					require.NoError(t, beginErr)
					defer func() { _ = writer.Rollback() }()
					_, err = writer.ExecContext(ctx, `SELECT id FROM campaign_purchases WHERE id=$1 FOR UPDATE`, showPurchase)
				case "collision":
					seedShowPurchase(t, db, showOther, "camp-other", "cert", "BGS")
				}
				require.NoError(t, err)
				list, err := store.GetList(ctx, showList)
				require.NoError(t, err)
				// Reconstruct the service to prove replay recognition is persisted.
				svc = sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
				replayed, err := svc.UpdateItem(ctx, showList, item.ID, cmd)
				require.NoError(t, err, "the original command was already applied")
				after, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				require.Equal(t, stored, after, "replay must not repack, re-acknowledge, or increment the version")
				require.Equal(t, list, replayed.List)
				require.Equal(t, change.availability, replayed.Items[0].Evaluation.Availability)
				require.Equal(t, change.status, replayed.Items[0].Evaluation.Status)
				if change.name == "price" {
					require.True(t, replayed.Items[0].PriceChanged)
					require.True(t, replayed.Items[0].SupportChanged)
				}
				if change.name == "collision" {
					require.True(t, replayed.Items[0].Evaluation.PriceAssociationUnclear)
					_, err = db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='camp-other'`)
					require.NoError(t, err)
					replayed, err = svc.UpdateItem(ctx, showList, item.ID, cmd)
					require.NoError(t, err)
					require.True(t, replayed.Items[0].Evaluation.PriceAssociationUnclear)
					var holds int
					require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&holds))
					require.Equal(t, 2, holds)
				}
				// A stale but different intent is not an idempotent replay.
				different := cmd
				different.EvaluationVersion = "different-intent"
				_, err = svc.UpdateItem(ctx, showList, item.ID, different)
				require.ErrorIs(t, err, sp.ErrConflict)
				if change.name == "price" {
					_, err = svc.UpdateItem(ctx, showList, item.ID, sp.UpdateItem{Version: 2, EvaluationVersion: replayed.Items[0].Evaluation.Version, Acknowledge: true})
					require.NoError(t, err)
					_, err = svc.UpdateItem(ctx, showList, item.ID, cmd)
					require.ErrorIs(t, err, sp.ErrConflict, "an intervening command supersedes the old replay")
				}
			})
		}
	}
}
