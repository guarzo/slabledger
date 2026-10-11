package postgres

import (
	"context"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

const validShowTransitionPayload = `{"commandId":"55555555-5555-4555-8555-555555555555","listId":"33333333-3333-4333-8333-333333333333","confirmedItemIds":[],"replay":false}`
const validShowRejectedPayload = `{"commandId":"55555555-5555-4555-8555-555555555555","listId":"33333333-3333-4333-8333-333333333333","certNumber":"12345678","outcome":"rejected","code":"no_match","itemId":"","purchaseId":"","cardName":"","confirmedAt":"","acknowledgedPriceCents":0,"acknowledgedStatus":""}`

func TestShowPrepReceiptInvalidWriteIsStorageError(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*sp.CommandReceipt)
	}{
		{"missing scan", func(r *sp.CommandReceipt) { r.Scan = nil }},
		{"wrong command", func(r *sp.CommandReceipt) { r.Scan.CommandID = showOther }},
		{"wrong list", func(r *sp.CommandReceipt) { r.Scan.ListID = showOther }},
		{"wrong kind", func(r *sp.CommandReceipt) { r.Kind = "confirm_existing" }},
		{"both result kinds", func(r *sp.CommandReceipt) { r.Transition = &sp.ConfirmExistingResult{} }},
		{"empty cert", func(r *sp.CommandReceipt) { r.Scan.CertNumber = "" }},
		{"empty success member", func(r *sp.CommandReceipt) { r.Scan.ItemID = "" }},
		{"empty success purchase", func(r *sp.CommandReceipt) { r.Scan.PurchaseID = "" }},
		{"empty confirmation time", func(r *sp.CommandReceipt) { r.Scan.ConfirmedAt = "" }},
		{"invalid confirmation time", func(r *sp.CommandReceipt) { r.Scan.ConfirmedAt = "not-a-time" }},
		{"empty success status", func(r *sp.CommandReceipt) { r.Scan.AcknowledgedStatus = "" }},
		{"unsupported success status", func(r *sp.CommandReceipt) { r.Scan.AcknowledgedStatus = "unexpected" }},
		{"code on successful scan", func(r *sp.CommandReceipt) { r.Scan.Code = "sold" }},
		{"empty rejection code", func(r *sp.CommandReceipt) { r.Scan.Outcome = "rejected" }},
		{"unsupported rejection code", func(r *sp.CommandReceipt) { r.Scan.Outcome = "rejected"; r.Scan.Code = "unexpected" }},
		{"missing transition cohort", func(r *sp.CommandReceipt) {
			r.Kind, r.Scan = "confirm_existing", nil
			r.Transition = &sp.ConfirmExistingResult{CommandID: showCommand, ListID: showList}
		}},
		{"empty transition member", func(r *sp.CommandReceipt) {
			r.Kind, r.Scan = "confirm_existing", nil
			r.Transition = &sp.ConfirmExistingResult{CommandID: showCommand, ListID: showList, ConfirmedItemIDs: []string{""}}
		}},
		{"duplicate transition member", func(r *sp.CommandReceipt) {
			r.Kind, r.Scan = "confirm_existing", nil
			r.Transition = &sp.ConfirmExistingResult{CommandID: showCommand, ListID: showList, ConfirmedItemIDs: []string{showPurchase, showPurchase}}
		}},
		{"transition replay is not original history", func(r *sp.CommandReceipt) {
			r.Kind, r.Scan = "confirm_existing", nil
			r.Transition = &sp.ConfirmExistingResult{CommandID: showCommand, ListID: showList, ConfirmedItemIDs: []string{}, Replay: true}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			_, err := store.CreateList(ctx, showList, "Invalid write")
			require.NoError(t, err)
			receipt := sp.CommandReceipt{CommandID: showCommand, ListID: showList, Kind: "scan", PayloadHash: "hash", Scan: &sp.ScanOutcome{
				CommandID: showCommand, ListID: showList, CertNumber: "12345678", Outcome: "confirmed",
				ItemID: showPurchase, PurchaseID: "p", CardName: "Card", ConfirmedAt: "2026-10-09T12:00:00Z", AcknowledgedStatus: sp.NoListedPrice,
			}}
			tt.change(&receipt)
			err = store.Within(ctx, func(tx sp.Session) error { return tx.InsertCommandReceipt(ctx, receipt) })
			require.ErrorContains(t, err, "encode show command receipt")
			require.NotErrorIs(t, err, sp.ErrNotFound)
			var count int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_command_receipts`).Scan(&count))
			require.Zero(t, count, "invalid input must never commit a receipt")
		})
	}
}

func TestShowPrepScanReceiptValidRoundTrips(t *testing.T) {
	for _, tt := range []struct {
		name, outcome, code string
		status              sp.Status
		price               int
	}{
		{"confirmed", "confirmed", "", sp.Supported, 12345},
		{"already confirmed", "already_confirmed", "", sp.ThinEvidence, 12345},
		{"rechecked", "rechecked", "", sp.NeedsReview, 12345},
		{"missing asking allowed", "confirmed", "", sp.NoListedPrice, 0},
		{"negative asking preserved", "confirmed", "", sp.NoListedPrice, -1},
		{"no recent comps allowed", "confirmed", "", sp.NoRecentComps, 12345},
		{"below target allowed", "confirmed", "", sp.BelowTarget, 12345},
		{"mixed evidence allowed", "confirmed", "", sp.MixedEvidence, 12345},
		{"contents changed", "rejected", "contents_changed", "", 0},
		{"no match", "rejected", "no_match", "", 0},
		{"ambiguous cert", "rejected", "ambiguous_cert", "", 0},
		{"identity changed", "rejected", "identity_changed", "", 0},
		{"not received", "rejected", "not_received", "", 0},
		{"sold", "rejected", "sold", "", 0},
		{"refunded", "rejected", "refunded", "", 0},
		{"campaign closed", "rejected", "campaign_closed", "", 0},
		{"removed", "rejected", "removed", "", 0},
		{"unknown availability", "rejected", "unknown_availability", "", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			_, err := store.CreateList(ctx, showList, "Round trip")
			require.NoError(t, err)
			scan := &sp.ScanOutcome{CommandID: showCommand, ListID: showList, CertNumber: "12345678", Outcome: tt.outcome, Code: tt.code, AcknowledgedStatus: tt.status, AcknowledgedPriceCents: tt.price}
			if tt.outcome != "rejected" {
				scan.ItemID, scan.PurchaseID, scan.CardName, scan.ConfirmedAt = showPurchase, "p", "Card", "2026-10-09T12:00:00Z"
			}
			receipt := sp.CommandReceipt{CommandID: showCommand, ListID: showList, Kind: "scan", PayloadHash: "hash", Scan: scan}
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error { return tx.InsertCommandReceipt(ctx, receipt) }))
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				got, found, err := tx.GetCommandReceipt(ctx, showCommand)
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, receipt, got)
				return nil
			}))
		})
	}
}
