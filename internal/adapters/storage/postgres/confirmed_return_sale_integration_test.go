package postgres

import (
	"context"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoordinatedSalePreparationDurable(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "sale-cert", 42, "ext-848")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `UPDATE campaign_sales SET order_id='',dh_idempotency_key='' WHERE id='old-sale'`)
	require.NoError(t, err)
	p := NewPurchaseStore(db.DB, mocks.NewMockLogger())
	sales := NewSaleStore(db.DB, mocks.NewMockLogger())
	coord := inventory.NewDHMutationCoordinator(store, store)
	keys := []string{}
	fail := true
	remote := &mocks.DHSaleRecorderMock{RecordInventorySaleFn: func(c context.Context, r inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
		keys = append(keys, r.IdempotencyKey)
		var key string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT dh_idempotency_key FROM campaign_sales WHERE id='old-sale'`).Scan(&key))
		require.Equal(t, key, r.IdempotencyKey)
		if fail {
			_, e := db.ExecContext(ctx, `CREATE FUNCTION fail_sale_handle() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.dh_sale_id <> '' THEN RAISE EXCEPTION 'handle persist fails'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_sale_handle BEFORE UPDATE ON campaign_sales FOR EACH ROW EXECUTE FUNCTION fail_sale_handle()`)
			require.NoError(t, e)
		}
		target := 42
		return &inventory.DHSaleResult{DHSaleID: "sale-remote", SoldInventoryID: &target, Delisted: true}, nil
	}}
	err = inventory.RecordCoordinatedDHSale(ctx, coord, p, sales, remote, id, "old-sale", func() string { return "stable-mint" })
	require.Error(t, err)
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.Equal(t, keys[0], state.Sale.DHIdempotencyKey)
	_, err = db.ExecContext(ctx, `DROP TRIGGER fail_sale_handle ON campaign_sales; DROP FUNCTION fail_sale_handle()`)
	require.NoError(t, err)
	fail = false
	require.NoError(t, inventory.RecordCoordinatedDHSale(ctx, coord, p, sales, remote, id, "old-sale", func() string { return "must-not-remint" }))
	require.Equal(t, []string{keys[0], keys[0]}, keys)
}
func TestCoordinatedSaleReturnAndImportedFence(t *testing.T) {
	for _, unresolved := range []bool{false, true} {
		t.Run(fmt.Sprint(unresolved), func(t *testing.T) {
			db, store, returns, fake, id := setupReturnCore(t, "fenced-sale", 42, "ext-848")
			ctx := context.Background()
			calls := 0
			remote := &mocks.DHSaleRecorderMock{RecordInventorySaleFn: func(context.Context, inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
				calls++
				return nil, nil
			}}
			if unresolved {
				fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
					return nil, context.DeadlineExceeded
				}
				sale := "old-sale"
				_, err := returns.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, ExpectedTarget: observedReturnTarget("fenced-sale", 42)})
				require.Error(t, err)
			}
			err := inventory.RecordCoordinatedDHSale(ctx, inventory.NewDHMutationCoordinator(store, store), NewPurchaseStore(db.DB, mocks.NewMockLogger()), NewSaleStore(db.DB, mocks.NewMockLogger()), remote, id, "old-sale", func() string { return "mint" })
			if !unresolved {
				require.NoError(t, err)
			}
			require.Zero(t, calls)
		})
	}
}
