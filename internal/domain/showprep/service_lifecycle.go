package showprep

import (
	"context"
	"errors"
	"strings"
)

func validLifecycleCommand(listID, commandID, state string) bool {
	return canonicalScanID(listID) && canonicalScanID(commandID) && state != "" && state == strings.TrimSpace(state)
}

// A tombstone is the deletion receipt: unlike member/command rows it survives
// retirement. Replay must never inspect or recreate the former live state.
func (s *Service) DeleteList(ctx context.Context, listID string, cmd DeleteListCommand) (DeleteListResult, error) {
	if !validLifecycleCommand(listID, cmd.CommandID, cmd.ExpectedState) {
		return DeleteListResult{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return DeleteListResult{}, err
	}
	hash := Fingerprint(struct {
		Kind, ListID string
		Command      DeleteListCommand
	}{"delete", listID, cmd})
	result := DeleteListResult{ListID: listID, Deleted: true}
	err := s.store.Within(ctx, func(tx Session) error {
		retired, found, err := tx.GetRetiredList(ctx, listID)
		if err != nil {
			return err
		}
		if found {
			if retired.DeleteCommandID != cmd.CommandID {
				return ErrRetired
			}
			if retired.PayloadHash != hash {
				return ErrConflict
			}
			result.Replay = true
			return nil
		}
		if _, found, err := tx.GetCommandReceipt(ctx, cmd.CommandID); err != nil {
			return err
		} else if found {
			return ErrConflict
		}
		list, err := tx.GetList(ctx, listID)
		if err != nil {
			return err
		}
		items, err := tx.GetItems(ctx, listID)
		if err != nil {
			return err
		}
		if ListStateFingerprint(list, items) != cmd.ExpectedState {
			return ErrConflict
		}
		return tx.RetireList(ctx, RetiredList{ListID: listID, DeleteCommandID: cmd.CommandID, PayloadHash: hash, RetiredAt: Timestamp(s.now())})
	})
	if err != nil {
		return DeleteListResult{}, err
	}
	return result, nil
}

// Receipts are checked by callers first. Only an absent live show needs the
// tombstone lookup, so unrelated storage errors retain their original context.
func liveShowList(ctx context.Context, tx Session, listID string) (List, error) {
	list, err := tx.GetList(ctx, listID)
	if !errors.Is(err, ErrNotFound) {
		return list, err
	}
	if _, retired, readErr := tx.GetRetiredList(ctx, listID); readErr != nil {
		return List{}, readErr
	} else if retired {
		return List{}, ErrRetired
	}
	return List{}, err
}
