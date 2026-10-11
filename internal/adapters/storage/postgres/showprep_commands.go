package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

func (s *showPrepSession) GetCommandReceipt(ctx context.Context, id string) (sp.CommandReceipt, bool, error) {
	var receipt sp.CommandReceipt
	var payload []byte
	err := s.q.QueryRowContext(ctx, `SELECT command_id,list_id,kind,payload_hash,result FROM showprep_command_receipts WHERE command_id=$1`, id).Scan(&receipt.CommandID, &receipt.ListID, &receipt.Kind, &receipt.PayloadHash, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return sp.CommandReceipt{}, false, nil
	}
	if err != nil {
		return sp.CommandReceipt{}, false, fmt.Errorf("read show command receipt: %w", err)
	}
	switch receipt.Kind {
	case "scan":
		err = decodeShowReceiptResult(payload, &receipt.Scan, "commandId", "listId", "certNumber", "outcome", "code", "itemId", "purchaseId", "cardName", "confirmedAt", "acknowledgedPriceCents", "acknowledgedStatus")
	case "confirm_existing":
		err = decodeShowReceiptResult(payload, &receipt.Transition, "commandId", "listId", "confirmedItemIds", "replay")
	default:
		err = fmt.Errorf("unknown receipt kind %q", receipt.Kind)
	}
	if err == nil {
		err = validateShowReceipt(receipt)
	}
	if err != nil {
		return sp.CommandReceipt{}, false, fmt.Errorf("decode show command receipt %s: %w", id, err)
	}
	return receipt, true, nil
}
func (s *showPrepSession) InsertCommandReceipt(ctx context.Context, r sp.CommandReceipt) error {
	if err := validateShowReceipt(r); err != nil {
		return fmt.Errorf("encode show command receipt %s: %w", r.CommandID, err)
	}
	var result any = r.Scan
	if r.Kind == "confirm_existing" {
		result = r.Transition
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode show command receipt: %w", err)
	}
	_, err = s.q.ExecContext(ctx, `INSERT INTO showprep_command_receipts(command_id,list_id,kind,payload_hash,result) VALUES($1,$2,$3,$4,$5::jsonb)`, r.CommandID, r.ListID, r.Kind, r.PayloadHash, string(payload))
	if err != nil {
		return fmt.Errorf("insert show command receipt: %w", err)
	}
	return nil
}
func validateShowReceipt(r sp.CommandReceipt) error {
	switch r.Kind {
	case "scan":
		if r.Scan == nil || r.Transition != nil || r.Scan.CommandID != r.CommandID || r.Scan.ListID != r.ListID {
			return errors.New("scan result does not match receipt identity")
		}
		return validateShowScanOutcome(*r.Scan)
	case "confirm_existing":
		if r.Transition == nil || r.Scan != nil || r.Transition.CommandID != r.CommandID || r.Transition.ListID != r.ListID {
			return errors.New("transition result does not match receipt identity")
		}
		if r.Transition.ConfirmedItemIDs == nil || r.Transition.Replay {
			return errors.New("transition result requires an original confirmed item cohort")
		}
		seen := make(map[string]bool, len(r.Transition.ConfirmedItemIDs))
		for _, id := range r.Transition.ConfirmedItemIDs {
			if id == "" || seen[id] {
				return errors.New("transition result contains empty or duplicate item IDs")
			}
			seen[id] = true
		}
		return nil
	default:
		return fmt.Errorf("unknown receipt kind %q", r.Kind)
	}
}

// Stored receipts are complete, kind-specific history, not partial API updates.
// Check presence separately because unmarshalling missing/null scalar fields
// otherwise silently fabricates zero values (including a zero-cent baseline).
func decodeShowReceiptResult(payload []byte, result any, fields ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		return err
	}
	for _, field := range fields {
		value, ok := object[field]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("receipt result requires non-null field %q", field)
		}
	}
	if len(object) != len(fields) {
		return errors.New("receipt result contains foreign fields for its kind")
	}
	return json.Unmarshal(payload, result)
}

func validateShowScanOutcome(r sp.ScanOutcome) error {
	if r.CertNumber == "" {
		return errors.New("scan result requires cert number")
	}
	switch r.Outcome {
	case "confirmed", "already_confirmed", "rechecked":
		if r.Code != "" || r.ItemID == "" || r.PurchaseID == "" || r.ConfirmedAt == "" {
			return errors.New("successful scan result requires confirmation identity/time and no rejection code")
		}
		if _, err := time.Parse(time.RFC3339Nano, r.ConfirmedAt); err != nil {
			return fmt.Errorf("invalid scan confirmation time: %w", err)
		}
		switch r.AcknowledgedStatus {
		case sp.Supported, sp.ThinEvidence, sp.BelowTarget, sp.MixedEvidence, sp.NoRecentComps, sp.NeedsReview, sp.NoListedPrice:
			return nil
		default:
			return fmt.Errorf("invalid scan acknowledged status %q", r.AcknowledgedStatus)
		}
	case "rejected":
		switch r.Code {
		case "contents_changed", "no_match", "ambiguous_cert", "identity_changed", "not_received", "sold", "refunded", "campaign_closed", "removed", "unknown_availability":
			return nil
		default:
			return fmt.Errorf("invalid scan rejection code %q", r.Code)
		}
	default:
		return fmt.Errorf("invalid scan outcome %q", r.Outcome)
	}
}
