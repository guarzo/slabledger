//go:build integration

package confirmedreturns_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnHTTPLocalCapability(t *testing.T) {
	for _, tc := range []struct {
		name                                                 string
		local, completed, typedScope, typedRepo, typedRemote bool
		getStatus, postStatus                                int
	}{
		{name: "authenticated no DH local sale", local: true, getStatus: 200, postStatus: 200},
		{name: "completed state and replay DH disabled", completed: true, getStatus: 200, postStatus: 200},
		{name: "completed state and replay typed nil returner", completed: true, typedRemote: true, getStatus: 200, postStatus: 200},
		{name: "external return missing capability", getStatus: 200, postStatus: 503},
		{name: "external return typed nil capability", typedRemote: true, getStatus: 200, postStatus: 503},
		{name: "typed nil durable repository", typedRepo: true, getStatus: 503, postStatus: 503},
		{name: "typed nil service scope", typedScope: true, getStatus: 200, postStatus: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, initial, fake, id := setupIntegrationReturn(t, "http-local-capability")
			ctx := context.Background()
			body := `{"returnConfirmed":true,"expectedSaleId":null}`
			calls := 0
			if tc.local {
				_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=0 WHERE id=$1`, id)
				require.NoError(t, err)
				_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,created_at,updated_at) VALUES('local-sale',$1,'local',10000,'2026-01-03',now(),now())`, id)
				require.NoError(t, err)
				body = `{"returnConfirmed":true,"expectedSaleId":"local-sale"}`
			}
			if !tc.local && !tc.completed {
				body = `{"returnConfirmed":true,"expectedSaleId":null,"expectedTarget":{"dhInventoryId":42,"certNumber":"http-local-capability","grader":"PSA"}}`
			}
			if tc.completed {
				fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
					calls++
					return &inventory.DHReturnResult{DHInventoryID: 42, ExternalSaleID: 848, ItemStatus: "in_stock"}, nil
				}
				_, err := initial.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedTarget("http-local-capability")})
				require.NoError(t, err)
			}
			var scope inventory.PurchaseMutationScope = store
			var repo inventory.ConfirmedReturnRepository = store
			var remote inventory.DHReturner
			if tc.typedScope {
				scope = (*postgres.PurchaseMutationCoordinator)(nil)
			}
			if tc.typedRepo {
				repo = (*postgres.ConfirmedReturnStore)(nil)
			}
			if tc.typedRemote {
				remote = (*mocks.DHReturnerMock)(nil)
			}
			var generateID func() string
			if !tc.local && !tc.completed {
				generateID = uuid.NewString
			}
			if tc.typedRepo || tc.typedScope {
				remote = fake
			}
			returns := inventory.NewConfirmedReturnService(scope, repo, remote, nil, generateID)
			logger := mocks.NewMockLogger()
			purchases := postgres.NewPurchaseStore(db.DB, logger)
			inv := inventory.NewService(postgres.NewCampaignStore(db.DB, logger), purchases, postgres.NewSaleStore(db.DB, logger), nil, postgres.NewFinanceStore(db.DB, logger), nil, postgres.NewDHStore(db.DB, logger), inventory.WithIDGenerator(uuid.NewString), inventory.WithDisableBackgroundWorkers(), inventory.WithDHMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store, store))
			// Use the actual Router fallback and local-token auth, not direct handler invocation.
			api := httptest.NewServer(httpserver.NewRouter(httpserver.RouterConfig{CampaignsService: inv, ConfirmedReturnService: returns, LocalAPIToken: "local-fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)}).Setup())
			defer api.Close()
			path := "/api/purchases/" + id
			for _, endpoint := range []struct{ method, suffix string }{{"GET", "/confirmed-return"}, {"POST", "/confirm-return"}} {
				code, _ := returnHTTP(t, api, endpoint.method, path+endpoint.suffix, body, false)
				require.Equal(t, 401, code)
			}
			code, raw := returnHTTP(t, api, "GET", path+"/confirmed-return", "", true)
			require.Equal(t, tc.getStatus, code, string(raw))
			var before inventory.ConfirmedReturnState
			if code == 200 {
				require.NoError(t, json.Unmarshal(raw, &before))
				require.Equal(t, tc.completed, before.AwaitingListing)
			} else {
				require.Contains(t, string(raw), `"code":"coordination_unavailable"`)
			}
			code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", body, true)
			require.Equal(t, tc.postStatus, code, string(raw))
			if code == 503 {
				require.Contains(t, string(raw), `"code":"coordination_unavailable"`)
			}
			fresh, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			if tc.local {
				var result inventory.ConfirmedReturnState
				require.NoError(t, json.Unmarshal(raw, &result))
				require.Equal(t, "local", result.Outcome)
				require.Nil(t, result.Sale)
				require.Nil(t, fresh.Sale)
				require.Equal(t, before.Purchase, result.Purchase)
			} else if tc.completed {
				var result inventory.ConfirmedReturnState
				require.NoError(t, json.Unmarshal(raw, &result))
				require.Equal(t, "completed_replay", result.Outcome)
				require.True(t, result.AwaitingListing)
				require.Equal(t, before.Operation, result.Operation)
				require.Equal(t, before.Purchase, result.Purchase)
				require.Equal(t, 1, calls)
			}
			var operations, attempts int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM confirmed_dh_returns),(SELECT count(*) FROM dh_mutation_attempts)`).Scan(&operations, &attempts))
			if tc.completed {
				require.Equal(t, 1, operations)
				require.Equal(t, 1, attempts)
				require.True(t, fresh.AwaitingListing)
			} else {
				require.Zero(t, operations)
				require.Zero(t, attempts)
			}
		})
	}
}
