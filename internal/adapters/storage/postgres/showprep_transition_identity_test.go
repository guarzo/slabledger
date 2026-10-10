package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// These schedules use real observation SQL and its in-session collision latch.
func TestShowPrepTransitionLateCertCollision(t *testing.T) {
	for _, tt := range []struct {
		name                                                                         string
		duringEvidence, remove, holds, historical, failEvidence, afterCertValidation bool
	}{
		{name: "during locked observation"}, {name: "during evidence", duringEvidence: true},
		{name: "after initial cert validation", afterCertValidation: true},
		{name: "observed then removed", remove: true}, {name: "existing holds observed then removed", remove: true, holds: true},
		{name: "historical only", historical: true}, {name: "evidence failure", failEvidence: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, _, cmd := setupTransitionService(t)
			ctx := context.Background()
			const competitor = "77777777-7777-4777-8777-777777777777"
			if tt.holds || tt.historical {
				seedShowPurchase(t, db, competitor, "camp-competitor", "12345678", "BGS")
				_, err := store.ObservePriceAssociations(ctx, []string{showPurchase})
				require.NoError(t, err)
				_, err = db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='camp-competitor'`)
				require.NoError(t, err)
			}
			failure := errors.New("evidence failed")
			controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
				return store.Within(ctx, func(tx sp.Session) error {
					m := scanSessionMock(tx)
					inserted := false
					m.ObservePriceAssociationsFn = func(ctx context.Context, ids []string) (map[string]bool, error) {
						if !tt.historical && !tt.duringEvidence && !tt.afterCertValidation && !inserted {
							seedShowPurchase(t, db, competitor, "camp-competitor", "12345678", "BGS")
							inserted = true
						}
						return tx.ObservePriceAssociations(ctx, ids)
					}
					m.ReadSnapshotsFn = func(ctx context.Context, ids []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
						if tt.duringEvidence {
							seedShowPurchase(t, db, competitor, "camp-competitor", "12345678", "BGS")
						}
						if tt.remove {
							_, err := db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='camp-competitor'`)
							require.NoError(t, err)
							_, err = tx.ObservePriceAssociations(ctx, []string{showPurchase})
							require.NoError(t, err)
						}
						if tt.failEvidence {
							return nil, failure
						}
						return tx.ReadSnapshots(ctx, ids)
					}
					if tt.afterCertValidation {
						reads := 0
						m.ReadCertPurchaseIDsFn = func(ctx context.Context, cert string) ([]string, error) {
							ids, err := tx.ReadCertPurchaseIDs(ctx, cert)
							reads++
							if reads == 2 {
								seedShowPurchase(t, db, competitor, "camp-competitor", "12345678", "BGS")
							}
							return ids, err
						}
					}
					return fn(m)
				})
			}}
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			result, err := sp.NewService(controlled, nil, func() time.Time { return showScanNow }).ConfirmExisting(ctx, showList, cmd)
			after, readErr := store.GetItems(ctx, showList)
			require.NoError(t, readErr)
			if tt.historical {
				require.NoError(t, err)
				require.Len(t, result.ConfirmedItemIDs, 2)
				for _, i := range after {
					require.NotEmpty(t, i.PackedAt)
				}
				scanReceiptCount(t, db, 1)
			} else {
				if tt.failEvidence {
					require.ErrorIs(t, err, failure)
				} else {
					require.ErrorIs(t, err, sp.ErrConflict)
				}
				require.Equal(t, before, after)
				scanReceiptCount(t, db, 0)
			}
			var count int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds WHERE purchase_id IN ($1,$2)`, showPurchase, competitor).Scan(&count))
			require.Equal(t, 2, count, "rollback must retain actual collision observations")
		})
	}
}

func TestShowPrepTransitionFinancialContention(t *testing.T) {
	for _, tt := range []struct{ name, write string }{
		{"sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-10-09')`},
		{"refund", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`},
		{"receipt removal", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`},
		{"closure", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`},
		{"delete", `DELETE FROM campaign_purchases WHERE id=$1`},
	} {
		for _, commit := range []bool{false, true} {
			t.Run(tt.name+map[bool]string{true: " commit", false: " rollback"}[commit], func(t *testing.T) {
				db, store, svc, cmd := setupTransitionService(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				before, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				writer, err := db.BeginTx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = writer.Rollback() }()
				_, err = writer.ExecContext(ctx, tt.write, showOther)
				require.NoError(t, err)
				_, err = svc.ConfirmExisting(ctx, showList, cmd)
				require.ErrorIs(t, err, sp.ErrContended)
				scanReceiptCount(t, db, 0)
				after, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				require.Equal(t, before, after)
				// Batch validation released its earlier first-purchase lock on contention.
				requireShowPrepPurchaseUnlocked(t, ctx, db, showPurchase)
				if commit {
					require.NoError(t, writer.Commit())
				} else {
					require.NoError(t, writer.Rollback())
				}
				_, err = svc.ConfirmExisting(ctx, showList, cmd)
				if commit {
					require.ErrorIs(t, err, sp.ErrConflict)
					scanReceiptCount(t, db, 0)
				} else {
					require.NoError(t, err)
					scanReceiptCount(t, db, 1)
				}
			})
		}
	}
}

func TestShowPrepTransitionLocksCurrentCampaign(t *testing.T) {
	db, _, svc, cmd := setupTransitionService(t)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase) VALUES('camp-new','New','closed')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET campaign_id='camp-new' WHERE id=$1`, showOther)
	require.NoError(t, err)
	writer, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback() }()
	_, err = writer.ExecContext(ctx, `SELECT id FROM campaigns WHERE id='camp-new' FOR UPDATE`)
	require.NoError(t, err)
	_, err = svc.ConfirmExisting(ctx, showList, cmd)
	require.ErrorIs(t, err, sp.ErrContended)
	require.NoError(t, writer.Rollback())
	_, err = svc.ConfirmExisting(ctx, showList, cmd)
	require.ErrorIs(t, err, sp.ErrConflict)
	scanReceiptCount(t, db, 0)
}
