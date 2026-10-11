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

func TestShowPrepScanExactCertAndSavedIdentity(t *testing.T) {
	for _, tt := range []struct{ name, code string }{
		{"sold duplicate", "ambiguous_cert"}, {"refunded duplicate", "ambiguous_cert"}, {"closed duplicate", "ambiguous_cert"}, {"other grader duplicate", "ambiguous_cert"},
		{"cert not substring", "no_match"}, {"cert not trimmed", "no_match"},
		{"saved grader", "identity_changed"}, {"saved grade", "identity_changed"}, {"saved cert", "identity_changed"}, {"saved moved purchase", "identity_changed"}, {"saved owner replaced", "identity_changed"}, {"saved purchase removed", "removed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc := setupScanService(t)
			ctx := context.Background()
			cmd := showScanIntent()
			if len(tt.name) >= 5 && tt.name[:5] == "saved" {
				es, err := svc.Evaluate(ctx, []string{showPurchase})
				require.NoError(t, err)
				_, err = svc.AddItems(ctx, showList, []sp.AddItem{{PurchaseID: showPurchase, EvaluationVersion: es[0].Version}})
				require.NoError(t, err)
			}
			switch tt.name {
			case "sold duplicate", "refunded duplicate", "closed duplicate", "other grader duplicate":
				seedShowPurchase(t, db, showOther, "camp-other", "12345678", "BGS")
				var query string
				switch tt.name {
				case "sold duplicate":
					query = `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-10-09')`
				case "refunded duplicate":
					query = `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`
				case "closed duplicate":
					query = `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`
				}
				if query != "" {
					_, err := db.ExecContext(ctx, query, showOther)
					require.NoError(t, err)
				}
			case "cert not substring":
				cmd.CertNumber = "234567"
			case "cert not trimmed":
				_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET cert_number=' 12345678 ' WHERE id=$1`, showPurchase)
				require.NoError(t, err)
			case "saved grader":
				_, err := db.ExecContext(ctx, `UPDATE showprep_items SET grader='BGS'`)
				require.NoError(t, err)
			case "saved grade":
				_, err := db.ExecContext(ctx, `UPDATE showprep_items SET grade=9`)
				require.NoError(t, err)
			case "saved cert":
				_, err := db.ExecContext(ctx, `UPDATE showprep_items SET cert_number='old'`)
				require.NoError(t, err)
			case "saved moved purchase":
				_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET cert_number='changed' WHERE id=$1`, showPurchase)
				require.NoError(t, err)
			case "saved owner replaced":
				_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET cert_number='changed' WHERE id=$1`, showPurchase)
				require.NoError(t, err)
				seedShowPurchase(t, db, showOther, "camp-other", "12345678", "PSA")
			case "saved purchase removed":
				_, err := db.ExecContext(ctx, `DELETE FROM campaign_purchases WHERE id=$1`, showPurchase)
				require.NoError(t, err)
			}
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			result, err := svc.ScanConfirm(ctx, showList, cmd)
			require.NoError(t, err)
			require.Equal(t, "rejected", result.Original.Outcome)
			require.Equal(t, tt.code, result.Original.Code)
			scanReceiptCount(t, db, 1)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, before, after)
			if tt.code == "ambiguous_cert" {
				var n int
				require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&n))
				require.Equal(t, 2, n)
				_, err := db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='camp-other'`)
				require.NoError(t, err)
				holds, err := NewShowPrepStore(db.DB).ObservePriceAssociations(ctx, []string{showPurchase})
				require.NoError(t, err)
				require.True(t, holds[showPurchase])
			}
		})
	}
}

func TestShowPrepScanCandidateSetRequery(t *testing.T) {
	for _, change := range []string{"insert", "delete"} {
		t.Run(change, func(t *testing.T) {
			db, store, _ := setupScanService(t)
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase) VALUES('camp-other','Other','pending')`)
			require.NoError(t, err)
			controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
				return store.Within(ctx, func(tx sp.Session) error {
					m := scanSessionMock(tx)
					m.LockPurchasesFn = func(ctx context.Context, ids []string) error {
						require.Equal(t, []string{showPurchase}, ids)
						if change == "insert" {
							_, err := db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date) VALUES($1,'camp-other','Competitor','12345678','BGS',10,'2026-10-09')`, showOther)
							require.NoError(t, err)
						} else {
							_, err := db.ExecContext(ctx, `DELETE FROM campaign_purchases WHERE id=$1`, showPurchase)
							require.NoError(t, err)
						}
						return tx.LockPurchases(ctx, ids)
					}
					return fn(m)
				})
			}}
			result, err := sp.NewService(controlled, nil, nil).ScanConfirm(ctx, showList, showScanIntent())
			require.NoError(t, err)
			require.Equal(t, "identity_changed", result.Original.Code)
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Empty(t, items)
			scanReceiptCount(t, db, 1)
			if change == "insert" {
				var n int
				require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&n))
				require.Equal(t, 2, n)
			}
		})
	}
}

// A live competitor found by later association/evidence reads is an identity
// ambiguity, unlike a retained historical price hold after its owner is gone.
func TestShowPrepScanLateCertCompetitor(t *testing.T) {
	for _, tt := range []struct {
		name, outcome, code                                  string
		insertAfterRequery, insertDuringEvidence, historical bool
		removeDuringEvidence, preexistingHolds               bool
	}{
		{"after locked requery", "rejected", "ambiguous_cert", true, false, false, false, false},
		{"during cached evidence read", "rejected", "ambiguous_cert", false, true, false, false, false},
		{"observed then removed", "rejected", "ambiguous_cert", true, false, false, true, false},
		{"observed with existing holds then removed", "rejected", "ambiguous_cert", true, false, false, true, true},
		{"historical hold only", "confirmed", "", false, false, true, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, _ := setupScanService(t)
			ctx := context.Background()
			seedShowEvidence(t, store)
			if tt.historical || tt.preexistingHolds {
				seedShowPurchase(t, db, showOther, "camp-other", "12345678", "BGS")
				_, err := store.ObservePriceAssociations(ctx, []string{showPurchase})
				require.NoError(t, err)
				_, err = db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='camp-other'`)
				require.NoError(t, err)
			}
			controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
				return store.Within(ctx, func(tx sp.Session) error {
					m := scanSessionMock(tx)
					reads := 0
					m.ReadCertPurchaseIDsFn = func(ctx context.Context, cert string) ([]string, error) {
						ids, err := tx.ReadCertPurchaseIDs(ctx, cert)
						reads++
						if reads == 2 && tt.insertAfterRequery {
							seedShowPurchase(t, db, showOther, "camp-other", "12345678", "BGS")
						}
						return ids, err
					}
					m.ReadSnapshotsFn = func(ctx context.Context, identities []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
						if tt.insertDuringEvidence {
							seedShowPurchase(t, db, showOther, "camp-other", "12345678", "BGS")
						}
						if tt.removeDuringEvidence {
							_, err := db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='camp-other'`)
							require.NoError(t, err)
							// Repeat observation after deletion: a session latch must not reset.
							_, err = tx.ObservePriceAssociations(ctx, []string{showPurchase})
							require.NoError(t, err)
						}
						return tx.ReadSnapshots(ctx, identities)
					}
					return fn(m)
				})
			}}
			result, err := sp.NewService(controlled, nil, time.Now).ScanConfirm(ctx, showList, showScanIntent())
			require.NoError(t, err)
			require.Equal(t, tt.outcome, result.Original.Outcome)
			require.Equal(t, tt.code, result.Original.Code)
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			if tt.historical {
				require.Len(t, items, 1)
				require.EqualValues(t, 1, items[0].Version)
				require.Equal(t, 30000, items[0].AcknowledgedPriceCents)
				require.Equal(t, sp.Supported, items[0].AcknowledgedStatus)
				require.True(t, result.Current.Evaluation.PriceAssociationUnclear)
			} else {
				require.Empty(t, items, "ambiguous identity must never create confirmed membership")
				require.Empty(t, result.Original.ItemID)
			}
			scanReceiptCount(t, db, 1)
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				r, found, err := tx.GetCommandReceipt(ctx, showScanCommand)
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, result.Original, *r.Scan)
				return nil
			}))
			var holds int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds WHERE purchase_id IN ($1,$2)`, showPurchase, showOther).Scan(&holds))
			require.Equal(t, 2, holds, "rejection must persist the observed competitors without a later observation write")
		})
	}
}

func TestShowPrepScanStrictStorageFailurePreservesObservedCollision(t *testing.T) {
	db, store, _ := setupScanService(t)
	ctx := context.Background()
	failure := errors.New("evidence unavailable")
	var observedAt time.Time
	controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
		return store.Within(ctx, func(tx sp.Session) error {
			m := scanSessionMock(tx)
			reads := 0
			m.ReadCertPurchaseIDsFn = func(ctx context.Context, cert string) ([]string, error) {
				ids, err := tx.ReadCertPurchaseIDs(ctx, cert)
				reads++
				if reads == 2 {
					seedShowPurchase(t, db, showOther, "camp-other", "12345678", "BGS")
				}
				return ids, err
			}
			m.ReadSnapshotsFn = func(ctx context.Context, _ []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				require.NoError(t, tx.(*showPrepSession).q.QueryRowContext(ctx, `SELECT first_detected_at FROM showprep_price_holds WHERE purchase_id=$1`, showPurchase).Scan(&observedAt))
				return nil, failure
			}
			return fn(m)
		})
	}}
	_, err := sp.NewService(controlled, nil, nil).ScanConfirm(ctx, showList, showScanIntent())
	require.ErrorIs(t, err, failure)
	scanReceiptCount(t, db, 0)
	items, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Empty(t, items)
	_, err = db.ExecContext(ctx, `DELETE FROM campaigns WHERE id='camp-other'`)
	require.NoError(t, err)
	for _, id := range []string{showPurchase, showOther} {
		var at time.Time
		require.NoError(t, db.QueryRowContext(ctx, `SELECT first_detected_at FROM showprep_price_holds WHERE purchase_id=$1`, id).Scan(&at))
		require.True(t, observedAt.Equal(at))
	}
	holds, err := NewShowPrepStore(db.DB).ObservePriceAssociations(ctx, []string{showPurchase})
	require.NoError(t, err)
	require.True(t, holds[showPurchase])
	// Infrastructure failure did not finalize the command: identical payload can succeed after recovery.
	result, err := sp.NewService(NewShowPrepStore(db.DB), nil, nil).ScanConfirm(ctx, showList, showScanIntent())
	require.NoError(t, err)
	require.Equal(t, "confirmed", result.Original.Outcome)
	scanReceiptCount(t, db, 1)
}
