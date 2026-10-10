package showprep

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ScanConfirm records consent and its immutable receipt in the same transaction.
// A retry is history, not permission to acknowledge today's asking/support.
func (s *Service) ScanConfirm(ctx context.Context, listID string, cmd ScanCommand) (ScanResult, error) {
	if !canonicalScanID(listID) || !canonicalScanID(cmd.CommandID) || strings.TrimSpace(cmd.CertNumber) == "" || cmd.CertNumber != strings.TrimSpace(cmd.CertNumber) || cmd.WithdrawalGeneration < 0 || cmd.WithdrawalGeneration > MaxWithdrawalGeneration {
		return ScanResult{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	hash := Fingerprint(struct {
		Kind, ListID string
		Command      ScanCommand
	}{"scan", listID, cmd})
	result := ScanResult{Current: ScanCurrent{State: "unknown"}}
	err := s.store.Within(ctx, func(tx Session) error {
		receipt, found, err := tx.GetCommandReceipt(ctx, cmd.CommandID)
		if err != nil {
			return err
		}
		if found {
			if receipt.ListID != listID || receipt.Kind != "scan" || receipt.PayloadHash != hash {
				return ErrConflict
			}
			// Receipt shape is validated by storage; never repair corrupt history.
			result.Original, result.Replay = *receipt.Scan, true
			return nil
		}
		list, err := tx.GetList(ctx, listID)
		if err != nil {
			return err
		}
		result.Original = ScanOutcome{CommandID: cmd.CommandID, ListID: listID, CertNumber: cmd.CertNumber, Outcome: "rejected"}
		finalize := func(code string) error {
			result.Original.Code = code
			return tx.InsertCommandReceipt(ctx, CommandReceipt{CommandID: cmd.CommandID, ListID: listID, Kind: "scan", PayloadHash: hash, Scan: &result.Original})
		}
		if cmd.WithdrawalGeneration != list.WithdrawalGeneration {
			return finalize("contents_changed")
		}
		items, err := tx.GetItems(ctx, listID)
		if err != nil {
			return err
		}
		candidates, err := tx.ReadCertPurchaseIDs(ctx, cmd.CertNumber)
		if err != nil {
			return err
		}
		// Saved cert owners participate even if the live cert changed or vanished.
		ids := slices.Clone(candidates)
		for _, item := range items {
			if item.CertNumber == cmd.CertNumber {
				ids = append(ids, item.PurchaseID)
			}
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		if err := tx.LockPurchases(ctx, ids); err != nil {
			return err
		}
		fresh, err := tx.ReadCertPurchaseIDs(ctx, cmd.CertNumber)
		if err != nil {
			return err
		}
		// Observe before any terminal identity rejection so collisions stay durable.
		if _, err := tx.ObservePriceAssociations(ctx, ids); err != nil {
			return err
		}
		slices.Sort(candidates)
		slices.Sort(fresh)
		if !slices.Equal(candidates, fresh) {
			return finalize("identity_changed")
		}
		if len(candidates) > 1 {
			return finalize("ambiguous_cert")
		}
		if len(ids) == 0 {
			return finalize("no_match")
		}
		if len(ids) != 1 {
			return finalize("identity_changed")
		}
		now := s.now()
		es, err := evaluateBatch(ctx, tx, ids, now, true)
		if err != nil {
			return err
		}
		e := es[0]
		var member Item
		for _, item := range items {
			if item.PurchaseID == ids[0] || item.CertNumber == cmd.CertNumber {
				if e.Availability != Removed && e.Availability != Unknown && (item.PurchaseID != e.PurchaseID || item.CertNumber != e.CertNumber || !strings.EqualFold(strings.TrimSpace(item.Grader), e.Grader) || item.Grade != e.Grade) {
					return finalize("identity_changed")
				}
				member = item
			}
		}
		if e.Availability != Removed && e.Availability != Unknown && e.CertNumber != cmd.CertNumber {
			return finalize("identity_changed")
		}
		if e.Availability != Ready {
			code := string(e.Availability)
			if e.Availability == Unknown {
				code = "unknown_availability"
			}
			return finalize(code)
		}
		// Row locks cannot fence an exact-cert insertion in another campaign.
		// Revalidate after the later association/evidence reads; historical price
		// holds alone must not reject otherwise unique physical identity.
		latest, err := tx.ReadCertPurchaseIDs(ctx, cmd.CertNumber)
		if err != nil {
			return err
		}
		slices.Sort(latest)
		if !slices.Equal(candidates, latest) {
			if _, err := tx.ObservePriceAssociations(ctx, ids); err != nil {
				return err
			}
			if len(latest) > 1 {
				return finalize("ambiguous_cert")
			}
			return finalize("identity_changed")
		}
		if len(candidates) == 0 {
			return finalize("identity_changed")
		}
		if tx.CertCollisionObserved(cmd.CertNumber) {
			return finalize("ambiguous_cert")
		}
		outcome, err := confirmScanMember(ctx, tx, listID, cmd, member, e, now, hash)
		if err != nil {
			return err
		}
		result.Original = outcome
		result.Current = ScanCurrent{State: "present", Evaluation: &e}
		return finalize("")
	})
	if err != nil {
		return ScanResult{}, err
	}
	if result.Replay {
		result.Current = s.scanReplayCurrent(ctx, result.Original)
	}
	return result, nil
}

func canonicalScanID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

func confirmScanMember(ctx context.Context, tx Session, listID string, cmd ScanCommand, item Item, e Evaluation, now time.Time, hash string) (ScanOutcome, error) {
	outcome := "already_confirmed"
	if item.ID == "" {
		item = Item{ID: uuid.NewString(), PurchaseID: e.PurchaseID, CardName: e.CardName, CertNumber: e.CertNumber, Grader: e.Grader, Grade: e.Grade, AddedAt: Timestamp(now), PackedAt: Timestamp(now), Version: 1, AcknowledgedPriceCents: e.LocalPriceCents, AcknowledgedStatus: e.Status, LastCommand: hash}
		outcome = "confirmed"
		if err := tx.InsertItem(ctx, listID, item); err != nil {
			return ScanOutcome{}, err
		}
	} else if item.PackedAt == "" || item.AcknowledgedPriceCents != e.LocalPriceCents || item.AcknowledgedStatus != e.Status {
		outcome = "rechecked"
		if item.PackedAt == "" {
			item.PackedAt, outcome = Timestamp(now), "confirmed"
		}
		item.AcknowledgedPriceCents, item.AcknowledgedStatus = e.LocalPriceCents, e.Status
		item.LastCommand = hash
		item.Version++
		if err := tx.SaveItem(ctx, listID, item); err != nil {
			return ScanOutcome{}, err
		}
	}
	return ScanOutcome{CommandID: cmd.CommandID, ListID: listID, CertNumber: cmd.CertNumber, Outcome: outcome, ItemID: item.ID, PurchaseID: item.PurchaseID, CardName: item.CardName, ConfirmedAt: item.PackedAt, AcknowledgedPriceCents: item.AcknowledgedPriceCents, AcknowledgedStatus: item.AcknowledgedStatus}, nil
}

// The optional replay evaluation is omitted: current presence/availability needs
// no evidence refresh or write-capable association observation. Failed reads must
// not turn a completed command into an unfinalized intent.
func (s *Service) scanReplayCurrent(ctx context.Context, original ScanOutcome) ScanCurrent {
	unknown := ScanCurrent{State: "unknown"}
	if original.ItemID == "" {
		return unknown
	}
	items, err := s.store.GetItems(ctx, original.ListID)
	if err != nil {
		return unknown
	}
	for _, item := range items {
		if item.ID != original.ItemID || item.PurchaseID != original.PurchaseID {
			continue
		}
		ps, err := s.store.ReadPurchases(ctx, []string{original.PurchaseID})
		if err != nil {
			return unknown
		}
		p, ok := ps[original.PurchaseID]
		if !ok {
			return ScanCurrent{State: "unavailable"}
		}
		if AvailabilityOf(p) == Unknown {
			return unknown
		}
		if AvailabilityOf(p) != Ready || p.CertNumber != item.CertNumber || !strings.EqualFold(strings.TrimSpace(p.Grader), item.Grader) || p.Grade != item.Grade {
			return ScanCurrent{State: "unavailable"}
		}
		return ScanCurrent{State: "present"}
	}
	return ScanCurrent{State: "removed"}
}
