package handlers

import (
	"context"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"testing"
)

func TestConfiguredPushRequiresExactReceipt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *dh.InventoryPushResponse
	}{{"nil", nil}, {"wrong-cert", &dh.InventoryPushResponse{Results: []dh.InventoryResult{{CertNumber: "another", DHInventoryID: 42, Status: "in_stock"}}}}, {"unknown-status", &dh.InventoryPushResponse{Results: []dh.InventoryResult{{CertNumber: "cert", DHInventoryID: 42, Status: "unknown"}}}}} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			updater := &mocks.MockDHFieldsUpdater{UpdatePurchaseDHFieldsFn: func(context.Context, string, inventory.DHFieldsUpdate) error { writes++; return nil }}
			h := NewDHHandler(DHHandlerDeps{MutationRequired: true, Logger: mocks.NewMockLogger(), DHFieldsUpdater: updater, InventoryPusher: &mocks.DHInventoryPusherMock{PushInventoryFn: func(context.Context, []dh.InventoryItem) (*dh.InventoryPushResponse, error) { return tc.response, nil }}})
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("malformed receipt panic: %v", r)
				}
			}()
			if _, e := h.pushAndPersistDH(context.Background(), &inventory.Purchase{ID: "p1", CertNumber: "cert"}, 123, 25000); e == nil {
				t.Fatal("invalid receipt accepted")
			}
			if writes != 0 {
				t.Fatal("invalid receipt persisted")
			}
		})
	}
}
