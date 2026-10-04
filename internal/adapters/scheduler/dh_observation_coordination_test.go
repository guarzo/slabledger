package scheduler

import (
	"context"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"testing"
)

func TestInventoryPollMissingObservationCoordination(t *testing.T) {
	fetches := 0
	client := &mocks.MockDHInventoryListClient{ListInventoryFn: func(context.Context, dh.InventoryFilters) (*dh.InventoryListResponse, error) {
		fetches++
		return &dh.InventoryListResponse{}, nil
	}}
	s := NewDHInventoryPollScheduler(client, nil, nil, nil, nil, mocks.NewMockLogger(), DHInventoryPollConfig{}, WithDHInventoryObservationGuards(nil))
	s.poll(context.Background())
	if fetches != 0 {
		t.Fatal("uncoordinated snapshot fetched")
	}
}
