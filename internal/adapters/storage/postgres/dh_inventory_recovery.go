package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// RecoverDHInventory is local administrative recovery, never a provider receipt.
func (s *ConfirmedReturnStore) RecoverDHInventory(ctx context.Context, id, action, actor string) error {
	blocked := inventory.NewReturnConflict("dh_recovery_blocked", "Recovery requires an unlinked authorized return or an in-stock stranded void, without a sale, foreign target or unresolved DH work")
	if strings.TrimSpace(actor) == "" || (action != "relink-returned" && action != "release-void") {
		return blocked
	}
	return s.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		x := executor(owned, s.db)
		// An empty target-owner SELECT cannot row-lock a concurrent new owner.
		// This short local-only lock conflicts with all purchase linkage writes.
		if _, err := x.ExecContext(owned, `LOCK TABLE campaign_purchases IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		state, err := s.returnState(owned, id)
		if err != nil {
			return err
		}
		p, e, a := state.Purchase, state.Operation, state.PrecedingAttempt
		if state.Sale != nil || p.ReceivedAt == nil || p.CertNumber == "" || p.Grader == "" {
			return blocked
		}
		target, status, ignored := p.DHInventoryID, "in_stock", ""
		if action == "relink-returned" {
			if target != 0 || p.DHStatus != "" || p.DHSaleConflict != "" || p.DHSaleConflictAt != nil || e == nil || e.State != "completed" || e.ListingAuthorizedAt == nil || e.PurchaseID != nil || e.CapturedPurchaseID == id || e.Grader != p.Grader || e.CertNumber != p.CertNumber {
				return blocked
			}
			target, status = e.DHInventoryID, "listed"
		} else {
			if target <= 0 || p.DHStatus != inventory.DHStatusInStock || e != nil || a == nil || a.Kind != "void" || a.Phase != "recover_void_reset_delete" || a.Key != "" || a.ReturnOperationID != "" || a.PayloadIdentity == "" || a.Retryable {
				return blocked
			}
			ignored = a.ID
		}
		var unsafe bool
		err = x.QueryRowContext(owned, `SELECT
 EXISTS(SELECT 1 FROM campaign_purchases WHERE dh_inventory_id=$1 AND id<>$2) OR
 EXISTS(SELECT 1 FROM confirmed_dh_returns WHERE state<>'completed' AND
 (dh_inventory_id=$1 OR (grader=$3 AND cert_number=$4) OR captured_purchase_id=$2)) OR
 EXISTS(SELECT 1 FROM dh_mutation_attempts WHERE outcome='open' AND id<>$5 AND
 (dh_inventory_id=$1 OR (grader=$3 AND cert_number=$4) OR captured_purchase_id=$2))`, target, id, p.Grader, p.CertNumber, ignored).Scan(&unsafe)
		if err != nil {
			return err
		}
		if unsafe || target <= 0 {
			return blocked
		}
		audit, err := json.Marshal(map[string]string{"action": action, "actor": actor, "original_provider_outcome": "unknown"})
		if err != nil {
			return err
		}
		if a != nil {
			if err := s.OwnDHMutation(owned, id, a); err != nil {
				return err
			}
			result, err := x.ExecContext(owned, `UPDATE dh_mutation_attempts SET outcome='abandoned',settled_at=clock_timestamp(),receipt=$1
 WHERE id=$2 AND outcome='open' AND receipt='' AND captured_purchase_id=$3 AND dh_inventory_id=$4
 AND grader=$5 AND cert_number=$6 AND kind='void' AND phase='recover_void_reset_delete'
 AND idempotency_key='' AND return_operation_id IS NULL AND payload_identity=$7`, string(audit), a.ID, id, target, p.Grader, p.CertNumber, a.PayloadIdentity)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return blocked
			}
		}
		if _, err := x.ExecContext(owned, `UPDATE campaign_purchases SET dh_inventory_id=$1,dh_status=$2,
 dh_push_status='matched',dh_push_attempts=0,dh_sale_conflict='',dh_sale_conflict_at=NULL,updated_at=clock_timestamp() WHERE id=$3`, target, status, id); err != nil {
			return err
		}
		if err := NewDHEventStore(s.db).Record(owned, dhevents.Event{PurchaseID: id, CertNumber: p.CertNumber, Type: "inventory_recovered", Source: dhevents.SourceManualUI, PrevPushStatus: p.DHPushStatus, NewPushStatus: "matched", PrevDHStatus: string(p.DHStatus), NewDHStatus: status, DHInventoryID: target, DHCardID: p.DHCardID, Notes: string(audit)}); err != nil {
			return err
		}
		p.DHInventoryID = target
		if err := s.watermark(owned, p, "mutation_settled_at"); err != nil {
			return err
		}
		return s.AssertMutationAllowed(owned, id)
	})
}
