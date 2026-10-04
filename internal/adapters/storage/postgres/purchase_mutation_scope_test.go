package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestPurchaseMutationScope(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase,created_at,updated_at) VALUES('scope','Scope','pending',now(),now()); INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,purchase_date,created_at,updated_at) VALUES('scope-p','scope','Card','scope-cert','PSA','2026-01-01',now(),now())`)
	require.NoError(t, err)
	// RunMigrations retains its dedicated sql.Conn; exercise runtime ownership on
	// a fresh pool that has no migration-owned connection, exactly one backend.
	db = requireTestDB(t)
	scope := NewPurchaseMutationScope(db.DB)
	ps := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	tests := []struct {
		name     string
		rollback bool
	}{{"commit", false}, {"rollback", true}}
	db.SetMaxOpenConns(1)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sentinel := errors.New("rollback")
			status := "in_stock"
			if tt.rollback {
				status = "listed"
			}
			err := scope.WithPurchaseMutation(ctx, "scope-p", func(owned context.Context) error {
				require.NoError(t, ps.UpdatePurchaseDHStatus(owned, "scope-p", status))
				require.NoError(t, scope.WithPurchaseMutation(owned, "scope-p", func(nested context.Context) error {
					p, e := ps.GetPurchase(nested, "scope-p")
					require.NoError(t, e)
					require.Equal(t, status, p.DHStatus)
					require.Same(t, executor(owned, db.DB), executor(nested, db.DB))
					return nil
				}))
				if tt.rollback {
					return sentinel
				}
				return nil
			})
			if tt.rollback {
				require.ErrorIs(t, err, sentinel)
			} else {
				require.NoError(t, err)
			}
			readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
			defer readCancel()
			p, e := ps.GetPurchase(readCtx, "scope-p")
			require.NoError(t, e)
			require.Equal(t, "in_stock", p.DHStatus)
		})
	}
	db.SetMaxOpenConns(25)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- scope.WithPurchaseMutation(ctx, "scope-p", func(owned context.Context) error {
			close(entered)
			<-release
			return ps.UpdatePurchaseDHStatus(owned, "scope-p", "listed")
		})
	}()
	<-entered
	waitCtx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	require.Error(t, scope.WithPurchaseMutation(waitCtx, "scope-p", func(context.Context) error { t.Fatal("contending callback entered"); return nil }))
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, scope.WithPurchaseMutation(ctx, "scope-p", func(owned context.Context) error {
		p, e := ps.GetPurchase(owned, "scope-p")
		require.NoError(t, e)
		require.Equal(t, "listed", p.DHStatus)
		d, ok := owned.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(d), 90*time.Second)
		return nil
	}))
}
