package postgres

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// nonFinancialAttemptKinds are DH attempts that can never create, settle or
// reverse a sale. Anything not listed here (sale, void, return, and any unknown
// kind) stays blocking, so a new kind fails closed.
const nonFinancialAttemptKinds = `('push','list','price','link')`

// localKindClause is the per-attempt kind test for LOCAL sale/delete. An open
// sale/record attempt only matters while its local sale row still exists (that
// is the double-record risk); once the sale was un-sold it is remote-only
// uncertainty like any other. void and return attempts always block.
func localKindClause(sale *inventory.Sale) string {
	c := `(kind IN ` + nonFinancialAttemptKinds
	if sale == nil {
		c += ` OR (kind='sale' AND phase='record')`
	}
	return c + `)`
}

// openAttemptRows returns every open attempt that touches this purchase's
// identity, with whether each is a non-financial kind.
func (s *ConfirmedReturnStore) openAttemptRows(ctx context.Context, p *inventory.Purchase, extra string) ([]openAttempt, error) {
	rows, err := executor(ctx, s.db).QueryContext(ctx, `SELECT id, (`+extra+`) FROM dh_mutation_attempts WHERE outcome='open' AND
 (captured_purchase_id=$1 OR dh_inventory_id=NULLIF($2,0) OR (cert_number=$3 AND grader=$4 AND cert_number<>''))`, p.ID, p.DHInventoryID, p.CertNumber, p.Grader)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []openAttempt
	for rows.Next() {
		var a openAttempt
		if err := rows.Scan(&a.id, &a.ok); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type openAttempt struct {
	id string
	ok bool
}

// A failed, crashed or still-running push/list/price/link attempt is an
// unconfirmed DH write; none is proof that DH made no changes. A completed
// return episode for a different target is retained history for that old
// target. This warned path is only for acknowledged local Delete, never generic
// mutation. It returns the attempts to abandon and whether a warning is needed.
func (s *ConfirmedReturnStore) failedListingDeleteAttempts(ctx context.Context, state *inventory.ConfirmedReturnState) ([]string, bool, error) {
	p := state.Purchase
	if state.Sale != nil || p.DHSaleConflict != "" || p.DHSaleConflictAt != nil || p.DHStatus == inventory.DHStatusSold {
		return nil, false, nil
	}
	mismatched := state.Operation != nil && state.Operation.State == "completed" && !episodeTargetMatches(state.Operation, p)
	open, err := s.openAttemptRows(ctx, p, `captured_purchase_id=$1 AND COALESCE(dh_inventory_id,0)=$2 AND cert_number=$3 AND grader=$4
 AND `+localKindClause(nil)+` AND return_operation_id IS NULL AND receipt='' AND payload_identity<>''`)
	if err != nil {
		return nil, false, err
	}
	var ids []string
	for _, a := range open {
		if !a.ok {
			return nil, false, nil
		}
		ids = append(ids, a.id)
	}
	if state.Operation != nil && !mismatched {
		return nil, false, nil
	}
	return ids, len(ids) > 0 || mismatched, nil
}

func (ps *PurchaseStore) withFailedListingDelete(ctx context.Context, id string, remove func(context.Context) error) error {
	s := NewConfirmedReturnStore(ps.db)
	return s.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		state, err := s.returnState(owned, id)
		if err != nil {
			return err
		}
		ids, warn, err := s.failedListingDeleteAttempts(owned, state)
		if err != nil {
			return err
		}
		if warn {
			if !inventory.FailedListingDeleteConfirmed(ctx) {
				return inventory.NewReturnConflict("failed_listing_delete_confirmation_required", "DH state for this card is uncertain. Confirm Delete with the warning; DH may still hold the item (listed or sold).")
			}
			scope, err := ownedScope(owned, ps.db, id)
			if err != nil {
				return err
			}
			scope.warnedDelete = true
			defer func() { scope.warnedDelete = false }()
		}
		if len(ids) > 0 {
			result, err := executor(owned, ps.db).ExecContext(owned, `UPDATE dh_mutation_attempts SET outcome='abandoned',settled_at=clock_timestamp(),listing_retryable=false,receipt='abandoned on delete; original provider outcome unknown' WHERE id=ANY($1) AND outcome='open'`, ids)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != int64(len(ids)) {
				return inventory.NewReturnConflict("attempt_settlement_conflict", "DH attempts changed before Delete")
			}
		}
		// Re-run the ordinary guard and delete in the SAME transaction. Any failure
		// rolls back abandonment too; other matching attempts are never bypassed.
		return remove(owned)
	})
}
