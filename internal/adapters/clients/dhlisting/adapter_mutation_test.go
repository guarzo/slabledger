package dhlisting_test

import (
	"context"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"testing"
)

func TestInitialPatchSoldRejectionClassification(t *testing.T) {
	for _, tt := range []struct {
		name, message   string
		uncertain, want bool
	}{{"known", "Cannot update item with status 'sold'", false, true}, {"prior uncertainty", "Cannot update item with status 'sold'", true, false}, {"other422", "Invalid price", false, false}} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpx.RequestError{Uncertain: tt.uncertain, Err: &httpx.UpstreamError{StatusCode: 422, Message: tt.message}}
			client := &mocks.DHInventoryMutationClientMock{UpdateInventoryFn: func(context.Context, int, dh.InventoryUpdate) (*dh.InventoryResult, error) { return nil, upstream }}
			_, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(context.Background(), 42, inventory.DHInventoryStatusUpdate{Status: "listed"})
			if inventory.IsDefinitiveDHNonMutation(err) != tt.want {
				t.Fatalf("classification=%v err=%v", inventory.IsDefinitiveDHNonMutation(err), err)
			}
		})
	}
}
func TestVerifiedVoidRejectsWrongTarget(t *testing.T) {
	client := &mocks.DHInventoryMutationClientMock{VoidInventorySaleFn: func(context.Context, string, dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
		return &dh.VoidSaleResponse{Reversed: true, Items: []dh.InventoryListItem{{DHInventoryID: 99, Status: "in_stock"}}}, nil
	}}
	a := adapter.NewInventoryAdapter(client)
	if e := a.VoidInventorySaleVerified(context.Background(), "handle", 42, "un-sell"); e == nil {
		t.Fatal("void receipt for another inventory accepted")
	}
}
func TestCoordinatedAdapterRequiresMutationReceipt(t *testing.T) {
	client := &mocks.DHInventoryMutationClientMock{}
	a := adapter.NewInventoryAdapter(client).WithMutationReceipts()
	if _, e := a.UpdateInventoryStatus(context.Background(), 42, inventory.DHInventoryStatusUpdate{Status: "listed"}); e == nil {
		t.Fatal("nil PATCH receipt accepted")
	}
	if e := a.SyncChannels(context.Background(), 42, []string{"ebay"}); e == nil {
		t.Fatal("nil sync receipt accepted")
	}
}
