package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepLockedCollisionSurvivesRejectedMutation(t *testing.T) {
	for _, operation := range []struct {
		name      string
		pack, add bool
	}{
		{"pack", true, false},
		{"acknowledge", false, false},
		{"batch add", false, true},
	} {
		for _, deletion := range []struct {
			name   string
			remove func(context.Context, *DB) error
		}{
			{"purchase", func(ctx context.Context, db *DB) error {
				return NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(ctx, showOther)
			}},
			{"campaign", func(ctx context.Context, db *DB) error {
				return NewCampaignStore(db.DB, mocks.NewMockLogger()).DeleteCampaign(ctx, "other-camp")
			}},
		} {
			t.Run(operation.name+"/"+deletion.name, func(t *testing.T) {
				db := setupShowPrepTestDB(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				seedShowPurchase(t, db, showPurchase, "camp-show", "collision", "PSA")
				const unique = "44444444-4444-4444-8444-444444444444"
				seedShowPurchase(t, db, unique, "camp-show", "unique", "PSA")
				store := NewShowPrepStore(db.DB)
				seedShowEvidence(t, store)
				svc := sp.NewService(store, nil, time.Now)
				_, err := svc.CreateList(ctx, showList, "Concurrent collision")
				require.NoError(t, err)
				es, err := svc.Evaluate(ctx, []string{unique, showPurchase})
				require.NoError(t, err)
				require.Equal(t, sp.Supported, es[1].Status)
				if !operation.add {
					_, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[1].Version}})
					require.NoError(t, err)
				}
				before, err := store.GetItems(ctx, showList)
				require.NoError(t, err)

				// A real independent writer inserts only after the preliminary commit,
				// then deletes after locked observation but BEFORE rollback/rejection.
				insert, remove := make(chan struct{}), make(chan struct{})
				inserted, removed := make(chan error, 1), make(chan error, 1)
				writerDone := make(chan struct{})
				defer func() { cancel(); <-writerDone }()
				go func() {
					defer close(writerDone)
					select {
					case <-insert:
					case <-ctx.Done():
						return
					}
					_, err := db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase) VALUES('other-camp','Competitor','pending')`)
					if err == nil {
						_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date,dh_listing_price_cents) VALUES($1,'other-camp','Competitor','collision','BGS',10,'2026-09-01',99999)`, showOther)
					}
					inserted <- err
					select {
					case <-remove:
					case <-ctx.Done():
						return
					}
					removed <- deletion.remove(ctx, db)
				}()
				var observedAt time.Time
				insertedItems := 0
				controlled := &mocks.ShowPrepStoreMock{
					GetItemsFn: store.GetItems,
					ObservePriceAssociationsFn: func(ctx context.Context, ids []string) (map[string]bool, error) {
						holds, err := store.ObservePriceAssociations(ctx, ids)
						require.NoError(t, err)
						require.Empty(t, holds, "preliminary observation must precede the collision")
						close(insert)
						require.NoError(t, <-inserted)
						return holds, nil
					},
					WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
						return store.Within(ctx, func(tx sp.Session) error {
							return fn(&mocks.ShowPrepStoreMock{
								GetListFn: tx.GetList, GetItemsFn: tx.GetItems,
								LockPurchasesFn: tx.LockPurchases, ReadPurchasesFn: tx.ReadPurchases,
								ReadSnapshotsFn: tx.ReadSnapshots, SaveItemFn: tx.SaveItem,
								InsertItemFn: func(ctx context.Context, list string, item sp.Item) error {
									err := tx.InsertItem(ctx, list, item)
									if err == nil {
										insertedItems++
									}
									return err
								},
								ObservePriceAssociationsFn: func(ctx context.Context, ids []string) (map[string]bool, error) {
									holds, err := tx.ObservePriceAssociations(ctx, ids)
									require.NoError(t, err)
									require.True(t, holds[showPurchase], "locked validation must observe the new collision")
									require.NoError(t, tx.(*showPrepSession).q.QueryRowContext(ctx, `SELECT first_detected_at FROM showprep_price_holds WHERE purchase_id=$1`, showPurchase).Scan(&observedAt))
									close(remove)
									require.NoError(t, <-removed)
									return holds, nil
								},
							})
						})
					},
				}
				mutator := sp.NewService(controlled, nil, time.Now)
				if operation.add {
					_, err = mutator.AddItems(ctx, showList, []sp.AddItem{
						{PurchaseID: unique, EvaluationVersion: es[0].Version},
						{PurchaseID: showPurchase, EvaluationVersion: es[1].Version},
					})
					require.Equal(t, 1, insertedItems, "first batch member must actually be written before rejection")
				} else {
					cmd := sp.UpdateItem{Version: before[0].Version, EvaluationVersion: es[1].Version, Acknowledge: true}
					if operation.pack {
						yes := true
						cmd.Packed = &yes
					}
					_, err = mutator.UpdateItem(ctx, showList, before[0].ID, cmd)
				}
				require.ErrorIs(t, err, sp.ErrConflict)
				after, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				require.Equal(t, before, after, "rejection must roll back the entire list mutation")
				var count int
				require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&count))
				require.Equal(t, 2, count, "both observed identities must survive, even though the competitor is already gone")
				for _, hold := range []struct{ id, grader string }{{showPurchase, "PSA"}, {showOther, "BGS"}} {
					var cert, grader string
					var detected time.Time
					require.NoError(t, db.QueryRowContext(ctx, `SELECT cert_number,grader,first_detected_at FROM showprep_price_holds WHERE purchase_id=$1`, hold.id).Scan(&cert, &grader, &detected))
					require.Equal(t, "collision", cert)
					require.Equal(t, hold.grader, grader)
					require.True(t, observedAt.Equal(detected), "preserve the actual first observation")
				}
				// Reconstruct the service after the competitor is gone. No fresh
				// collision query can now explain away a missing durable observation.
				svc = sp.NewService(NewShowPrepStore(db.DB), nil, time.Now)
				es, err = svc.Evaluate(ctx, []string{showPurchase})
				require.NoError(t, err)
				require.True(t, es[0].PriceAssociationUnclear)
				require.Equal(t, sp.NeedsReview, es[0].Status)
				if operation.add {
					_, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
					require.NoError(t, err)
				}
				detail, err := svc.ListDetail(ctx, showList)
				require.NoError(t, err)
				require.Zero(t, detail.Summary.KnownValueCents)
				require.Equal(t, 1, detail.Summary.AmbiguousPriceCount)
				require.Zero(t, detail.Summary.PackedCount)
			})
		}
	}
}
