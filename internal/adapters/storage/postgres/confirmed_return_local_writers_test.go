package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnPendingLocalWriters(t *testing.T) {
	db, _, svc, fake, id := setupReturnCore(t, "writers-cert", 364577, "ext-848")
	ctx := context.Background()
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		return nil, context.DeadlineExceeded
	}
	sale := "old-sale"
	_, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale})
	require.Error(t, err)
	ss := NewSaleStore(db.DB, mocks.NewMockLogger())
	tests := []struct {
		name string
		run  func() error
	}{
		{"legacy key mint", func() error { _, err := ss.SetSaleIdempotencyKeyIfAbsent(ctx, sale, "new-key"); return err }},
		{"sale handle write", func() error { return ss.SetSaleDHSaleID(ctx, sale, "new-handle", time.Now()) }},
		{"link writer", func() error {
			return NewPurchaseStore(db.DB, mocks.NewMockLogger()).UpdatePurchaseDHFields(ctx, id, inventory.DHFieldsUpdate{InventoryID: 7, DHStatus: "in_stock"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { require.ErrorIs(t, tt.run(), inventory.ErrReturnConflict) })
	}
	var key, handle string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT dh_idempotency_key,dh_sale_id FROM campaign_sales WHERE id=$1`, sale).Scan(&key, &handle))
	require.Empty(t, key)
	require.Empty(t, handle)
	// Scoped readers really see uncommitted local transitions and use one backend.
	// Use a fresh runtime pool, separate from the migration helper's retained conn.
	db = requireTestDB(t)
	store := NewConfirmedReturnStore(db.DB)
	db.SetMaxOpenConns(1)
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
		_, err := executor(c, db.DB).ExecContext(c, `UPDATE campaign_purchases SET dh_status='scope-fresh' WHERE id=$1`, id)
		if err != nil {
			return err
		}
		p, err := NewPurchaseStore(db.DB, mocks.NewMockLogger()).GetPurchaseByCertNumber(c, "PSA", "writers-cert")
		require.NoError(t, err)
		require.Equal(t, "scope-fresh", p.DHStatus)
		return nil
	}))
}
