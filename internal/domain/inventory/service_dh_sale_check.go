package inventory

import "context"

// DHSaleCheck is an observation, not authorization to return or list inventory.
// The POST must still verify the captured target, sale and durable mutation state.
type DHSaleCheck struct {
	Status     string               `json:"status"`
	Resolvable bool                 `json:"resolvable"`
	Reason     string               `json:"reason"`
	Target     ReturnTargetIdentity `json:"target"`
}

// CheckDHSale reads durable state before and after the provider observation. It
// never creates/settles an attempt or interprets a status read as a return receipt.
func (s *ConfirmedReturnService) CheckDHSale(ctx context.Context, id string) (*DHSaleCheck, error) {
	state, err := s.GetReturnState(ctx, id)
	if err != nil {
		return nil, err
	}
	if state == nil || state.Purchase == nil {
		return nil, NewReturnConflict("coordination_unavailable", "return state is unavailable")
	}
	check := &DHSaleCheck{Target: ReturnTargetIdentity{DHInventoryID: state.Purchase.DHInventoryID, CertNumber: state.Purchase.CertNumber, Grader: state.Purchase.Grader}}
	if reason := dhSaleCheckHold(state); reason != "" {
		check.Reason = reason
		return check, nil
	}
	if !DHDependenciesPresent(s.remote) {
		return nil, NewReturnConflict("coordination_unavailable", "DH sale check requires DH capability")
	}
	status, err := s.remote.GetReturnInventoryStatus(ctx, check.Target.DHInventoryID, check.Target.CertNumber)
	if err != nil {
		return nil, err
	}
	if status != "sold" && status != "in_stock" && status != "listed" {
		return nil, NewReturnConflict("return_preflight_conflict", "DH sale check returned an unknown inventory status")
	}
	fresh, err := s.GetReturnState(ctx, id)
	if err != nil {
		return nil, err
	}
	if fresh == nil || fresh.Purchase == nil {
		return nil, NewReturnConflict("coordination_unavailable", "return state is unavailable")
	}
	// A changed episode (even a completed non-null-sale episode) can make a
	// proposed null-sale POST replay historical work rather than start a return.
	if fresh.Purchase.ID != state.Purchase.ID || fresh.Purchase.DHInventoryID != check.Target.DHInventoryID || fresh.Purchase.CertNumber != check.Target.CertNumber || fresh.Purchase.Grader != check.Target.Grader ||
		fresh.Sale != nil || fresh.Operation != nil || fresh.PrecedingAttempt != nil {
		check.Reason = "state_changed"
		return check, nil
	}
	check.Status = status
	if status == "sold" {
		check.Resolvable = true
	} else {
		check.Reason = "not_sold"
	}
	return check, nil
}

func dhSaleCheckHold(state *ConfirmedReturnState) string {
	if state.Sale != nil {
		return "local_sale_present"
	}
	if state.PrecedingAttempt != nil {
		return "mutation_pending"
	}
	if state.Operation != nil {
		return "return_episode_exists"
	}
	if state.Purchase.DHInventoryID <= 0 || state.Purchase.CertNumber == "" || state.Purchase.Grader == "" {
		return "target_unavailable"
	}
	return ""
}
