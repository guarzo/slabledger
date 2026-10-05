package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestReturnEnabledBuilderMutatorsFailClosed(t *testing.T) {
	cfg := config.Default()
	logger := mocks.NewMockLogger()
	remoteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { remoteCalls++; w.WriteHeader(500) }))
	defer server.Close()
	client := dh.NewClient(server.URL, dh.WithEnterpriseKey("fixture"))
	store := mocks.NewInMemoryCampaignStore()
	store.Sales["sale"] = &inventory.Sale{ID: "sale", PurchaseID: "p", SaleDate: "2026-01-02", SalePriceCents: 5000}
	repo := &mocks.PurchaseRepositoryMock{}
	saleRecorder := &mocks.DHSaleRecorderMock{}
	deps := BuildDeps{DHMutationRequired: true, Logger: logger, DHClient: client, DHOrdersClient: client, DHInventoryListClient: client, SyncStateStore: newMockSyncStateStore(), DHFieldsUpdater: repo, PurchaseByCertLookup: &mocks.MockPurchaseByCertLookup{}, DHPushPendingLister: repo, DHPushStatusUpdater: repo, DHPushCardIDSaver: &mocks.DHCardIDSaverMock{}, PurchaseRepo: repo, DHSaleStore: store, DHSaleRecorder: saleRecorder}
	poll := buildDHInventoryPollScheduler(&cfg, deps)
	require.NotNil(t, poll)
	poll.poll(context.Background())
	require.Zero(t, remoteCalls)
	push := buildDHPushScheduler(&cfg, deps)
	require.NotNil(t, push)
	push.processPurchase(context.Background(), inventory.Purchase{ID: "p", CertNumber: "cert", BuyCostCents: 1000}, inventory.DefaultDHPushConfig())
	require.Zero(t, remoteCalls)
	sold := buildDHSoldReconcilerScheduler(&cfg, deps)
	require.NotNil(t, sold)
	require.ErrorIs(t, sold.recordSale(context.Background(), &inventory.Purchase{ID: "p", DHInventoryID: 42}, store.Sales["sale"]), inventory.ErrReturnConflict)
	require.Empty(t, saleRecorder.RecordedSales())
}
