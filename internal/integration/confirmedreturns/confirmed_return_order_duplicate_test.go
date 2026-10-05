//go:build integration

package confirmedreturns_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestOrderPollDuplicatePreservesSuccessfulEvent(t *testing.T) {
	db, store, _, _, _ := setupIntegrationReturn(t, "duplicate-cert")
	logger := mocks.NewMockLogger()
	svc := csvimport.NewService(csvimport.Deps{Campaigns: postgres.NewCampaignStore(db.DB, logger), Purchases: postgres.NewPurchaseStore(db.DB, logger), Sales: postgres.NewSaleStore(db.DB, logger), Finance: postgres.NewFinanceStore(db.DB, logger), MutationGuards: store, Logger: logger, IDGen: func() string { return "first-sale" }})
	client := &mocks.MockDHOrdersClient{GetOrdersFn: func(context.Context, dh.OrderFilters) (*dh.OrdersResponse, error) {
		return &dh.OrdersResponse{Orders: []dh.Order{{OrderID: "ext-999", CertNumber: "duplicate-cert", Grade: "10", SoldAt: "2026-02-07T01:00:00Z", SalePriceCents: 27273, Channel: "ebay"}, {OrderID: "ext-1000", CertNumber: "duplicate-cert", Grade: "10", SoldAt: "2026-03-01T01:00:00Z", SalePriceCents: 45000, Channel: "ebay"}}, Meta: dh.PaginationMeta{TotalCount: 2}}, nil
	}}
	events := &mocks.MockEventRecorder{}
	s := scheduler.NewDHOrdersPollScheduler(client, nil, svc, events, logger, scheduler.DHOrdersPollConfig{})
	summary, e := s.RunOnce(context.Background(), "")
	require.NoError(t, e)
	require.Equal(t, 1, summary.Matched)
	require.Equal(t, 1, summary.Failed)
	count := 0
	for _, event := range events.Events {
		if event.Type == dhevents.TypeSold {
			count++
			require.Equal(t, "ext-999", event.DHOrderID)
			require.Equal(t, 27273, event.SalePriceCents)
		}
	}
	require.Equal(t, 1, count)
}
