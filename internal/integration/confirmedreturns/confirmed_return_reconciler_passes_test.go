//go:build integration

package confirmedreturns_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func runActualSoldPasses(t *testing.T, repo *postgres.PurchaseStore, sales *postgres.SaleStore, store *postgres.ConfirmedReturnStore, recorder inventory.DHSaleRecorder) (int, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	recoveries, fetches := 0, 0
	recovery := &mocks.InMemoryCampaignStore{ListSalesNeedingDHRecordFn: func(c context.Context, n int) ([]inventory.SaleNeedingDHRecord, error) {
		recoveries++
		return sales.ListSalesNeedingDHRecord(c, n)
	}}
	client := &mocks.MockDHInventoryListClient{}
	s := scheduler.NewDHSoldReconcilerScheduler(repo, repo, mocks.NewMockLogger(), scheduler.DHSoldReconcilerConfig{Enabled: true, Interval: time.Hour}, scheduler.WithDHSoldSweep(client, repo, sales, recorder, sales, repo), scheduler.WithDHSaleHandleRecovery(recovery, recorder, sales, repo), scheduler.WithDHSoldMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, repo, sales))
	client.ListInventoryFn = func(_ context.Context, f dh.InventoryFilters) (*dh.InventoryListResponse, error) {
		fetches++
		response := &dh.InventoryListResponse{}
		if f.Status == "listed" {
			response.Items = []dh.InventoryListItem{{DHInventoryID: 42, CertNumber: "passes-cert", Status: "listed"}}
			response.Meta.TotalCount = 1
		} else {
			s.Stop()
		}
		return response, nil
	}
	s.Start(ctx)
	require.NoError(t, ctx.Err())
	return recoveries, fetches
}
func TestBothSoldPassesAfterReturnCompletionRollback(t *testing.T) {
	db, store, returns, fake, id := setupIntegrationReturn(t, "passes-cert")
	ctx := context.Background()
	logger := mocks.NewMockLogger()
	_, e := db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,order_id,created_at,updated_at) VALUES('legacy-sale','return-p','ebay',12000,'2026-01-04','',now(),now()); UPDATE campaign_purchases SET dh_status='sold' WHERE id='return-p'; CREATE FUNCTION fail_return_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'local delete fails'; END $$; CREATE TRIGGER fail_return_delete BEFORE DELETE ON campaign_sales FOR EACH ROW EXECUTE FUNCTION fail_return_delete()`)
	require.NoError(t, e)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_return_delete ON campaign_sales; DROP FUNCTION IF EXISTS fail_return_delete()`)
	})
	keys := []string{}
	fake.ReturnInventoryToStockFn = func(_ context.Context, _ int, stringKey string) (*inventory.DHReturnResult, error) {
		keys = append(keys, stringKey)
		return &inventory.DHReturnResult{DHInventoryID: 42, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
	}
	expected := "legacy-sale"
	request := inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &expected}
	state, e := returns.ConfirmReturn(ctx, id, request)
	require.Error(t, e)
	require.NotNil(t, state.Operation.ObservedReceipt)
	recorder := &mocks.DHSaleRecorderMock{}
	repo := postgres.NewPurchaseStore(db.DB, logger)
	sales := postgres.NewSaleStore(db.DB, logger)
	recoveries, fetches := runActualSoldPasses(t, repo, sales, store, recorder)
	require.Equal(t, 1, recoveries)
	require.Equal(t, 2, fetches)
	require.Empty(t, recorder.RecordedSales())
	sale, e := sales.GetSaleByPurchaseID(ctx, id)
	require.NoError(t, e)
	require.Empty(t, sale.DHIdempotencyKey, "pending return cannot mint a new sale key")
	_, e = db.ExecContext(ctx, `DROP TRIGGER fail_return_delete ON campaign_sales; DROP FUNCTION fail_return_delete()`)
	require.NoError(t, e)
	request.OperationID = state.Operation.ID
	state, e = returns.ConfirmReturn(ctx, id, request)
	require.NoError(t, e)
	require.True(t, state.AwaitingListing)
	require.Equal(t, []string{keys[0], keys[0]}, keys)
	runActualSoldPasses(t, repo, sales, store, recorder)
	require.Empty(t, recorder.RecordedSales())
}
func TestBothSoldPassesLegacyMintCommitsBeforeRemote(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "passes-cert")
	ctx := context.Background()
	logger := mocks.NewMockLogger()
	_, e := db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,created_at,updated_at) VALUES('legacy-sale','return-p','local',12000,'2026-01-04',now(),now()); UPDATE campaign_purchases SET dh_status='sold' WHERE id='return-p'; CREATE FUNCTION fail_pass_handle() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.dh_sale_id <> '' THEN RAISE EXCEPTION 'handle write fails'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_pass_handle BEFORE UPDATE ON campaign_sales FOR EACH ROW EXECUTE FUNCTION fail_pass_handle()`)
	require.NoError(t, e)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_pass_handle ON campaign_sales; DROP FUNCTION IF EXISTS fail_pass_handle()`)
	})
	recorder := &mocks.DHSaleRecorderMock{RecordInventorySaleFn: func(_ context.Context, r inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
		var key string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT dh_idempotency_key FROM campaign_sales WHERE id='legacy-sale'`).Scan(&key))
		require.Equal(t, key, r.IdempotencyKey)
		require.NotEmpty(t, key)
		target := 42
		return &inventory.DHSaleResult{DHSaleID: "handle", SoldInventoryID: &target, Delisted: true, Replayed: true}, nil
	}}
	repo := postgres.NewPurchaseStore(db.DB, logger)
	sales := postgres.NewSaleStore(db.DB, logger)
	recovery, fetches := runActualSoldPasses(t, repo, sales, store, recorder)
	require.Equal(t, 1, recovery)
	require.Equal(t, 2, fetches)
	calls := recorder.RecordedSales()
	require.Len(t, calls, 2)
	require.Equal(t, calls[0], calls[1])
	state, e := store.GetReturnState(ctx, id)
	require.NoError(t, e)
	require.Equal(t, calls[0].IdempotencyKey, state.Sale.DHIdempotencyKey)
	require.NotNil(t, state.PrecedingAttempt)
	_, e = db.ExecContext(ctx, `DROP TRIGGER fail_pass_handle ON campaign_sales; DROP FUNCTION fail_pass_handle()`)
	require.NoError(t, e)
	require.NoError(t, inventory.RecordCoordinatedDHSale(ctx, inventory.NewDHMutationCoordinator(store, store), repo, sales, recorder, id, "legacy-sale", uuid.NewString))
	calls = recorder.RecordedSales()
	require.Len(t, calls, 3)
	require.Equal(t, calls[0], calls[2])
	state, e = store.GetReturnState(ctx, id)
	require.NoError(t, e)
	require.Nil(t, state.PrecedingAttempt)
	require.Equal(t, "handle", state.Sale.DHSaleID)
}
