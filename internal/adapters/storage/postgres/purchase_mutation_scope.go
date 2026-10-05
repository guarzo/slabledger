package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type mutationExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type mutationScopeKey struct{}
type mutationScope struct {
	db          *sql.DB
	tx          *sql.Tx
	ids         map[string]bool
	attemptID   string
	returnID    string
	observation bool
}

func executor(ctx context.Context, db *sql.DB) mutationExecutor {
	if s, ok := ctx.Value(mutationScopeKey{}).(*mutationScope); ok && s.db == db {
		return s.tx
	}
	return db
}
func ownedScope(ctx context.Context, db *sql.DB, id string) (*mutationScope, error) {
	s, ok := ctx.Value(mutationScopeKey{}).(*mutationScope)
	if !ok || s.db != db || !s.ids[id] {
		return nil, inventory.NewReturnConflict("mutation_scope_required", "purchase mutation ownership required")
	}
	return s, nil
}

type PurchaseMutationCoordinator struct{ db *sql.DB }

func NewPurchaseMutationScope(db *sql.DB) *PurchaseMutationCoordinator {
	return &PurchaseMutationCoordinator{db: db}
}

var _ inventory.PurchaseMutationScope = (*PurchaseMutationCoordinator)(nil)

func (c *PurchaseMutationCoordinator) WithPurchaseMutation(ctx context.Context, id string, fn func(context.Context) error) error {
	if id == "" || fn == nil {
		return errors.New("purchase scope requires id and callback")
	}
	if s, ok := ctx.Value(mutationScopeKey{}).(*mutationScope); ok {
		if s.db != c.db || !s.ids[id] {
			return inventory.NewReturnConflict("nested_mutation_scope", "cannot acquire another purchase inside an owning scope")
		}
		return fn(ctx)
	}
	return c.withTransaction(ctx, func(owned context.Context) error {
		if err := c.lockPurchase(owned, id); err != nil {
			return err
		}
		return fn(owned)
	})
}
func (c *PurchaseMutationCoordinator) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(bounded, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin purchase mutation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	s := &mutationScope{db: c.db, tx: tx, ids: map[string]bool{}}
	owned := context.WithValue(bounded, mutationScopeKey{}, s)
	if err := fn(owned); err != nil {
		return err
	}
	if err := bounded.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit purchase mutation: %w", err)
	}
	return nil
}
func (c *PurchaseMutationCoordinator) lockPurchase(ctx context.Context, id string) error {
	s := ctx.Value(mutationScopeKey{}).(*mutationScope)
	// Transaction locks survive pooler backend switches only until transaction end.
	if _, err := s.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7148))`, "purchase:"+id); err != nil {
		return fmt.Errorf("lock purchase mutation: %w", err)
	}
	var cert, grader string
	err := s.tx.QueryRowContext(ctx, `SELECT cert_number,grader FROM campaign_purchases WHERE id=$1`, id).Scan(&cert, &grader)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && cert != "" {
		if _, err := s.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7149))`, grader+":"+cert); err != nil {
			return err
		}
	}
	s.ids[id] = true
	return nil
}
