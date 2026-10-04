//go:build integration

package confirmedreturns_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestCoordinatedPriceUsesValidatedSnapshot(t *testing.T) {
	for _, kind := range []string{"reviewed", "override"} {
		t.Run(kind, func(t *testing.T) {
			_, _, _, _, id := setupIntegrationReturn(t, "price-snapshot-cert")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			logger := mocks.NewMockLogger()
			// The consumer owns one backend; the independent ordinary writer has
			// its own connection and does not participate in purchase ownership.
			db := openConfirmedReturnsTestDB(t, ctx)
			db.SetMaxOpenConns(1)
			db.SetMaxIdleConns(1)
			store := postgres.NewConfirmedReturnStore(db.DB)
			writerDB := openConfirmedReturnsTestDB(t, ctx)
			repo := postgres.NewPurchaseStore(db.DB, logger)
			validated, committed := make(chan struct{}), make(chan error, 1)
			guards := &mocks.DHMutationGuardsMock{}
			checks := 0
			guards.AssertMutationAllowedFn = func(c context.Context, p string) error {
				if e := store.AssertMutationAllowed(c, p); e != nil {
					return e
				}
				checks++
				if checks == 2 {
					close(validated)
					select {
					case e := <-committed:
						return e
					case <-c.Done():
						return c.Err()
					}
				}
				return nil
			}
			go func() {
				select {
				case <-validated:
				case <-ctx.Done():
					committed <- ctx.Err()
					return
				}
				var err error
				if kind == "reviewed" {
					err = postgres.NewPricingStore(writerDB.DB, logger).UpdateReviewedPrice(ctx, id, 30000, "manual")
				} else {
					err = postgres.NewPurchaseStore(writerDB.DB, logger).UpdatePurchasePriceOverride(ctx, id, 30000, "manual")
				}
				committed <- err
			}()
			var sentPrice, patches atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "PATCH", r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory/42", r.URL.Path)
				patches.Add(1)
				var request dh.InventoryUpdate
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.NotNil(t, request.ListingPriceCents)
				sentPrice.Store(int32(*request.ListingPriceCents))
				var payload, outcome string
				require.NoError(t, writerDB.QueryRowContext(ctx, `SELECT payload_identity,outcome FROM dh_mutation_attempts WHERE kind='price'`).Scan(&payload, &outcome))
				require.JSONEq(t, `{"ID":42,"Status":"in_stock","Price":25000}`, payload)
				require.Equal(t, "open", outcome, "marker must already be committed")
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"dh_inventory_id":42,"status":"in_stock","listing_price_cents":%d}`, *request.ListingPriceCents)
			}))
			defer provider.Close()
			remote := adapter.NewInventoryAdapter(dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))).WithMutationReceipts()
			price := dhpricing.NewService(repo, remote, repo, repo, logger, dhpricing.WithMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), guards, store))
			result := price.SyncPurchasePrice(ctx, id)
			require.NoError(t, result.Err)
			require.Equal(t, dhpricing.OutcomeSynced, result.Outcome)
			require.Equal(t, int32(25000), sentPrice.Load(), "wire must match the validated immutable journal, not a later ordinary commit")
			require.Equal(t, int32(1), patches.Load())
			var outcome string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT outcome FROM dh_mutation_attempts WHERE kind='price'`).Scan(&outcome))
			require.Equal(t, "succeeded", outcome)
			p, e := repo.GetPurchase(ctx, id)
			require.NoError(t, e)
			require.Equal(t, 25000, p.DHListingPriceCents)
			require.Equal(t, 30000, inventory.ResolveListingPriceCents(p), "new ordinary price remains committed for a later sync")
		})
	}
}

// The sale is committed by actual CreateSale before its best-effort status
// write, while the persisted DH status still looks listable.
func TestCoordinatedPriceCreateSaleInterleaving(t *testing.T) {
	for _, status := range []string{"listed", "in_stock"} {
		t.Run(status, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "price-sale-cert")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			logger := mocks.NewMockLogger()
			_, e := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status=$1 WHERE id=$2`, status, id)
			require.NoError(t, e)
			repo := postgres.NewPurchaseStore(db.DB, logger)
			sales := postgres.NewSaleStore(db.DB, logger)
			campaigns := postgres.NewCampaignStore(db.DB, logger)
			var patches, salePosts atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "PATCH" {
					patches.Add(1)
					_, _ = fmt.Fprintf(w, `{"dh_inventory_id":42,"status":%q,"listing_price_cents":25000}`, status)
				} else {
					require.Equal(t, "POST", r.Method)
					require.Equal(t, "/api/v1/enterprise/inventory/42/sale", r.URL.Path)
					salePosts.Add(1)
					_, _ = fmt.Fprint(w, `{"sale_id":"deal-sale","dh_inventory_id":42,"sold_inventory_id":42,"delisted":true,"item_status":"sold"}`)
				}
			}))
			defer provider.Close()
			remote := adapter.NewInventoryAdapter(dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))).WithMutationReceipts()
			coord := inventory.NewDHMutationCoordinator(store, store)
			saleCommitted, allowStatus := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(allowStatus) })
			guard := &mocks.DHMutationGuardsMock{
				ObservationTimeFn: func(c context.Context) (time.Time, error) {
					close(saleCommitted)
					select {
					case <-allowStatus:
						return store.ObservationTime(c)
					case <-c.Done():
						return time.Time{}, c.Err()
					}
				},
				ApplyDHSoldObservationFn: store.ApplyDHSoldObservation,
			}
			inv := inventory.NewService(campaigns, repo, sales, nil, postgres.NewFinanceStore(db.DB, logger), nil, postgres.NewDHStore(db.DB, logger), inventory.WithIDGenerator(uuid.NewString), inventory.WithDisableBackgroundWorkers(), inventory.WithDHSaleRecorder(remote), inventory.WithDHMutationCoordinator(coord, store, store, guard))
			price := dhpricing.NewService(repo, remote, repo, repo, logger, dhpricing.WithMutationCoordinator(coord, store, store))
			p, e := repo.GetPurchase(ctx, id)
			require.NoError(t, e)
			campaign, e := campaigns.GetCampaign(ctx, p.CampaignID)
			require.NoError(t, e)
			sa := &inventory.Sale{PurchaseID: id, SaleDate: "2026-02-01", SalePriceCents: 26000, SaleChannel: inventory.SaleChannelLocal}
			saleDone := make(chan error, 1)
			go func() { saleDone <- inv.CreateSale(ctx, sa, campaign, p) }()
			select {
			case <-saleCommitted:
			case <-ctx.Done():
				t.Fatal("actual CreateSale did not commit before status update")
			}
			savedSale, e := sales.GetSaleByPurchaseID(ctx, id)
			require.NoError(t, e)
			require.Equal(t, sa.ID, savedSale.ID)
			stale, e := repo.GetPurchase(ctx, id)
			require.NoError(t, e)
			require.Equal(t, status, stale.DHStatus)
			result := price.SyncPurchasePrice(ctx, id)
			require.ErrorIs(t, result.Err, inventory.ErrReturnConflict)
			require.Contains(t, result.Err.Error(), "current_sale_present")
			require.Zero(t, patches.Load(), "current sale must exclude PATCH even with stale listable status")
			state, e := store.GetReturnState(ctx, id)
			require.NoError(t, e)
			require.Nil(t, state.PrecedingAttempt, "a newly prepared but never dispatched price must not fence sale retirement")
			releaseOnce.Do(func() { close(allowStatus) })
			select {
			case e := <-saleDone:
				require.NoError(t, e)
			case <-ctx.Done():
				t.Fatal("CreateSale did not finish")
			}
			require.Equal(t, int32(1), salePosts.Load(), "scoped price eligibility must not broadly ban sale mutations")
			recorded, e := sales.GetSaleByPurchaseID(ctx, id)
			require.NoError(t, e)
			require.Equal(t, "deal-sale", recorded.DHSaleID)
			require.Zero(t, patches.Load())
		})
	}
}
