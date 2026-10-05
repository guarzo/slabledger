package inventory

import "context"

type mutationStateReader interface {
	GetReturnState(context.Context, string) (*ConfirmedReturnState, error)
}

// A prior keyed sale is replayed to its own receipt BEFORE preparing a void.
// If it remains uncertain, generic local un-sell still succeeds without sending
// an untracked void. Its old immutable journal remains open for diagnosis.
func (s *service) recoverSaleBeforeUnsell(ctx context.Context, id, expected string) (bool, error) {
	reader, ok := s.mutationRepo.(mutationStateReader)
	if !ok {
		return false, NewReturnConflict("coordination_unavailable", "un-sell state reader required")
	}
	state, e := reader.GetReturnState(ctx, id)
	if e != nil {
		return false, e
	}
	a := state.PrecedingAttempt
	if a == nil || a.Kind != "sale" || a.Phase != "record" {
		return false, nil
	}
	sa := state.Sale
	if sa == nil || (expected != "" && sa.ID != expected) || sa.OrderID != "" || a.Key == "" || sa.DHIdempotencyKey != a.Key || salePayload(sa.ID, persistedDHSaleRequest(sa, state.Purchase, a.Key)) != a.PayloadIdentity {
		return false, NewReturnConflict("sale_precondition_failed", "uncertain sale does not match current un-sell")
	}
	if e := RecordCoordinatedDHSale(ctx, s.mutationCoordinator, s.purchases, s.sales, s.dhSaleRecorder, id, sa.ID, s.idGen); e == nil {
		return false, nil
	}
	e = s.mutationScope.WithPurchaseMutation(ctx, id, func(c context.Context) error {
		if e := s.mutationRepo.OwnDHMutation(c, id, a); e != nil {
			return e
		}
		fresh, e := s.sales.GetSaleByPurchaseID(c, id)
		if e != nil {
			return e
		}
		p, e := s.purchases.GetPurchase(c, id)
		if e != nil {
			return e
		}
		if fresh.ID != sa.ID || fresh.DHIdempotencyKey != a.Key || salePayload(fresh.ID, persistedDHSaleRequest(fresh, p, a.Key)) != a.PayloadIdentity {
			return NewReturnConflict("sale_precondition_failed", "sale changed before local un-sell")
		}
		if p.DHInventoryID != 0 && p.DHStatus == DHStatusSold {
			if e := s.purchases.ResetDHFieldsForRelistAfterVoid(c, id); e != nil {
				return e
			}
		}
		return s.sales.DeleteSaleByPurchaseID(c, id)
	})
	return true, e
}
