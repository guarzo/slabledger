package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Competing feature commands wait for full-cohort receipt commit. The stale
// delete cannot erase new consent; removal/replay cannot reconfirm withdrawn rows.
func TestShowPrepTransitionSerializesFeatureCommands(t *testing.T) {
	for _, operation := range []string{"remove", "delete", "scan", "transition retry"} {
		t.Run(operation, func(t *testing.T) {
			db, store, svc, cmd := setupTransitionService(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			deleteCmd := deletionIntent(t, ctx, svc)
			locked, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
				return store.Within(ctx, func(tx sp.Session) error {
					m := scanSessionMock(tx)
					m.InsertCommandReceiptFn = func(ctx context.Context, r sp.CommandReceipt) error {
						close(locked)
						select {
						case <-release:
							return tx.InsertCommandReceipt(ctx, r)
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					return fn(m)
				})
			}}
			transitioned := make(chan error, 1)
			go func() {
				_, err := sp.NewService(controlled, nil, func() time.Time { return showScanNow }).ConfirmExisting(ctx, showList, cmd)
				transitioned <- err
			}()
			select {
			case <-locked:
			case err := <-transitioned:
				t.Fatalf("transition failed before pause: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			competed := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "remove":
					err = svc.RemoveItem(ctx, showList, cmd.ItemIDs[0])
				case "delete":
					_, err = svc.DeleteList(ctx, showList, deleteCmd)
				case "scan":
					_, err = svc.ScanConfirm(ctx, showList, showScanIntent())
				case "transition retry":
					_, err = svc.ConfirmExisting(ctx, showList, cmd)
				}
				competed <- err
			}()
			waitShowPrepQueryLock(t, ctx, db, `SELECT pg_advisory_xact_lock(731946)%`)
			close(release)
			require.NoError(t, <-transitioned)
			if operation == "delete" {
				require.ErrorIs(t, <-competed, sp.ErrConflict)
			} else {
				require.NoError(t, <-competed)
			}
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			want := 2
			if operation == "remove" {
				want = 1
			}
			require.Len(t, items, want)
			for _, i := range items {
				require.Equal(t, "2026-10-09T12:00:00Z", i.PackedAt)
				require.EqualValues(t, 2, i.Version)
			}
			receipts := 1
			if operation == "scan" {
				receipts = 2
			}
			scanReceiptCount(t, db, receipts)
			before := items
			replay, err := svc.ConfirmExisting(ctx, showList, cmd)
			require.NoError(t, err)
			require.True(t, replay.Replay)
			require.Len(t, replay.ConfirmedItemIDs, 2)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestShowPrepDeleteSerializesPendingCommands(t *testing.T) {
	for _, operation := range []string{"scan replay", "remove", "transition"} {
		t.Run(operation, func(t *testing.T) {
			db, store, svc := setupScanService(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			scan, err := svc.ScanConfirm(ctx, showList, showScanIntent())
			require.NoError(t, err)
			cmd := deletionIntent(t, ctx, svc)
			retired, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
				return store.Within(ctx, func(tx sp.Session) error {
					m := scanSessionMock(tx)
					m.RetireListFn = func(ctx context.Context, r sp.RetiredList) error {
						if err := tx.RetireList(ctx, r); err != nil {
							return err
						}
						close(retired)
						select {
						case <-release:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					return fn(m)
				})
			}}
			deleted := make(chan error, 1)
			go func() { _, err := sp.NewService(controlled, nil, nil).DeleteList(ctx, showList, cmd); deleted <- err }()
			select {
			case <-retired:
			case err := <-deleted:
				t.Fatalf("delete failed before pause: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			competed := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "scan replay":
					_, err = svc.ScanConfirm(ctx, showList, showScanIntent())
				case "remove":
					err = svc.RemoveItem(ctx, showList, scan.Original.ItemID)
				case "transition":
					_, err = svc.ConfirmExisting(ctx, showList, sp.ConfirmExistingCommand{CommandID: showTransitionCommand, ExpectedState: cmd.ExpectedState, ItemIDs: []string{scan.Original.ItemID}})
				}
				competed <- err
			}()
			waitShowPrepQueryLock(t, ctx, db, `SELECT pg_advisory_xact_lock(731946)%`)
			close(release)
			require.NoError(t, <-deleted)
			if operation == "remove" {
				require.ErrorIs(t, <-competed, sp.ErrNotFound)
			} else {
				require.ErrorIs(t, <-competed, sp.ErrRetired)
			}
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Empty(t, items)
			scanReceiptCount(t, db, 0)
		})
	}
}

func TestShowPrepTransitionBlocksFinancialWriterThroughReceipt(t *testing.T) {
	for _, tt := range []struct{ name, write string }{
		{"sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-10-09')`},
		{"refund", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`},
		{"receipt removal", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`},
		{"closure", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`},
		{"delete", `DELETE FROM campaign_purchases WHERE id=$1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc, cmd := setupTransitionService(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			locked, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			controlled := &mocks.ShowPrepStoreMock{WithinFn: func(ctx context.Context, fn func(sp.Session) error) error {
				return store.Within(ctx, func(tx sp.Session) error {
					m := scanSessionMock(tx)
					m.InsertCommandReceiptFn = func(ctx context.Context, r sp.CommandReceipt) error {
						close(locked)
						select {
						case <-release:
							return tx.InsertCommandReceipt(ctx, r)
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					return fn(m)
				})
			}}
			transitioned := make(chan error, 1)
			go func() {
				_, err := sp.NewService(controlled, nil, func() time.Time { return showScanNow }).ConfirmExisting(ctx, showList, cmd)
				transitioned <- err
			}()
			select {
			case <-locked:
			case err := <-transitioned:
				t.Fatalf("transition failed before pause: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			conn, err := db.Conn(ctx)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			var pid int
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			written := make(chan error, 1)
			go func() { _, err := conn.ExecContext(ctx, tt.write, showOther); written <- err }()
			require.Eventually(t, func() bool {
				var waiting bool
				err := db.QueryRowContext(ctx, `SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting)
				return err == nil && waiting
			}, 2*time.Second, 10*time.Millisecond)
			close(release)
			require.NoError(t, <-transitioned)
			require.NoError(t, <-written)
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			for _, i := range before {
				require.Equal(t, "2026-10-09T12:00:00Z", i.PackedAt)
				require.EqualValues(t, 2, i.Version)
			}
			replay, err := svc.ConfirmExisting(ctx, showList, cmd)
			require.NoError(t, err)
			require.True(t, replay.Replay)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, before, after)
			scanReceiptCount(t, db, 1)
		})
	}
}
