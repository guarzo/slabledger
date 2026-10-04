package scheduler

import (
	"context"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"testing"
)

func TestDHSoldReconcilerConfiguredMissingCoordination(t *testing.T) {
	store := mocks.NewInMemoryCampaignStore()
	store.Sales["s-p-42"] = saleFor("p-42")
	recorder := &mocks.DHSaleRecorderMock{}
	statuses := []string{}
	repo := resolverFor(map[int]inventory.DHStatus{42: "sold"}, nil)
	s := NewDHSoldReconcilerScheduler(store, store, mocks.NewMockLogger(), DHSoldReconcilerConfig{},
		WithDHSoldSweep(listClientByStatus(map[string][]dh.InventoryListItem{"listed": {invItem("cert", 42)}}, nil, &statuses), repo, store, recorder, store, store),
		WithDHSaleHandleRecovery(store, recorder, store, store), WithDHSoldMutationCoordinator(nil, nil, repo, store))
	s.sweepDH(context.Background())
	s.recoverDHSaleHandles(context.Background())
	if len(recorder.RecordedSales()) != 0 {
		t.Fatal("missing configured coordination dispatched a DH sale")
	}
}
