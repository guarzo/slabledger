//go:build integration

package confirmedreturns_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestCoordinatedListingRotationKeepsOpenFence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		intake, flag bool
	}{
		{"patch_unknown_503", false, false},
		{"intake_rate_limited_flag", true, true},
		{"intake_rate_limited_text", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, returns, _, id := setupIntegrationReturn(t, "rotation-cert")
			ctx := context.Background()
			if tc.intake {
				_, e := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=0,dh_push_status='pending' WHERE id=$1`, id)
				require.NoError(t, e)
			}
			var mutations atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" {
					_, _ = fmt.Fprint(w, `{"results":[{"dh_inventory_id":42,"cert_number":"rotation-cert","status":"in_stock"}],"meta":{"total_count":1}}`)
					return
				}
				if mutations.Add(1) == 1 {
					if tc.intake {
						require.Equal(t, "POST", r.Method)
						require.Equal(t, "/api/v1/enterprise/inventory/psa_import", r.URL.Path)
						_, _ = fmt.Fprintf(w, `{"success":true,"results":[{"cert_number":"rotation-cert","resolution":"psa_error","error":"PSA API rate limit exceeded","rate_limited":%t}]}`, tc.flag)
					} else {
						require.Equal(t, "PATCH", r.Method)
						require.Equal(t, "/api/v1/enterprise/inventory/42", r.URL.Path)
						w.WriteHeader(503)
						_, _ = fmt.Fprint(w, `{"error":"PSA API rate limit exceeded"}`)
					}
					return
				}
				switch {
				case r.URL.Path == "/api/v1/enterprise/inventory/psa_import":
					_, _ = fmt.Fprint(w, `{"success":true,"results":[{"cert_number":"rotation-cert","resolution":"matched","dh_card_id":123,"dh_inventory_id":42,"status":"in_stock"}]}`)
				case r.Method == "PATCH":
					_, _ = fmt.Fprint(w, `{"dh_inventory_id":42,"status":"listed","listing_price_cents":25000}`)
				default:
					_, _ = fmt.Fprint(w, `{"dh_inventory_id":42,"status":"listed","channels":[{"name":"ebay","status":"pending"},{"name":"shopify","status":"pending"}]}`)
				}
			}))
			defer provider.Close()
			client := dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithPSAKeys("key1,key2"), dh.WithRateLimitRPS(1000))
			remote := adapter.NewInventoryAdapter(client).WithMutationReceipts()
			repo := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
			listing, e := dhlisting.NewDHListingService(repo, mocks.NewMockLogger(),
				dhlisting.WithDHListingLister(remote), dhlisting.WithDHListingFieldsUpdater(repo),
				dhlisting.WithDHListingConfigLoader(postgres.NewDHStore(db.DB, mocks.NewMockLogger())),
				dhlisting.WithDHListingPSAImporter(adapter.NewPSAImporterAdapter(client)),
				dhlisting.WithDHListingCardIDSaver(&mocks.DHCardIDSaverMock{}),
				dhlisting.WithDHListingPushStatusUpdater(repo),
				dhlisting.WithDHListingMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store))
			require.NoError(t, e)
			result := listing.ListPurchases(ctx, []string{"rotation-cert"})
			require.Zero(t, result.Listed, "earlier unknown mutation must not be hidden by rotation")
			require.Equal(t, int32(1), mutations.Load())
			state, e := store.GetReturnState(ctx, id)
			require.NoError(t, e)
			require.NotNil(t, state.PrecedingAttempt)
			attemptID := state.PrecedingAttempt.ID
			require.Equal(t, "open", state.PrecedingAttempt.Outcome)
			_, e = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET started_at=clock_timestamp()-interval '95 seconds' WHERE id=$1`, state.PrecedingAttempt.ID)
			require.NoError(t, e)
			_, e = remote.GetReturnInventoryStatus(ctx, 42, "rotation-cert")
			require.NoError(t, e)
			state, e = returns.GetReturnState(ctx, id)
			require.NoError(t, e)
			require.NotNil(t, state.PrecedingAttempt)
			require.Equal(t, attemptID, state.PrecedingAttempt.ID)
			require.Equal(t, "open", state.PrecedingAttempt.Outcome)
			require.Zero(t, listing.ListPurchases(ctx, []string{"rotation-cert"}).Listed)
			require.Equal(t, int32(1), mutations.Load())
		})
	}
}
