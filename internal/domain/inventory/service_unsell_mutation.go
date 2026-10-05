package inventory

import (
	"context"
	"encoding/json"
	"time"
)

// ConfirmedUnsellCAS is the narrow legacy seam. It owns its own committed
// preparation, and never calls Run from inside return ownership.
type ConfirmedUnsellCAS interface {
	DeleteSaleByPurchaseIDCAS(context.Context, string, string) error
}

func (s *service) DeleteSaleByPurchaseIDCAS(ctx context.Context, id, expected string) error {
	if !s.mutationRequired {
		return NewReturnConflict("coordination_unavailable", "confirmed legacy void requires coordination")
	}
	return s.deleteSaleCoordinated(ctx, id, expected)
}
func (s *service) deleteSaleCoordinated(ctx context.Context, id, expected string) error {
	if !DHDependenciesPresent(s.mutationScope, s.mutationRepo, s.mutationGuards) {
		return NewReturnConflict("coordination_unavailable", "un-sell coordination required")
	}
	if e := s.mutationRepo.AssertDHPreparationAllowed(ctx); e != nil {
		return e
	}
	if handled, e := s.recoverSaleBeforeUnsell(ctx, id, expected); handled || e != nil {
		return e
	}
	var attempt *DHMutationAttempt
	var captured string
	e := s.mutationScope.WithPurchaseMutation(ctx, id, func(c context.Context) error {
		sa, e := s.sales.GetSaleByPurchaseID(c, id)
		if e != nil {
			return e
		}
		if expected != "" && sa.ID != expected {
			return NewReturnConflict("sale_precondition_failed", "current sale differs from confirmed sale")
		}
		p, e := s.purchases.GetPurchase(c, id)
		if e != nil {
			return e
		}
		if e := s.mutationGuards.AssertMutationAllowed(c, id); e != nil {
			return e
		}
		captured = sa.ID
		payload, _ := json.Marshal(struct {
			SaleID, Handle, Order string
			Request               DHSaleRequest
			Reset                 bool
		}{sa.ID, sa.DHSaleID, sa.OrderID, persistedDHSaleRequest(sa, p, sa.DHIdempotencyKey), p.DHStatus == DHStatusSold})
		attempt, e = s.mutationRepo.PrepareDHMutation(c, id, DHMutationRequest{Kind: "void", Phase: "recover_void_reset_delete", PayloadIdentity: string(payload)})
		return e
	})
	if e != nil {
		return e
	}
	// Compatibility: unlike Run, remote uncertainty is not a callback error.
	// Local reset/delete may commit under the owned OPEN marker, but only a
	// verified whole void outcome settles it. No false rejection/success receipt.
	e = s.mutationScope.WithPurchaseMutation(ctx, id, func(c context.Context) error {
		if e := s.mutationRepo.OwnDHMutation(c, id, attempt); e != nil {
			return e
		}
		sa, e := s.sales.GetSaleByPurchaseID(c, id)
		if e != nil {
			return e
		}
		if sa.ID != captured || (expected != "" && sa.ID != expected) {
			return NewReturnConflict("sale_precondition_failed", "sale changed before un-sell execution")
		}
		p, e := s.purchases.GetPurchase(c, id)
		if e != nil {
			return e
		}
		payload, _ := json.Marshal(struct {
			SaleID, Handle, Order string
			Request               DHSaleRequest
			Reset                 bool
		}{sa.ID, sa.DHSaleID, sa.OrderID, persistedDHSaleRequest(sa, p, sa.DHIdempotencyKey), p.DHStatus == DHStatusSold})
		if string(payload) != attempt.PayloadIdentity {
			return NewReturnConflict("identity_conflict", "prepared un-sell request changed")
		}
		handle := sa.DHSaleID
		certain := true
		if handle == "" && sa.DHIdempotencyKey != "" && p.DHInventoryID != 0 {
			if s.dhSaleRecorder == nil {
				certain = false
			} else {
				result, remoteErr := s.dhSaleRecorder.RecordInventorySale(c, persistedDHSaleRequest(sa, p, sa.DHIdempotencyKey))
				if remoteErr != nil || !validateRecoveredSale(result, p.DHInventoryID) {
					certain = false
					if remoteErr != nil {
						s.flagDHUnsellFailure(c, p, "recover dh sale handle", remoteErr)
					}
				} else {
					handle = result.DHSaleID
				}
			}
		}
		if handle != "" {
			verified, ok := s.dhSaleRecorder.(DHVerifiedSaleVoider)
			if !ok || !DHDependenciesPresent(verified) {
				certain = false
			} else if remoteErr := verified.VoidInventorySaleVerified(c, handle, p.DHInventoryID, "un-sell"); remoteErr != nil {
				certain = false
				s.flagDHUnsellFailure(c, p, "void on dh", remoteErr)
			}
		}
		if handle != "" || (p.DHInventoryID != 0 && p.DHStatus == DHStatusSold) {
			if e := s.purchases.ResetDHFieldsForRelistAfterVoid(c, id); e != nil {
				return e
			}
		}
		if e := s.sales.DeleteSaleByPurchaseID(c, id); e != nil {
			return e
		}
		if certain {
			return s.mutationRepo.SettleDHMutation(c, id, attempt, DHMutationSettlement{Outcome: "succeeded", Receipt: "whole-void-and-local-unsell-completed:" + handle})
		}
		return nil
	})
	if e != nil && ctx.Err() != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return s.mutationScope.WithPurchaseMutation(cleanup, id, func(c context.Context) error {
			if e := s.mutationRepo.OwnDHMutation(c, id, attempt); e != nil {
				return e
			}
			current, e := s.sales.GetSaleByPurchaseID(c, id)
			if e != nil {
				return e
			}
			if current.ID != captured || expected != "" && current.ID != expected {
				return NewReturnConflict("sale_precondition_failed", "sale changed before canceled un-sell cleanup")
			}
			p, e := s.purchases.GetPurchase(c, id)
			if e != nil {
				return e
			}
			if p.DHInventoryID != 0 && p.DHStatus == DHStatusSold {
				if e := s.purchases.ResetDHFieldsForRelistAfterVoid(c, id); e != nil {
					return e
				}
			}
			return s.sales.DeleteSaleByPurchaseID(c, id)
		})
	}
	return e
}

// Keep best-effort sold UI updates conditional on the actual current sale.
func (s *service) markCurrentSaleSold(ctx context.Context, sa *Sale, p *Purchase) error {
	if !s.mutationRequired {
		return s.purchases.UpdatePurchaseDHStatus(ctx, sa.PurchaseID, DHStatusSold)
	}
	if s.mutationGuards == nil {
		return NewReturnConflict("coordination_unavailable", "sale status coordination required")
	}
	at, e := s.mutationGuards.ObservationTime(ctx)
	if e != nil {
		return e
	}
	_, e = s.mutationGuards.ApplyDHSoldObservation(ctx, sa.PurchaseID, sa.ID, p.DHInventoryID, at, func(c context.Context) error {
		return s.purchases.UpdatePurchaseDHStatus(c, sa.PurchaseID, DHStatusSold)
	})
	return e
}
