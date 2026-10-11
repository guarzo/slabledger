package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Shared Fn mocks only intercept the receipt boundary; all locking, evaluation,
// membership and transaction persistence below are the real adapter.
func scanSessionMock(tx sp.Session) *mocks.ShowPrepStoreMock {
	return &mocks.ShowPrepStoreMock{
		GetCommandReceiptFn: tx.GetCommandReceipt, InsertCommandReceiptFn: tx.InsertCommandReceipt,
		GetListFn: tx.GetList, GetItemsFn: tx.GetItems, ReadCertPurchaseIDsFn: tx.ReadCertPurchaseIDs,
		GetRetiredListFn: tx.GetRetiredList, RetireListFn: tx.RetireList,
		LockPurchasesFn: tx.LockPurchases, ReadPurchasesFn: tx.ReadPurchases,
		ObservePriceAssociationsFn: tx.ObservePriceAssociations, ReadSnapshotsFn: tx.ReadSnapshots,
		CertCollisionObservedFn: tx.CertCollisionObserved,
		InsertItemFn:            tx.InsertItem, SaveItemFn: tx.SaveItem,
	}
}

func TestShowPrepScanFinancialContention(t *testing.T) {
	for _, tt := range []struct{ name, write, code string }{
		{"sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-10-09')`, "sold"},
		{"refund", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`, "refunded"},
		{"receipt removal", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`, "not_received"},
		{"closure", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`, "campaign_closed"},
	} {
		for _, commit := range []bool{false, true} {
			t.Run(tt.name+map[bool]string{true: "/commit", false: "/rollback"}[commit], func(t *testing.T) {
				db, store, svc := setupScanService(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				writer, err := db.BeginTx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = writer.Rollback() }()
				_, err = writer.ExecContext(ctx, tt.write, showPurchase)
				require.NoError(t, err)
				_, err = svc.ScanConfirm(ctx, showList, showScanIntent())
				require.ErrorIs(t, err, sp.ErrContended)
				require.ErrorIs(t, err, sp.ErrConflict)
				items, err := store.GetItems(ctx, showList)
				require.NoError(t, err)
				require.Empty(t, items)
				scanReceiptCount(t, db, 0)
				if commit {
					require.NoError(t, writer.Commit())
				} else {
					require.NoError(t, writer.Rollback())
				}
				result, err := svc.ScanConfirm(ctx, showList, showScanIntent())
				require.NoError(t, err)
				scanReceiptCount(t, db, 1)
				items, err = store.GetItems(ctx, showList)
				require.NoError(t, err)
				if commit {
					require.Equal(t, "rejected", result.Original.Outcome)
					require.Equal(t, tt.code, result.Original.Code)
					require.Empty(t, items)
				} else {
					require.Equal(t, "confirmed", result.Original.Outcome)
					require.Len(t, items, 1)
					require.EqualValues(t, 1, items[0].Version)
					require.Equal(t, "2026-10-09T12:00:00Z", items[0].PackedAt)
					require.Equal(t, 30000, items[0].AcknowledgedPriceCents)
				}
			})
		}
	}
}

func TestShowPrepScanBlocksFinancialWritersUntilReceiptCommit(t *testing.T) {
	for _, tt := range []struct{ name, write string }{
		{"sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-10-09')`},
		{"refund", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`},
		{"receipt removal", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`},
		{"closure", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, _ := setupScanService(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			locked, release := make(chan struct{}), make(chan struct{})
			result := make(chan sp.ScanResult, 1)
			scanErr := make(chan error, 1)
			controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
				return store.Within(ctx, func(tx sp.Session) error {
					m := scanSessionMock(tx)
					m.InsertCommandReceiptFn = func(ctx context.Context, r sp.CommandReceipt) error {
						close(locked)
						select {
						case <-release:
						case <-ctx.Done():
							return ctx.Err()
						}
						return tx.InsertCommandReceipt(ctx, r)
					}
					return fn(m)
				})
			}}
			go func() {
				r, e := sp.NewService(controlled, nil, func() time.Time { return showScanNow }).ScanConfirm(ctx, showList, showScanIntent())
				result <- r
				scanErr <- e
			}()
			select {
			case <-locked:
			case err := <-scanErr:
				t.Fatalf("scan failed before locked confirmation: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			conn, err := db.Conn(ctx)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			var pid int
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			written := make(chan error, 1)
			go func() { _, err := conn.ExecContext(ctx, tt.write, showPurchase); written <- err }()
			require.Eventually(t, func() bool {
				var waiting bool
				err := db.QueryRowContext(ctx, `SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting)
				return err == nil && waiting
			}, 2*time.Second, 10*time.Millisecond)
			select {
			case err := <-written:
				t.Fatalf("financial writer interleaved before receipt commit: %v", err)
			default:
			}
			close(release)
			require.NoError(t, <-scanErr)
			require.NoError(t, <-written)
			original := <-result
			require.Equal(t, "confirmed", original.Original.Outcome)
			require.Equal(t, 30000, original.Original.AcknowledgedPriceCents)
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.Equal(t, original.Original.ItemID, items[0].ID)
			require.Equal(t, original.Original.ConfirmedAt, items[0].PackedAt)
			require.EqualValues(t, 1, items[0].Version)
			scanReceiptCount(t, db, 1)
			replay, err := sp.NewService(NewShowPrepStore(db.DB), nil, nil).ScanConfirm(ctx, showList, showScanIntent())
			require.NoError(t, err)
			require.Equal(t, original.Original, replay.Original)
			require.Equal(t, "unavailable", replay.Current.State)
		})
	}
}
