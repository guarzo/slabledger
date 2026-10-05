package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func withLocalPurchaseMutation(ctx context.Context, db *sql.DB, id string, awaiting bool, fn func(context.Context) error) error {
	store := NewConfirmedReturnStore(db)
	return store.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		if err := store.assertAllowed(owned, id, awaiting); err != nil {
			return err
		}
		before, err := store.returnState(owned, id)
		if err != nil {
			return err
		}
		if err := fn(owned); err != nil {
			return err
		}
		scope, _ := ownedScope(owned, db, id)
		if scope.observation {
			return nil
		}
		p, err := (&PurchaseStore{base: base{db: db}}).GetPurchase(owned, id)
		if errors.Is(err, inventory.ErrPurchaseNotFound) {
			p = before.Purchase
		} else if err != nil {
			return err
		}
		return store.watermark(owned, p, "mutation_settled_at")
	})
}
func (s *ConfirmedReturnStore) assertOrderNotReturned(ctx context.Context, p *inventory.Purchase, order string) error {
	if order == "" {
		return nil
	}
	var returned bool
	err := executor(ctx, s.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM confirmed_dh_returns WHERE grader=$1 AND cert_number=$2 AND returned_order_id=$3 AND state='completed')`, p.Grader, p.CertNumber, order).Scan(&returned)
	if err != nil {
		return err
	}
	if returned {
		return inventory.NewReturnConflict("returned_order", "this canonical external order has a retained confirmed-return receipt")
	}
	return nil
}

var _ inventory.DHMutationGuards = (*ConfirmedReturnStore)(nil)

func (s *ConfirmedReturnStore) IsReturnedOrder(ctx context.Context, id, order string) (bool, error) {
	returned := false
	err := s.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		p, err := (&PurchaseStore{base: base{db: s.db}}).GetPurchase(owned, id)
		if err != nil {
			return err
		}
		err = s.assertOrderNotReturned(owned, p, order)
		var conflict *inventory.ReturnConflict
		if errors.As(err, &conflict) && conflict.Code == "returned_order" {
			returned = true
			return nil
		}
		return err
	})
	return returned, err
}

func (ss *SaleStore) guardSaleInsert(ctx context.Context, sale *inventory.Sale) error {
	p, err := (&PurchaseStore{base: base{db: ss.db}}).GetPurchase(ctx, sale.PurchaseID)
	if err != nil {
		return err
	}
	return NewConfirmedReturnStore(ss.db).assertOrderNotReturned(ctx, p, sale.OrderID)
}
func (ss *SaleStore) withSaleMutation(ctx context.Context, saleID string, fn func(context.Context) error) error {
	var purchaseID string
	err := executor(ctx, ss.db).QueryRowContext(ctx, `SELECT purchase_id FROM campaign_sales WHERE id=$1`, saleID).Scan(&purchaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return inventory.ErrSaleNotFound
	}
	if err != nil {
		return err
	}
	return withLocalPurchaseMutation(ctx, ss.db, purchaseID, false, fn)
}
func (ps *PurchaseStore) execDHMutation(ctx context.Context, id, op, query string, args ...any) error {
	return withLocalPurchaseMutation(ctx, ps.db, id, true, func(owned context.Context) error { return ps.execAndExpectRow(owned, op, query, args...) })
}

// DeleteCampaign owns all affected purchases in stable ID order, in ONE
// transaction. Lock the campaign against concurrent FK insertions before the
// membership read; no partially deleted campaign on an unresolved fence.
func (cs *CampaignStore) deleteCampaignGuarded(ctx context.Context, id string) error {
	coordinator := NewPurchaseMutationScope(cs.db)
	run := func(owned context.Context) error {
		x := executor(owned, cs.db)
		var campaign string
		err := x.QueryRowContext(owned, `SELECT id FROM campaigns WHERE id=$1 FOR UPDATE`, id).Scan(&campaign)
		if errors.Is(err, sql.ErrNoRows) {
			return inventory.ErrCampaignNotFound
		}
		if err != nil {
			return err
		}
		rows, err := x.QueryContext(owned, `SELECT id FROM campaign_purchases WHERE campaign_id=$1 ORDER BY id`, id)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, p)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		s := owned.Value(mutationScopeKey{}).(*mutationScope)
		// Never expand an existing single-purchase scope out of lock order.
		if len(s.ids) > 0 {
			for _, p := range ids {
				if !s.ids[p] {
					return inventory.NewReturnConflict("nested_mutation_scope", "campaign delete requires its own multi-purchase scope")
				}
			}
		}
		for _, p := range ids {
			if !s.ids[p] {
				if err := coordinator.lockPurchase(owned, p); err != nil {
					return err
				}
			}
			if err := NewConfirmedReturnStore(cs.db).assertAllowed(owned, p, false); err != nil {
				return err
			}
		}
		if _, err := x.ExecContext(owned, `DELETE FROM campaign_sales WHERE purchase_id IN(SELECT id FROM campaign_purchases WHERE campaign_id=$1)`, id); err != nil {
			return err
		}
		if _, err := x.ExecContext(owned, `DELETE FROM campaign_purchases WHERE campaign_id=$1`, id); err != nil {
			return err
		}
		_, err = x.ExecContext(owned, `DELETE FROM campaigns WHERE id=$1`, id)
		return err
	}
	if s, ok := ctx.Value(mutationScopeKey{}).(*mutationScope); ok {
		if s.db != cs.db {
			return inventory.NewReturnConflict("nested_mutation_scope", "different database ownership")
		}
		return run(ctx)
	}
	return coordinator.withTransaction(ctx, run)
}
