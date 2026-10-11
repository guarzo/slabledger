package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

const showDeleteCommand = "88888888-8888-4888-8888-888888888888"

func deletionIntent(t *testing.T, ctx context.Context, svc *sp.Service) sp.DeleteListCommand {
	t.Helper()
	d, err := svc.ListDetail(ctx, showList)
	require.NoError(t, err)
	return sp.DeleteListCommand{CommandID: showDeleteCommand, ExpectedState: d.StateFingerprint}
}

func TestShowPrepDeleteTerminalLifetime(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{true: "empty", false: "member and receipt"}[empty], func(t *testing.T) {
			db, store, svc := setupScanService(t)
			ctx := context.Background()
			if !empty {
				_, err := svc.ScanConfirm(ctx, showList, showScanIntent())
				require.NoError(t, err)
			}
			cmd := deletionIntent(t, ctx, svc)
			result, err := svc.DeleteList(ctx, showList, cmd)
			require.NoError(t, err)
			require.Equal(t, sp.DeleteListResult{ListID: showList, Deleted: true}, result)
			_, err = store.GetList(ctx, showList)
			require.ErrorIs(t, err, sp.ErrNotFound)
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Empty(t, items)
			scanReceiptCount(t, db, 0)
			svc = sp.NewService(NewShowPrepStore(db.DB), nil, nil)
			replay, err := svc.DeleteList(ctx, showList, cmd)
			require.NoError(t, err)
			result.Replay = true
			require.Equal(t, result, replay)
			for _, name := range []string{"Scan", "Changed"} {
				_, err = svc.CreateList(ctx, showList, name)
				require.ErrorIs(t, err, sp.ErrRetired)
				require.NotErrorIs(t, err, sp.ErrNotFound)
			}
			_, err = svc.ScanConfirm(ctx, showList, showScanIntent())
			require.ErrorIs(t, err, sp.ErrRetired)
			require.NotErrorIs(t, err, sp.ErrNotFound)
			_, err = svc.ConfirmExisting(ctx, showList, sp.ConfirmExistingCommand{CommandID: showTransitionCommand, ExpectedState: cmd.ExpectedState, ItemIDs: []string{}})
			require.ErrorIs(t, err, sp.ErrRetired)
			altered := cmd
			altered.ExpectedState = "changed"
			_, err = svc.DeleteList(ctx, showList, altered)
			require.ErrorIs(t, err, sp.ErrConflict)
			altered = cmd
			altered.CommandID = showTransitionCommand
			_, err = svc.DeleteList(ctx, showList, altered)
			require.ErrorIs(t, err, sp.ErrRetired)
			var count int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_retired_lists WHERE list_id=$1 AND delete_command_id=$2`, showList, showDeleteCommand).Scan(&count))
			require.Equal(t, 1, count)
			// Financial data/evidence holds are not owned by the retired show.
			ps, err := store.ReadPurchases(ctx, []string{showPurchase})
			require.NoError(t, err)
			require.Len(t, ps, 1)
		})
	}
}

func TestShowPrepDeleteRejectsStaleState(t *testing.T) {
	for _, change := range []string{"rename", "remove", "recheck", "new scan"} {
		t.Run(change, func(t *testing.T) {
			db, store, svc := setupScanService(t)
			ctx := context.Background()
			first, err := svc.ScanConfirm(ctx, showList, showScanIntent())
			require.NoError(t, err)
			cmd := deletionIntent(t, ctx, svc)
			switch change {
			case "rename":
				_, err = svc.RenameList(ctx, showList, "Changed")
				require.NoError(t, err)
			case "remove":
				require.NoError(t, svc.RemoveItem(ctx, showList, first.Original.ItemID))
			case "recheck":
				_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=40000 WHERE id=$1`, showPurchase)
				require.NoError(t, err)
				next := showScanIntent()
				next.CommandID = showTransitionCommand
				_, err = svc.ScanConfirm(ctx, showList, next)
				require.NoError(t, err)
			case "new scan":
				seedShowPurchase(t, db, showOther, "camp-other", "87654321", "PSA")
				next := sp.ScanCommand{CommandID: showTransitionCommand, CertNumber: "87654321"}
				_, err = svc.ScanConfirm(ctx, showList, next)
				require.NoError(t, err)
			}
			before, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			listBefore, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			_, err = svc.DeleteList(ctx, showList, cmd)
			require.ErrorIs(t, err, sp.ErrConflict)
			after, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, before, after)
			listAfter, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, listBefore, listAfter)
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				_, found, err := tx.GetRetiredList(ctx, showList)
				require.NoError(t, err)
				require.False(t, found)
				return nil
			}))
		})
	}
}

func TestShowPrepDeleteFailureRollback(t *testing.T) {
	db, store, svc := setupScanService(t)
	ctx := context.Background()
	_, err := svc.ScanConfirm(ctx, showList, showScanIntent())
	require.NoError(t, err)
	cmd := deletionIntent(t, ctx, svc)
	before, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_show_deletion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'delete unavailable'; END; $$; CREATE TRIGGER reject_show_deletion BEFORE DELETE ON showprep_lists FOR EACH ROW EXECUTE FUNCTION reject_show_deletion()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS reject_show_deletion() CASCADE`)
		require.NoError(t, err)
	})
	_, err = svc.DeleteList(ctx, showList, cmd)
	require.ErrorContains(t, err, "delete unavailable")
	after, err := store.GetItems(ctx, showList)
	require.NoError(t, err)
	require.Equal(t, before, after)
	scanReceiptCount(t, db, 1)
	require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
		_, found, err := tx.GetRetiredList(ctx, showList)
		require.NoError(t, err)
		require.False(t, found)
		return nil
	}))
}

// Pause after the real delete writes, before commit; creation waits on the same
// feature lock, then sees the durable tombstone rather than recreating the UUID.
func TestShowPrepDeleteSerializesDelayedCreate(t *testing.T) {
	db, store, svc := setupScanService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
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
		t.Fatalf("delete before pause: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	created := make(chan error, 1)
	go func() { _, err := svc.CreateList(ctx, showList, "Scan"); created <- err }()
	waitShowPrepQueryLock(t, ctx, db, `SELECT pg_advisory_xact_lock(731946)%`)
	select {
	case err := <-created:
		t.Fatalf("create crossed delete transaction: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-deleted)
	require.ErrorIs(t, <-created, sp.ErrRetired)
	replay, err := svc.DeleteList(ctx, showList, cmd)
	require.NoError(t, err)
	require.True(t, replay.Replay)
}
