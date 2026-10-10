package postgres

import (
	"context"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func seedShowScanMember(t *testing.T, store *ShowPrepStore) sp.Item {
	t.Helper()
	item := sp.Item{ID: showPurchase, PurchaseID: "deleted-purchase", CardName: "Saved card", CertNumber: "123", Grader: "PSA", Grade: 10, AddedAt: "2026-10-09T12:00:00Z", PackedAt: "2026-10-09T12:01:00Z", Version: 1, AcknowledgedPriceCents: 12345, AcknowledgedStatus: sp.ThinEvidence, LastCommand: "scan-hash"}
	require.NoError(t, store.Within(context.Background(), func(tx sp.Session) error { return tx.InsertItem(context.Background(), showList, item) }))
	return item
}

func TestShowPrepRemovalGeneration(t *testing.T) {
	for _, tt := range []struct {
		name           string
		generation     int64
		removeID       string
		wantGeneration int64
		wantItems      int
		conflict       bool
	}{
		{"member removed", 0, showPurchase, 1, 0, false},
		{"absent member", 0, showOther, 0, 1, false},
		{"upper bound", 9007199254740990, showPurchase, 9007199254740991, 0, false},
		{"overflow rolls back deletion", 9007199254740991, showPurchase, 9007199254740991, 1, true},
		{"absent at bound", 9007199254740991, showOther, 9007199254740991, 1, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			_, err := store.CreateList(ctx, showList, "Removal")
			require.NoError(t, err)
			item := seedShowScanMember(t, store)
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, []sp.Item{item}, items, "insert persists full confirmation without extra version increment")
			_, err = db.ExecContext(ctx, `UPDATE showprep_lists SET withdrawal_generation=$2,updated_at='2026-09-01' WHERE id=$1`, showList, tt.generation)
			require.NoError(t, err)
			before, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			for range 2 {
				err = store.RemoveItem(ctx, showList, tt.removeID)
				if tt.conflict {
					require.ErrorIs(t, err, sp.ErrConflict)
				} else {
					require.NoError(t, err)
				}
			}
			got, err := store.GetList(ctx, showList)
			require.NoError(t, err)
			require.Equal(t, tt.wantGeneration, got.WithdrawalGeneration)
			items, err = store.GetItems(ctx, showList)
			require.NoError(t, err)
			require.Len(t, items, tt.wantItems)
			if tt.wantItems == 0 {
				require.NotEqual(t, before.UpdatedAt, got.UpdatedAt)
			} else {
				require.Equal(t, before.UpdatedAt, got.UpdatedAt)
			}
			lists, err := store.Lists(ctx)
			require.NoError(t, err)
			require.Equal(t, []sp.List{got}, lists)
			renamed, err := store.RenameList(ctx, showList, "Renamed")
			require.NoError(t, err)
			require.Equal(t, tt.wantGeneration, renamed.WithdrawalGeneration)
		})
	}
}

func TestShowPrepRetirementLifetime(t *testing.T) {
	for _, tt := range []struct {
		name     string
		rollback bool
	}{{"retired ID cannot resurrect", false}, {"rollback retains live show", true}} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			original, err := store.CreateList(ctx, showList, "Show")
			require.NoError(t, err)
			duplicate, err := store.CreateList(ctx, showList, "Show")
			require.NoError(t, err)
			require.Equal(t, original, duplicate)
			retired := sp.RetiredList{ListID: showList, DeleteCommandID: showCommand, PayloadHash: "delete-hash", RetiredAt: "2026-10-09T12:00:00Z"}
			err = store.Within(ctx, func(tx sp.Session) error {
				_, found, err := tx.GetRetiredList(ctx, showList)
				require.NoError(t, err)
				require.False(t, found)
				if err := tx.RetireList(ctx, retired); err != nil {
					return err
				}
				if tt.rollback {
					return sp.ErrConflict
				}
				return nil
			})
			if tt.rollback {
				require.ErrorIs(t, err, sp.ErrConflict)
			} else {
				require.NoError(t, err)
			}
			_, err = store.GetList(ctx, showList)
			if tt.rollback {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, sp.ErrNotFound)
			}
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				got, found, err := tx.GetRetiredList(ctx, showList)
				require.NoError(t, err)
				require.Equal(t, !tt.rollback, found)
				if found {
					require.Equal(t, retired, got)
				}
				return nil
			}))
			_, err = store.CreateList(ctx, showList, "Show")
			if tt.rollback {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, sp.ErrConflict)
			}
		})
	}
}

func TestShowPrepSaveItemStillUsesVersionCAS(t *testing.T) {
	for _, tt := range []struct {
		name     string
		version  int64
		conflict bool
	}{{"next version", 2, false}, {"stale version", 1, true}, {"skipped version", 3, true}} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			_, err := store.CreateList(ctx, showList, "CAS")
			require.NoError(t, err)
			item := seedShowScanMember(t, store)
			item.Version = tt.version
			item.LastCommand = "new-hash"
			err = store.Within(ctx, func(tx sp.Session) error { return tx.SaveItem(ctx, showList, item) })
			if tt.conflict {
				require.ErrorIs(t, err, sp.ErrConflict)
			} else {
				require.NoError(t, err)
			}
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			if tt.conflict {
				require.Equal(t, int64(1), items[0].Version)
				require.Equal(t, "scan-hash", items[0].LastCommand)
			} else {
				require.Equal(t, item, items[0])
			}
		})
	}
}
