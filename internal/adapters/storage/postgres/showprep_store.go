package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/jackc/pgx/v5/pgconn"
)

type showPrepQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
type ShowPrepStore struct{ db *sql.DB }
type showPrepSession struct {
	q             showPrepQuery
	observedHolds []showPrepPriceHold
}

var _ sp.Store = (*ShowPrepStore)(nil)
var _ sp.Session = (*showPrepSession)(nil)

func NewShowPrepStore(db *sql.DB) *ShowPrepStore { return &ShowPrepStore{db: db} }

// A short, feature-local advisory lock serializes list mutations with evidence
// publication/hold observation. No source/network work runs while it is held.
// Financial writers use their ordinary row/FK locks, not this advisory lock.
func (s *ShowPrepStore) Within(ctx context.Context, fn func(sp.Session) error) error {
	var mutationErr error
	err := s.transaction(ctx, func(tx *showPrepSession) error {
		if _, err := tx.q.ExecContext(ctx, `SAVEPOINT showprep_mutation`); err != nil {
			return err
		}
		mutationErr = fn(tx)
		if mutationErr == nil {
			return nil
		}
		// Reject all list writes, but retain exactly what locked validation saw.
		// Re-querying collisions here could miss a competitor already deleted.
		if _, err := tx.q.ExecContext(ctx, `ROLLBACK TO SAVEPOINT showprep_mutation`); err != nil {
			return fmt.Errorf("rollback show preparation mutation: %w", err)
		}
		return tx.restorePriceHolds(ctx)
	})
	if err != nil {
		return err // A safety-persistence failure is not an ordinary business conflict.
	}
	return mutationErr
}
func (s *ShowPrepStore) transaction(ctx context.Context, fn func(*showPrepSession) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin show preparation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(731946)`); err != nil {
		return err
	}
	if err := fn(&showPrepSession{q: tx}); err != nil {
		return err
	}
	return tx.Commit()
}
func showPrepError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return sp.ErrNotFound
	}
	return err
}

func (s *ShowPrepStore) ReadPurchases(ctx context.Context, ids []string) (map[string]sp.Purchase, error) {
	return (&showPrepSession{q: s.db}).ReadPurchases(ctx, ids)
}
func (s *showPrepSession) ReadPurchases(ctx context.Context, ids []string) (map[string]sp.Purchase, error) {
	out := map[string]sp.Purchase{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.QueryContext(ctx, `SELECT p.id,p.campaign_id,p.card_name,p.cert_number,p.grader,p.grade_value,p.gem_rate_id,
 c.id IS NOT NULL,COALESCE(c.phase,''),EXISTS(SELECT 1 FROM campaign_sales sale WHERE sale.purchase_id=p.id),
 p.was_refunded,p.received_at IS NOT NULL,p.dh_listing_price_cents,p.dh_last_synced_at,
 p.reviewed_price_cents,p.reviewed_at,p.override_price_cents,p.override_set_at
 FROM campaign_purchases p LEFT JOIN campaigns c ON c.id=p.campaign_id WHERE p.id=ANY($1::text[])`, ids)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var p sp.Purchase
		var local inventory.Purchase
		if err := rows.Scan(&p.ID, &p.CampaignID, &p.CardName, &p.CertNumber, &p.Grader, &p.Grade, &p.ProfileID, &p.CampaignExists, &p.Phase, &p.Sold, &p.Refunded, &p.Received, &p.ListedPriceCents, &p.ListingSyncedAt, &local.ReviewedPriceCents, &local.ReviewedAt, &local.OverridePriceCents, &local.OverrideSetAt); err != nil {
			return nil, err
		}
		p.Known = true
		p.Exists = true
		p.LocalPriceCents = inventory.ResolveListingPriceCents(&local)
		out[p.ID] = p
	}
	return out, rows.Err()
}
func (s *showPrepSession) LockPurchases(ctx context.Context, ids []string) error {
	// Match DeleteCampaign's purchase-before-campaign order. Lock purchases
	// before resolving their current campaigns so reassignment cannot interleave.
	// NOWAIT avoids cycles with bulk DELETE's unordered child-row locks: reject
	// contention and release earlier batch locks rather than deadlock either writer.
	// FOR UPDATE still conflicts with sale FK KEY SHARE and receipt/refund writes.
	for _, query := range []string{
		`SELECT id FROM campaign_purchases WHERE id=ANY($1::text[]) ORDER BY id FOR UPDATE NOWAIT`,
		`SELECT c.id FROM campaigns c WHERE c.id IN (SELECT campaign_id FROM campaign_purchases WHERE id=ANY($1::text[])) ORDER BY c.id FOR UPDATE NOWAIT`,
	} {
		rows, err := s.q.QueryContext(ctx, query, ids)
		if err != nil {
			return showPrepLockError(err)
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
		}
		rowErr := rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if rowErr != nil {
			return showPrepLockError(rowErr)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func showPrepLockError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "55P03" { // lock_not_available
		return sp.ErrConflict
	}
	return err
}
