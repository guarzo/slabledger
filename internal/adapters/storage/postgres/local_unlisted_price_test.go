package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestUnlistedPriceSkipPreservesExistingGuards(t *testing.T) {
	for _, tc := range []struct{ name, sql, code string }{
		{"open attempt", `INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('existing','return-p','return-p',42,'local-price-cert','PSA','price','patch','historical')`, "preceding_dh_mutation_uncertain"},
		{"current sale", `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,created_at,updated_at) VALUES('sale','return-p','local','2026-01-01',now(),now())`, "current_sale_present"},
		{"pending return", `INSERT INTO confirmed_dh_returns(id,idempotency_key,captured_purchase_id,dh_inventory_id,cert_number,grader) VALUES('return','key','return-p',42,'local-price-cert','PSA')`, "confirmed_return_unresolved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "local-price-cert", 42, "")
			_, err := db.ExecContext(context.Background(), tc.sql)
			require.NoError(t, err)
			var before string
			require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COALESCE(jsonb_agg(to_jsonb(a))::text,'[]') FROM dh_mutation_attempts a`).Scan(&before))
			fields := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			remote := &mocks.DHInventoryListerMock{UpdateInventoryStatusFn: func(context.Context, int, inventory.DHInventoryStatusUpdate) (int, error) {
				t.Fatal("guarded price dispatched")
				return 0, nil
			}}
			price := dhpricing.NewService(fields, remote, fields, fields, mocks.NewMockLogger(), dhpricing.WithMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store))
			err = price.SyncPurchasePrice(context.Background(), id).Err
			require.ErrorIs(t, err, inventory.ErrReturnConflict)
			var conflict *inventory.ReturnConflict
			require.ErrorAs(t, err, &conflict)
			require.Equal(t, tc.code, conflict.Code)
			var after string
			require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COALESCE(jsonb_agg(to_jsonb(a))::text,'[]') FROM dh_mutation_attempts a`).Scan(&after))
			require.Equal(t, before, after, "skip must not settle or replace historical attempts")
		})
	}
}

func TestUnlistedPricesRemainLocalUntilExplicitListing(t *testing.T) {
	for _, tc := range []struct {
		name        string
		inventoryID int
		want        dhpricing.Outcome
	}{
		{"pushed stock", 42, dhpricing.OutcomeSkippedUnlisted}, {"not pushed", 0, dhpricing.OutcomeSkippedNoInventory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "local-price-cert", tc.inventoryID, "")
			ctx := context.Background()
			fields := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			remote := &mocks.DHInventoryListerMock{UpdateInventoryStatusFn: func(context.Context, int, inventory.DHInventoryStatusUpdate) (int, error) {
				t.Fatal("unlisted price edit dispatched")
				return 0, nil
			}}
			price := dhpricing.NewService(fields, remote, fields, fields, mocks.NewMockLogger(), dhpricing.WithMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store))
			for _, cents := range []int{1800, 2500} {
				_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=$1 WHERE id=$2`, cents, id)
				require.NoError(t, err)
				result := price.SyncPurchasePrice(ctx, id)
				require.NoError(t, result.Err)
				require.Equal(t, tc.want, result.Outcome)
				var attempts int
				require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM dh_mutation_attempts`).Scan(&attempts))
				require.Zero(t, attempts, "local price save must not prepare a DH attempt")
				drift, err := fields.ListDHPriceDrift(ctx)
				require.NoError(t, err)
				require.Empty(t, drift, "unlisted inventory must not enter scheduled repricing")
			}
			if tc.inventoryID == 0 {
				return
			}
			remote.UpdateInventoryStatusFn = func(_ context.Context, target int, update inventory.DHInventoryStatusUpdate) (int, error) {
				require.Equal(t, 42, target)
				require.Equal(t, "listed", update.Status)
				require.Equal(t, 2500, update.ListingPriceCents)
				return 2500, nil
			}
			listing, err := dhlisting.NewDHListingService(fields, mocks.NewMockLogger(), dhlisting.WithDHListingLister(remote), dhlisting.WithDHListingFieldsUpdater(fields), dhlisting.WithDHListingMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store), dhlisting.WithDHListingConfigLoader(NewDHStore(db.DB, mocks.NewMockLogger())))
			require.NoError(t, err)
			listed := listing.(dhlisting.ExplicitListingService).ListPurchaseExplicit(ctx, id, "local-price-cert", "")
			require.Equal(t, 1, listed.Listed, "listing must not depend on prior price PATCH: %v", listed.FailedCerts)
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Nil(t, state.Sale)
			require.Nil(t, state.PrecedingAttempt)
			require.Equal(t, 2500, state.Purchase.DHListingPriceCents)
			require.Equal(t, 2500, state.Purchase.ReviewedPriceCents)
		})
	}
}
