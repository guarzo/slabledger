package scheduler

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"testing"
)

func TestDHPushMissingRequiredCoordination(t *testing.T) {
	importer := &mocks.DHPSAImportClientMock{}
	s := NewDHPushScheduler(nil, &mocks.MockDHPushStatusUpdater{}, importer, &mocks.MockDHFieldsUpdater{}, &mocks.DHCardIDSaverMock{}, mocks.NewMockLogger(), DHPushConfig{}, WithDHPushMutationCoordinator(nil, nil, nil, nil))
	s.processPurchase(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "cert", BuyCostCents: 1000, DHPushStatus: "pending"}, inventory.DefaultDHPushConfig())
	if importer.Calls != 0 {
		t.Fatalf("unsafe PSA import calls=%d", importer.Calls)
	}
}
