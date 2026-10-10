package showprep

import (
	"context"
	"slices"
	"strings"
)

// ConfirmExisting is explicit exact-cohort consent, not a best-effort bulk add.
// Stored membership binds the command; asking/support are read at its commit.
func (s *Service) ConfirmExisting(ctx context.Context, listID string, cmd ConfirmExistingCommand) (ConfirmExistingResult, error) {
	if !validLifecycleCommand(listID, cmd.CommandID, cmd.ExpectedState) || cmd.ItemIDs == nil {
		return ConfirmExistingResult{}, ErrInvalid
	}
	cmd.ItemIDs = slices.Clone(cmd.ItemIDs)
	slices.Sort(cmd.ItemIDs)
	for i, id := range cmd.ItemIDs {
		if !canonicalScanID(id) || (i > 0 && cmd.ItemIDs[i-1] == id) {
			return ConfirmExistingResult{}, ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return ConfirmExistingResult{}, err
	}
	hash := Fingerprint(struct {
		Kind, ListID string
		Command      ConfirmExistingCommand
	}{"confirm_existing", listID, cmd})
	result := ConfirmExistingResult{ListID: listID, CommandID: cmd.CommandID, ConfirmedItemIDs: slices.Clone(cmd.ItemIDs)}
	err := s.store.Within(ctx, func(tx Session) error {
		receipt, found, err := tx.GetCommandReceipt(ctx, cmd.CommandID)
		if err != nil {
			return err
		}
		if found {
			if receipt.ListID != listID || receipt.Kind != "confirm_existing" || receipt.PayloadHash != hash {
				return ErrConflict
			}
			result = *receipt.Transition
			result.ConfirmedItemIDs = slices.Clone(result.ConfirmedItemIDs)
			result.Replay = true
			return nil
		}
		list, err := liveShowList(ctx, tx, listID)
		if err != nil {
			return err
		}
		items, err := tx.GetItems(ctx, listID)
		if err != nil {
			return err
		}
		memberIDs := make([]string, 0, len(items))
		purchaseIDs := make([]string, 0, len(items))
		for _, item := range items {
			memberIDs = append(memberIDs, item.ID)
			purchaseIDs = append(purchaseIDs, item.PurchaseID)
		}
		slices.Sort(memberIDs)
		if !slices.Equal(memberIDs, cmd.ItemIDs) || ListStateFingerprint(list, items) != cmd.ExpectedState {
			return ErrConflict
		}
		slices.Sort(purchaseIDs)
		purchaseIDs = slices.Compact(purchaseIDs)
		if err := tx.LockPurchases(ctx, purchaseIDs); err != nil {
			return err
		}
		now := s.now()
		es, err := evaluateBatch(ctx, tx, purchaseIDs, now, true)
		if err != nil {
			return err
		}
		byID := make(map[string]Evaluation, len(es))
		for _, e := range es {
			byID[e.PurchaseID] = e
		}
		for _, item := range items {
			e := byID[item.PurchaseID]
			if e.Availability != Ready || item.CertNumber == "" || item.CertNumber != e.CertNumber || !strings.EqualFold(strings.TrimSpace(item.Grader), e.Grader) || item.Grade != e.Grade {
				return ErrConflict
			}
			if err := validateTransitionCert(ctx, tx, item, purchaseIDs); err != nil {
				return err
			}
		}
		// Exact-cert inserts elsewhere do not conflict with these row locks. Repeat
		// live reads after evaluation and all initial association validation, and
		// reject any collision observed in the same session even if now removed.
		for _, item := range items {
			if err := validateTransitionCert(ctx, tx, item, purchaseIDs); err != nil {
				return err
			}
			if tx.CertCollisionObserved(item.CertNumber) {
				return ErrConflict
			}
		}
		for _, item := range items {
			e := byID[item.PurchaseID]
			if item.PackedAt == "" {
				item.PackedAt = Timestamp(now)
			}
			item.AcknowledgedPriceCents = e.LocalPriceCents
			item.AcknowledgedStatus = e.Status
			item.LastCommand = hash
			item.Version++
			if err := tx.SaveItem(ctx, listID, item); err != nil {
				return err
			}
		}
		return tx.InsertCommandReceipt(ctx, CommandReceipt{CommandID: cmd.CommandID, ListID: listID, Kind: "confirm_existing", PayloadHash: hash, Transition: &result})
	})
	if err != nil {
		return ConfirmExistingResult{}, err
	}
	return result, nil
}

func validateTransitionCert(ctx context.Context, tx Session, item Item, ids []string) error {
	candidates, err := tx.ReadCertPurchaseIDs(ctx, item.CertNumber)
	if err != nil {
		return err
	}
	if len(candidates) != 1 || candidates[0] != item.PurchaseID {
		// A business rejection rolls show writes back but keeps actual observations.
		if _, err := tx.ObservePriceAssociations(ctx, ids); err != nil {
			return err
		}
		return ErrConflict
	}
	return nil
}
