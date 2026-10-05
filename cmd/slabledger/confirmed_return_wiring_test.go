package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnLegacyCASCapability(t *testing.T) {
	for _, tc := range []struct {
		name      string
		service   inventory.Service
		available bool
	}{
		{"supported", inventory.NewService(nil, nil, nil, nil, nil, nil, nil, inventory.WithIDGenerator(func() string { return "id" })), true},
		{"unsupported", &mocks.MockInventoryService{}, false},
		{"absent", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := mocks.NewCapturingLogger()
			var unsell inventory.LegacyConfirmedUnsell
			require.NotPanics(t, func() { unsell = confirmedReturnLegacyUnsell(context.Background(), tc.service, logger) })
			if tc.available {
				require.NotNil(t, unsell)
				require.ErrorIs(t, unsell(context.Background(), "purchase", "sale"), inventory.ErrReturnConflict)
				require.Empty(t, logger.Entries())
			} else {
				require.Nil(t, unsell)
				require.Len(t, logger.Entries(), 1)
				require.Equal(t, "error", logger.Entries()[0].Level)
				require.Contains(t, logger.Entries()[0].Message, "sale CAS")
			}
		})
	}
}

func TestConfirmedReturnRuntimeForwarding(t *testing.T) {
	store := &postgres.ConfirmedReturnStore{}
	coord := inventory.NewDHMutationCoordinator(store, store)
	returns := inventory.NewConfirmedReturnService(store, store, nil, nil, func() string { return "id" })
	w := &wiring{campaignsInit: campaignsInitResult{returnStore: store, mutationCoordinator: coord, confirmedReturns: returns}}
	h := w.handlerInputs(nil, mocks.NewMockLogger(), nil)
	s := w.schedulerDeps(nil, mocks.NewMockLogger())
	require.Same(t, store, h.ReturnStore)
	require.Same(t, coord, h.MutationCoordinator)
	require.Same(t, returns, h.ConfirmedReturns)
	require.Same(t, store, s.ReturnStore)
	require.Same(t, coord, s.MutationCoordinator)
}
func TestHTTPListingWiringMissingStoreIsFailClosed(t *testing.T) {
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(500) }))
	defer provider.Close()
	received := "2026-01-01"
	p := &inventory.Purchase{ID: "p", CertNumber: "cert", ReceivedAt: &received, DHInventoryID: 42, DHStatus: "in_stock", ReviewedPriceCents: 25000}
	inventorySvc := &mocks.MockInventoryService{GetPurchasesByCertNumbersFn: func(context.Context, []string) (map[string]*inventory.Purchase, error) {
		return map[string]*inventory.Purchase{"cert": p}, nil
	}}
	listing := buildHTTPListingService(context.Background(), handlerInputs{DHClient: dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture")), CampaignsService: inventorySvc, Logger: mocks.NewMockLogger()})
	require.NotNil(t, listing)
	result := listing.ListPurchases(context.Background(), []string{"cert"})
	require.ErrorIs(t, result.Error, inventory.ErrReturnConflict)
	require.Zero(t, calls)
	require.Implements(t, (*dhlisting.ExplicitListingService)(nil), listing)
}
