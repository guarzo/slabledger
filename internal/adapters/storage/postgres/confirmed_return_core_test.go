package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func setupReturnCore(t *testing.T, cert string, target int, order string) (*DB, *ConfirmedReturnStore, *inventory.ConfirmedReturnService, *mocks.DHReturnerMock, string) {
	t.Helper()
	db := setupTestDB(t)
	ctx := context.Background()
	// Retained history is intentionally outside campaign cascades.
	_, err := db.ExecContext(ctx, `TRUNCATE confirmed_dh_returns,dh_mutation_attempts,dh_target_watermarks; INSERT INTO campaigns(id,name,phase,created_at,updated_at) VALUES('return-c','Returns','pending',now(),now())`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,dh_inventory_id,dh_status,dh_push_status,dh_push_attempts,dh_listing_price_cents,dh_channels_json,reviewed_price_cents,received_at,purchase_date,created_at,updated_at) VALUES('return-p','return-c','Card',$1,'PSA',$2,'in_stock','pending',4,999,'["ebay"]',25000,now(),'2026-01-01',now(),now())`, cert, target)
	require.NoError(t, err)
	if order != "" {
		_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,order_id,created_at,updated_at) VALUES('old-sale','return-p','ebay',10000,'2026-01-03',$1,now(),now())`, order)
		require.NoError(t, err)
	}
	store := NewConfirmedReturnStore(db.DB)
	fake := &mocks.DHReturnerMock{GetReturnInventoryStatusFn: func(context.Context, int, string) (string, error) { return "sold", nil }}
	n := 0
	svc := inventory.NewConfirmedReturnService(store, store, fake, nil, func() string { n++; return fmt.Sprintf("return-generated-%d", n) })
	return db, store, svc, fake, "return-p"
}

func TestConfirmedReturnCoreLifecycle(t *testing.T) {
	tests := []struct {
		name, cert, order string
		target            int
		external          int64
	}{
		{"Charizard already unsold", "160944741", "", 147840, 443},
		{"Spheal already unsold", "162787413", "", 364577, 848},
		{"captured imported sale", "ext-local", "ext-848", 364577, 848},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc, fake, id := setupReturnCore(t, tt.cert, tt.target, tt.order)
			ctx := context.Background()
			calls := 0
			key := ""
			req := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
			if tt.order != "" {
				sale := "old-sale"
				req.ExpectedSaleID = &sale
			}
			fake.ReturnInventoryToStockFn = func(c context.Context, target int, k string) (*inventory.DHReturnResult, error) {
				calls++
				require.Equal(t, tt.target, target)
				var persisted string
				require.NoError(t, db.QueryRowContext(ctx, `SELECT idempotency_key FROM confirmed_dh_returns WHERE captured_purchase_id=$1`, id).Scan(&persisted))
				require.Equal(t, k, persisted)
				if key == "" {
					key = k
				} else {
					require.Equal(t, key, k)
				}
				if calls == 1 && tt.external == 443 {
					return nil, &inventory.DHReturnError{Code: "sale_attribution_missing", Message: "missing link", Status: 409}
				}
				return &inventory.DHReturnResult{DHInventoryID: target, ItemStatus: "in_stock", ExternalSaleID: tt.external, Restored: calls == 1}, nil
			}
			state, err := svc.ConfirmReturn(ctx, id, req)
			if tt.external == 443 {
				require.Error(t, err)
				require.Equal(t, "sale_attribution_missing", state.Operation.LastError.Code)
				req.OperationID = state.Operation.ID
				state, err = svc.ConfirmReturn(ctx, id, req)
			}
			require.NoError(t, err)
			require.Equal(t, "completed", state.Operation.State)
			require.Equal(t, fmt.Sprintf("ext-%d", tt.external), state.Operation.ReturnedOrderID)
			require.True(t, state.AwaitingListing)
			require.Nil(t, state.Sale)
			p, err := NewPurchaseStore(db.DB, mocks.NewMockLogger()).GetPurchase(ctx, id)
			require.NoError(t, err)
			require.Equal(t, tt.target, p.DHInventoryID)
			require.Equal(t, "in_stock", p.DHStatus)
			require.Equal(t, "matched", p.DHPushStatus)
			require.Zero(t, p.DHPushAttempts)
			require.Zero(t, p.DHListingPriceCents)
			require.Equal(t, "[]", p.DHChannelsJSON)
			require.Equal(t, 25000, p.ReviewedPriceCents)
			require.Empty(t, p.DHUnlistedDetectedAt)
			require.ErrorIs(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }), inventory.ErrReturnConflict)
			before := calls
			require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AuthorizeReturnedListing(c, id, state.Operation.ID) }))
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
			require.NoError(t, err)
			replay, err := svc.ConfirmReturn(ctx, id, req)
			require.NoError(t, err)
			require.Equal(t, before, calls)
			require.Equal(t, "listed", replay.Purchase.DHStatus)
			require.False(t, replay.AwaitingListing)
			require.NoError(t, NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(ctx, id))
			var live *string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT purchase_id FROM confirmed_dh_returns WHERE id=$1`, state.Operation.ID).Scan(&live))
			require.Nil(t, live)
			_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,dh_inventory_id,received_at,purchase_date,created_at,updated_at) VALUES('recreated','return-c','Card',$1,'PSA',$2,now(),'2026-01-01',now(),now())`, tt.cert, tt.target)
			require.NoError(t, err)
			sale := &inventory.Sale{ID: "returned-sale", PurchaseID: "recreated", OrderID: fmt.Sprintf("ext-%d", tt.external), SaleChannel: inventory.SaleChannelEbay, SaleDate: "2026-01-03", CreatedAt: time.Now(), UpdatedAt: time.Now()}
			require.ErrorIs(t, NewSaleStore(db.DB, mocks.NewMockLogger()).CreateSale(ctx, sale), inventory.ErrReturnConflict)
			sale.ID = "new-sale"
			sale.OrderID = "ext-99999"
			require.NoError(t, NewSaleStore(db.DB, mocks.NewMockLogger()).CreateSale(ctx, sale))
		})
	}
}

func TestConfirmedReturnConflictsAndUncertainty(t *testing.T) {
	tests := []struct {
		name, status string
		target       int
		external     int64
		failure      error
	}{
		{"listed replay", "listed", 364577, 848, nil}, {"sold replay", "sold", 364577, 848, nil}, {"unknown replay", "unknown", 364577, 848, nil}, {"wrong target", "in_stock", 3, 848, nil}, {"wrong order", "in_stock", 364577, 9, nil}, {"timeout", "", 0, 0, context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, store, svc, fake, id := setupReturnCore(t, "conflict-cert", 364577, "ext-848")
			sale := "old-sale"
			req := inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale}
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				return &inventory.DHReturnResult{DHInventoryID: tt.target, ItemStatus: tt.status, ExternalSaleID: tt.external}, tt.failure
			}
			state, err := svc.ConfirmReturn(context.Background(), id, req)
			require.Error(t, err)
			require.NotNil(t, state.Operation)
			require.NotEqual(t, "completed", state.Operation.State)
			require.ErrorIs(t, NewSaleStore(db.DB, mocks.NewMockLogger()).DeleteSaleByPurchaseID(context.Background(), id), inventory.ErrReturnConflict)
			require.ErrorIs(t, NewPurchaseStore(db.DB, mocks.NewMockLogger()).DeletePurchase(context.Background(), id), inventory.ErrReturnConflict)
			require.ErrorIs(t, NewCampaignStore(db.DB, mocks.NewMockLogger()).DeleteCampaign(context.Background(), "return-c"), inventory.ErrReturnConflict)
			p, e := NewPurchaseStore(db.DB, mocks.NewMockLogger()).GetPurchase(context.Background(), id)
			require.NoError(t, e)
			require.Equal(t, 364577, p.DHInventoryID)
			require.Error(t, store.WithPurchaseMutation(context.Background(), id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }))
		})
	}
}

func TestConfirmedReturnCompletionRollbackSameKey(t *testing.T) {
	db, store, svc, fake, id := setupReturnCore(t, "rollback-cert", 364577, "ext-848")
	ctx := context.Background()
	key := ""
	// A real DB-side completion failure, not an interface mock swallowing writes.
	_, err := db.ExecContext(ctx, `CREATE FUNCTION fail_return_completion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='completed' THEN RAISE EXCEPTION 'completion unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_completion BEFORE UPDATE ON confirmed_dh_returns FOR EACH ROW EXECUTE FUNCTION fail_return_completion()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_completion ON confirmed_dh_returns; DROP FUNCTION IF EXISTS fail_return_completion()`)
	})
	fake.ReturnInventoryToStockFn = func(_ context.Context, _ int, stringKey string) (*inventory.DHReturnResult, error) {
		if key == "" {
			key = stringKey
		} else {
			require.Equal(t, key, stringKey)
		}
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	sale := "old-sale"
	req := inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale}
	state, err := svc.ConfirmReturn(ctx, id, req)
	require.Error(t, err)
	require.Equal(t, "pending", state.Operation.State)
	require.NotNil(t, state.Operation.ObservedReceipt)
	require.Equal(t, "completion", state.Operation.LastError.Phase)
	require.NotNil(t, state.PrecedingAttempt)
	_, err = db.ExecContext(ctx, `DROP TRIGGER fail_completion ON confirmed_dh_returns; DROP FUNCTION fail_return_completion()`)
	require.NoError(t, err)
	req.OperationID = state.Operation.ID
	state, err = svc.ConfirmReturn(ctx, id, req)
	require.NoError(t, err)
	require.Equal(t, "completed", state.Operation.State)
	require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AuthorizeReturnedListing(c, id, state.Operation.ID) }))
	// Lost response and duplicate requests are historical no-ops even after a new sale.
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,order_id,created_at,updated_at) VALUES('new-current', $1,'ebay','2026-01-04','ext-999',now(),now())`, id)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='sold' WHERE id=$1`, id)
	require.NoError(t, err)
	state, err = svc.ConfirmReturn(ctx, id, req)
	require.NoError(t, err)
	require.Equal(t, "new-current", state.Sale.ID)
	require.Equal(t, "sold", state.Purchase.DHStatus)
	bad := "not-current"
	_, err = svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &bad})
	require.ErrorIs(t, err, inventory.ErrReturnConflict)
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 999}, nil
	}
	next := "new-current"
	state, err = svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &next})
	require.NoError(t, err)
	require.Equal(t, "ext-999", state.Operation.ReturnedOrderID)
	require.False(t, errors.Is(err, inventory.ErrReturnConflict))
}
