//go:build integration

package confirmedreturns_test

import (
	"context"
	"io"
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

func TestIntegratedUnkeyedRequestOutlivesCaller(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "outlive-cert")
	ctx := context.Background()
	logger := mocks.NewMockLogger()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	var patches, returns atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "PATCH":
			if patches.Add(1) == 1 {
				close(started)
			}
			<-release // deliberately ignore r.Context().Done: work can survive cancellation
			_, _ = io.WriteString(w, `{"dh_inventory_id":42,"status":"in_stock","listing_price_cents":25000}`)
			close(finished)
		case "GET":
			_, _ = io.WriteString(w, `{"results":[{"dh_inventory_id":42,"cert_number":"outlive-cert","status":"in_stock"}],"meta":{"total_count":1}}`)
		default:
			returns.Add(1)
			w.WriteHeader(500)
		}
	}))
	defer func() { releaseOnce.Do(func() { close(release) }); provider.Close() }()
	client := dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))
	remote := adapter.NewInventoryAdapter(client).WithMutationReceipts()
	repo := postgres.NewPurchaseStore(db.DB, logger)
	price := dhpricing.NewService(repo, remote, repo, repo, logger, dhpricing.WithMutationCoordinator(inventory.NewDHMutationCoordinator(store, store), store, store))
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan dhpricing.SyncResult, 1)
	go func() { done <- price.SyncPurchasePrice(caller, id) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("PATCH never dispatched")
	}
	cancel()
	select {
	case result := <-done:
		require.Error(t, result.Err)
	case <-time.After(5 * time.Second):
		t.Fatal("caller ownership did not end")
	}
	state, e := store.GetReturnState(ctx, id)
	require.NoError(t, e)
	require.NotNil(t, state.PrecedingAttempt)
	require.Nil(t, state.Operation)
	// Simulate age beyond the resource bound; age is never completion evidence.
	_, e = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET started_at=clock_timestamp()-interval '95 seconds' WHERE id=$1`, state.PrecedingAttempt.ID)
	require.NoError(t, e)
	status, e := remote.GetReturnInventoryStatus(ctx, 42, "outlive-cert")
	require.NoError(t, e)
	require.Equal(t, "in_stock", status)
	returnsSvc := inventory.NewConfirmedReturnService(store, store, remote, nil, uuid.NewString)
	_, e = returnsSvc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedTarget("outlive-cert")})
	require.ErrorIs(t, e, inventory.ErrReturnConflict)
	require.Contains(t, e.Error(), "preceding_dh_mutation_uncertain")
	require.Error(t, price.SyncPurchasePrice(ctx, id).Err)
	require.Equal(t, int32(1), patches.Load())
	require.Zero(t, returns.Load())
	releaseOnce.Do(func() { close(release) })
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("remote work did not continue")
	}
	_, e = returnsSvc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedTarget("outlive-cert")})
	require.ErrorIs(t, e, inventory.ErrReturnConflict)
	state, e = store.GetReturnState(ctx, id)
	require.NoError(t, e)
	require.NotNil(t, state.PrecedingAttempt)
	require.Equal(t, "open", state.PrecedingAttempt.Outcome)
	require.Equal(t, int32(1), patches.Load(), "unsafe transport attempted retry")
}
