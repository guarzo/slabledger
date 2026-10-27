package dhlisting_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestVerifiedVoidItemStatus(t *testing.T) {
	for _, tt := range []struct {
		name, wire string
		ok         bool
	}{
		{"exact restoration", `{"reversed":true,"items":[{"dh_inventory_id":42,"item_status":"in_stock"}]}`, true},
		{"wrong target", `{"reversed":true,"items":[{"dh_inventory_id":99,"item_status":"in_stock"}]}`, false},
		{"wrong status", `{"reversed":true,"items":[{"dh_inventory_id":42,"item_status":"sold"}]}`, false},
		{"GET status is not a void receipt", `{"reversed":true,"items":[{"dh_inventory_id":42,"status":"in_stock"}]}`, false},
		{"already void empty", `{"reversed":false,"items":[]}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var receipt dh.VoidSaleResponse
			require.NoError(t, json.Unmarshal([]byte(tt.wire), &receipt))
			client := &mocks.DHInventoryMutationClientMock{VoidInventorySaleFn: func(context.Context, string, dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) { return &receipt, nil }}
			err := adapter.NewInventoryAdapter(client).VoidInventorySaleVerified(context.Background(), "sale", 42, "returned")
			if tt.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
