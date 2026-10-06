//go:build integration

package confirmedreturns_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestReturnTargetChangesBetweenHTTPReadAndPOST(t *testing.T) {
	const cert = "http-target-drift-cert"
	db, store, _, _, id := setupIntegrationReturn(t, cert)
	ctx := context.Background()
	var dispatches atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/enterprise/inventory" {
			_, _ = fmt.Fprint(w, `{"results":[{"dh_inventory_id":42,"cert_number":"http-target-drift-cert","status":"sold"}],"meta":{"total_count":1}}`)
			return
		}
		dispatches.Add(1)
		t.Errorf("changed target reached provider: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	defer provider.Close()
	adapter := dhadapter.NewInventoryAdapter(dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))).WithMutationReceipts()
	returnsSvc := inventory.NewConfirmedReturnService(store, store, adapter, nil, uuid.NewString)
	logger := mocks.NewMockLogger()
	handler := handlers.NewCampaignsHandler(nil, nil, nil, nil, logger, nil, handlers.WithConfirmedReturnService(returnsSvc))
	api := httptest.NewServer(httpserver.NewRouter(httpserver.RouterConfig{CampaignsHandler: handler, LocalAPIToken: "local-fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)}).Setup())
	defer api.Close()
	path := "/api/purchases/" + id
	code, raw := returnHTTP(t, api, "GET", path+"/confirmed-return", "", true)
	require.Equal(t, 200, code, string(raw))
	var state inventory.ConfirmedReturnState
	require.NoError(t, json.Unmarshal(raw, &state))
	require.Equal(t, 42, state.Purchase.DHInventoryID)
	// This local linkage change occurs after the operator observed target 42.
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=99 WHERE id=$1`, id)
	require.NoError(t, err)
	code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", `{"returnConfirmed":true,"expectedSaleId":null,"expectedTarget":{"dhInventoryId":42,"certNumber":"http-target-drift-cert","grader":"PSA"}}`, true)
	require.Equal(t, 409, code, string(raw))
	require.Contains(t, string(raw), "identity_conflict")
	require.Zero(t, dispatches.Load())
	var episodes, attempts int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM confirmed_dh_returns),(SELECT count(*) FROM dh_mutation_attempts)`).Scan(&episodes, &attempts))
	require.Zero(t, episodes)
	require.Zero(t, attempts)
}
