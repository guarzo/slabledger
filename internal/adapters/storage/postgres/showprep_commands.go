package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

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
		err = json.Unmarshal(payload, &receipt.Scan)
	case "confirm_existing":
		err = json.Unmarshal(payload, &receipt.Transition)
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
		return fmt.Errorf("encode show command receipt: %w", err)
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
		switch r.Scan.Outcome {
		case "confirmed", "already_confirmed", "rechecked", "rejected":
			return nil
		default:
			return fmt.Errorf("invalid scan outcome %q", r.Scan.Outcome)
		}
	case "confirm_existing":
		if r.Transition == nil || r.Scan != nil || r.Transition.CommandID != r.CommandID || r.Transition.ListID != r.ListID {
			return errors.New("transition result does not match receipt identity")
		}
		return nil
	default:
		return fmt.Errorf("unknown receipt kind %q", r.Kind)
	}
}
