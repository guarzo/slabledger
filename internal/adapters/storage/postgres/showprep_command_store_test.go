package postgres

import (
	"context"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

const showCommand = "55555555-5555-4555-8555-555555555555"

func TestShowPrepCommandReceiptLifetime(t *testing.T) {
	for _, tt := range []struct {
		name, kind string
		scan       *sp.ScanOutcome
		transition *sp.ConfirmExistingResult
	}{
		{"scan", "scan", &sp.ScanOutcome{CommandID: showCommand, ListID: showList, Outcome: "confirmed", ItemID: showPurchase, PurchaseID: "deleted-purchase", AcknowledgedPriceCents: 12345, AcknowledgedStatus: sp.ThinEvidence}, nil},
		{"transition", "confirm_existing", nil, &sp.ConfirmExistingResult{CommandID: showCommand, ListID: showList, ConfirmedItemIDs: []string{showPurchase}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			_, err := store.CreateList(ctx, showList, "Receipts")
			require.NoError(t, err)
			seedShowScanMember(t, store)
			receipt := sp.CommandReceipt{CommandID: showCommand, ListID: showList, Kind: tt.kind, PayloadHash: "test-payload", Scan: tt.scan, Transition: tt.transition}
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				_, found, err := tx.GetCommandReceipt(ctx, showCommand)
				require.NoError(t, err)
				require.False(t, found)
				return tx.InsertCommandReceipt(ctx, receipt)
			}))
			// A duplicate UUID cannot overwrite its original result, even on another show.
			require.Error(t, store.Within(ctx, func(tx sp.Session) error { return tx.InsertCommandReceipt(ctx, receipt) }))
			require.NoError(t, store.RemoveItem(ctx, showList, showPurchase))
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				got, found, err := tx.GetCommandReceipt(ctx, showCommand)
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, receipt, got)
				return nil
			}))
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				return tx.RetireList(ctx, sp.RetiredList{ListID: showList, DeleteCommandID: showOther, PayloadHash: "delete", RetiredAt: "2026-10-09T12:00:00Z"})
			}))
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				_, found, err := tx.GetCommandReceipt(ctx, showCommand)
				require.NoError(t, err)
				require.False(t, found)
				return nil
			}))
		})
	}
}

func TestShowPrepReceiptInvalidPayloadIsStorageError(t *testing.T) {
	for _, tt := range []struct{ name, kind, payload string }{
		{"null", "scan", `null`}, {"empty", "scan", `{}`}, {"wrong shape", "scan", `[]`},
		{"wrong command", "scan", `{"commandId":"other","listId":"33333333-3333-4333-8333-333333333333","outcome":"confirmed"}`},
		{"bad outcome", "scan", `{"commandId":"55555555-5555-4555-8555-555555555555","listId":"33333333-3333-4333-8333-333333333333","outcome":"nonsense"}`},
		{"invalid transition", "confirm_existing", `null`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			_, err := store.CreateList(ctx, showList, "Bad receipt")
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `INSERT INTO showprep_command_receipts(command_id,list_id,kind,payload_hash,result) VALUES($1,$2,$3,'hash',$4::jsonb)`, showCommand, showList, tt.kind, tt.payload)
			require.NoError(t, err)
			err = store.Within(ctx, func(tx sp.Session) error { _, _, err := tx.GetCommandReceipt(ctx, showCommand); return err })
			require.Error(t, err)
			require.NotErrorIs(t, err, sp.ErrNotFound)
		})
	}
}

func TestShowPrepReceiptAndMemberRollbackTogether(t *testing.T) {
	for _, tt := range []struct {
		name     string
		rollback bool
	}{{"commit", false}, {"rollback", true}} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			_, err := store.CreateList(ctx, showList, "Atomic")
			require.NoError(t, err)
			err = store.Within(ctx, func(tx sp.Session) error {
				if err := tx.InsertItem(ctx, showList, sp.Item{ID: showPurchase, PurchaseID: "p", AddedAt: "2026-10-09T12:00:00Z", Version: 1}); err != nil {
					return err
				}
				if err := tx.InsertCommandReceipt(ctx, sp.CommandReceipt{CommandID: showCommand, ListID: showList, Kind: "scan", PayloadHash: "hash", Scan: &sp.ScanOutcome{CommandID: showCommand, ListID: showList, Outcome: "confirmed"}}); err != nil {
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
			items, err := store.GetItems(ctx, showList)
			require.NoError(t, err)
			if tt.rollback {
				require.Empty(t, items)
			} else {
				require.Len(t, items, 1)
			}
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				_, found, err := tx.GetCommandReceipt(ctx, showCommand)
				require.NoError(t, err)
				require.Equal(t, !tt.rollback, found)
				return nil
			}))
		})
	}
}
