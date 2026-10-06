//go:build integration

package confirmedreturns_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfiguredSchedulerPushAcceptance(t *testing.T) {
	for _, mode := range []string{"success", "uncertain", "fresh-dismissed", "config-error", "return-hold", "unsupported-grader"} {
		t.Run(mode, func(t *testing.T) {
			db, store, returns, fake, id := setupIntegrationReturn(t, "push-cert")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			logger := mocks.NewMockLogger()
			_, e := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=0,dh_push_status='pending',buy_cost_cents=1000 WHERE id=$1`, id)
			require.NoError(t, e)
			repo := postgres.NewPurchaseStore(db.DB, logger)
			cached, e := repo.GetPurchase(ctx, id)
			require.NoError(t, e)
			if mode == "return-hold" {
				_, e = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=42 WHERE id=$1`, id)
				require.NoError(t, e)
				fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
					return &inventory.DHReturnResult{DHInventoryID: 42, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
				}
				_, e = returns.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedTarget("push-cert")})
				require.NoError(t, e)
			}
			remote := &mocks.DHPSAImportClientMock{PSAImportFn: func(_ context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
				require.Len(t, items, 1)
				require.Equal(t, 1777, items[0].CostBasisCents)
				require.Equal(t, "push-cert", items[0].CertNumber)
				if mode == "uncertain" {
					return nil, context.DeadlineExceeded
				}
				return &dh.PSAImportResponse{Success: true, Results: []dh.PSAImportResult{{CertNumber: "push-cert", DHCardID: 123, DHInventoryID: 99, Status: "in_stock", Resolution: dh.PSAImportStatusMatched}}}, nil
			}}
			var cfg scheduler.DHPushConfigLoader = postgres.NewDHStore(db.DB, logger)
			if mode == "config-error" {
				cfg = &mocks.InMemoryCampaignStore{GetDHPushConfigFn: func(context.Context) (*inventory.DHPushConfig, error) { return nil, errors.New("config unavailable") }}
			}
			var s *scheduler.DHPushScheduler
			pending := &mocks.PurchaseRepositoryMock{GetPurchasesByDHPushStatusFn: func(c context.Context, _ string, _ int) ([]inventory.Purchase, error) {
				_, e := db.ExecContext(c, `UPDATE campaign_purchases SET buy_cost_cents=1777 WHERE id=$1`, id)
				require.NoError(t, e)
				if mode == "fresh-dismissed" {
					_, e = db.ExecContext(c, `UPDATE campaign_purchases SET dh_push_status='dismissed' WHERE id=$1`, id)
					require.NoError(t, e)
				}
				if mode == "unsupported-grader" {
					_, e = db.ExecContext(c, `UPDATE campaign_purchases SET grader='CGC' WHERE id=$1`, id)
					require.NoError(t, e)
				}
				s.Stop()
				return []inventory.Purchase{*cached}, nil
			}}
			s = scheduler.NewDHPushScheduler(pending, repo, remote, repo, postgres.NewCardIDMappingRepository(db.DB), logger, scheduler.DHPushConfig{Enabled: true, Interval: time.Hour}, scheduler.WithDHPushConfigLoader(cfg), scheduler.WithDHPushHoldSetter(repo), scheduler.WithDHPushMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store, repo))
			s.Start(ctx)
			require.NoError(t, ctx.Err())
			state, e := store.GetReturnState(ctx, id)
			require.NoError(t, e)
			switch mode {
			case "success":
				require.Equal(t, 1, remote.Calls)
				require.Nil(t, state.PrecedingAttempt)
				require.Equal(t, 99, state.Purchase.DHInventoryID)
				require.Equal(t, "matched", state.Purchase.DHPushStatus)
			case "uncertain":
				require.Equal(t, 1, remote.Calls)
				require.NotNil(t, state.PrecedingAttempt)
				require.Zero(t, state.Purchase.DHInventoryID)
			default:
				require.Zero(t, remote.Calls)
				require.Nil(t, state.PrecedingAttempt)
				if mode == "return-hold" {
					require.True(t, state.AwaitingListing)
				}
			}
		})
	}
}
func TestConfiguredBulkMatchAcceptance(t *testing.T) {
	for _, mode := range []string{"success", "uncertain", "fresh-dismissed", "missing-fields", "return-hold", "unsupported-grader", "fresh-sale"} {
		t.Run(mode, func(t *testing.T) {
			db, store, returns, fake, id := setupIntegrationReturn(t, "bulk-cert")
			ctx := context.Background()
			logger := mocks.NewMockLogger()
			repo := postgres.NewPurchaseStore(db.DB, logger)
			_, e := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=0,dh_push_status='pending',buy_cost_cents=1000 WHERE id=$1`, id)
			require.NoError(t, e)
			cached, e := repo.GetPurchase(ctx, id)
			require.NoError(t, e)
			if mode == "return-hold" {
				_, e = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=42 WHERE id=$1`, id)
				require.NoError(t, e)
				fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
					return &inventory.DHReturnResult{DHInventoryID: 42, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
				}
				_, e = returns.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedTarget("bulk-cert")})
				require.NoError(t, e)
			}
			discovery := &mocks.PurchaseRepositoryMock{GetPurchaseFn: repo.GetPurchase, ListAllUnsoldPurchasesFn: func(c context.Context) ([]inventory.Purchase, error) {
				_, e := db.ExecContext(c, `UPDATE campaign_purchases SET buy_cost_cents=1777 WHERE id=$1`, id)
				require.NoError(t, e)
				if mode == "fresh-dismissed" {
					_, e = db.ExecContext(c, `UPDATE campaign_purchases SET dh_push_status='dismissed' WHERE id=$1`, id)
					require.NoError(t, e)
				}
				if mode == "unsupported-grader" {
					_, e = db.ExecContext(c, `UPDATE campaign_purchases SET grader='CGC' WHERE id=$1`, id)
					require.NoError(t, e)
				}
				if mode == "fresh-sale" {
					_, e = db.ExecContext(c, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,created_at,updated_at) VALUES('new-sale','return-p','local',12000,'2026-01-04',now(),now())`)
					require.NoError(t, e)
				}
				return []inventory.Purchase{*cached}, nil
			}}
			remote := &mocks.DHInventoryPusherMock{PushInventoryFn: func(_ context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
				require.Len(t, items, 1)
				require.Equal(t, 1777, items[0].CostBasisCents)
				require.Equal(t, "bulk-cert", items[0].CertNumber)
				if mode == "uncertain" {
					return nil, context.DeadlineExceeded
				}
				return &dh.InventoryPushResponse{Results: []dh.InventoryResult{{DHInventoryID: 99, CertNumber: "bulk-cert", Status: "in_stock", AssignedPriceCents: 25000}}}, nil
			}}
			saver := &mocks.DHCardIDSaverMock{GetMappedSetFn: func(context.Context, string) (map[string]string, error) {
				return map[string]string{cached.DHCardKey(): "123"}, nil
			}, GetExternalIDFn: func(context.Context, string, string, string, string) (string, error) { return "123", nil }}
			var writer handlers.DHFieldsUpdater = repo
			if mode == "missing-fields" {
				writer = nil
			}
			h := handlers.NewDHHandler(handlers.DHHandlerDeps{Logger: logger, PurchaseLister: discovery, CardIDSaver: saver, InventoryPusher: remote, DHFieldsUpdater: writer, PushStatusUpdater: repo, CandidatesSaver: repo, MutationRequired: true, MutationCoordinator: inventory.NewDHMutationCoordinator(store, store), MutationGuards: store})
			r := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
			r = r.WithContext(context.WithValue(ctx, middleware.UserContextKey, &auth.User{ID: 1}))
			w := httptest.NewRecorder()
			h.HandleBulkMatch(w, r)
			require.Equal(t, 202, w.Code)
			h.Wait()
			state, e := store.GetReturnState(ctx, id)
			require.NoError(t, e)
			switch mode {
			case "success":
				require.Equal(t, 1, remote.CallCount)
				require.Nil(t, state.PrecedingAttempt)
				require.Equal(t, 99, state.Purchase.DHInventoryID)
			case "uncertain":
				require.Equal(t, 1, remote.CallCount)
				require.NotNil(t, state.PrecedingAttempt)
				require.Zero(t, state.Purchase.DHInventoryID)
			default:
				require.Zero(t, remote.CallCount)
				if mode == "return-hold" {
					require.True(t, state.AwaitingListing)
				}
			}
		})
	}
}
