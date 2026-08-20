package dhlisting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestInventoryAdapter_RecordInventorySale(t *testing.T) {
	t.Run("translates the request, formats sold_at as UTC RFC3339, translates the response", func(t *testing.T) {
		client := &stubInventoryClient{
			recordSaleFn: func(_ context.Context, inventoryID int, _ string, _ dh.InventorySaleRequest) (*dh.InventorySaleResponse, error) {
				return &dh.InventorySaleResponse{
					SaleID:              "sale_1",
					DHInventoryID:       inventoryID,
					SoldInventoryID:     dh.IntPtr(inventoryID),
					ItemStatus:          "sold",
					Delisted:            true,
					RealizedProfitCents: 2500,
				}, nil
			},
		}

		// A non-UTC input zone: the adapter must normalise before formatting,
		// or a retry from a differently-zoned process would change the body.
		soldAt := time.Date(2026, 8, 15, 9, 22, 0, 0, time.FixedZone("EDT", -4*3600))
		result, err := adapter.NewInventoryAdapter(client).RecordInventorySale(context.Background(), inventory.DHSaleRequest{
			DHInventoryID:    314,
			IdempotencyKey:   "slabledger-sale-abc",
			SalePriceCents:   45000,
			SoldAt:           soldAt,
			CounterpartyName: "Jane Buyer",
			Notes:            "in person",
		})

		require.NoError(t, err)
		require.Len(t, client.recordSaleCalls, 1)
		call := client.recordSaleCalls[0]
		require.Equal(t, 314, call.inventoryID)
		require.Equal(t, "slabledger-sale-abc", call.idempotencyKey)
		require.Equal(t, 45000, call.req.SalePriceCents)
		require.Equal(t, "2026-08-15T13:22:00Z", call.req.SoldAt, "sold_at must be UTC-normalised RFC3339 regardless of the input zone")
		require.Equal(t, "Jane Buyer", call.req.CounterpartyName)

		require.Equal(t, "sale_1", result.DHSaleID)
		require.Equal(t, 314, *result.SoldInventoryID)
		require.True(t, result.Delisted)
		require.Equal(t, 2500, result.RealizedProfitCents)
	})

	t.Run("classifies a client error into a domain sentinel", func(t *testing.T) {
		client := &stubInventoryClient{
			recordSaleFn: func(context.Context, int, string, dh.InventorySaleRequest) (*dh.InventorySaleResponse, error) {
				return nil, &httpx.UpstreamError{StatusCode: 409, Body: `{"code":"item_unavailable"}`}
			},
		}
		_, err := adapter.NewInventoryAdapter(client).RecordInventorySale(context.Background(), inventory.DHSaleRequest{DHInventoryID: 1})
		require.ErrorIs(t, err, inventory.ErrDHItemUnavailable)
	})
}

func TestInventoryAdapter_VoidInventorySale(t *testing.T) {
	t.Run("forwards dh sale id and reason", func(t *testing.T) {
		client := &stubInventoryClient{}
		require.NoError(t, adapter.NewInventoryAdapter(client).VoidInventorySale(context.Background(), "sale_1", "returned"))
		require.Len(t, client.voidSaleCalls, 1)
		require.Equal(t, "sale_1", client.voidSaleCalls[0].dhSaleID)
		require.Equal(t, "returned", client.voidSaleCalls[0].req.Reason)
	})

	t.Run("treats a not-found sale as success, not an error", func(t *testing.T) {
		client := &stubInventoryClient{
			voidSaleFn: func(context.Context, string, dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
				return nil, &httpx.UpstreamError{StatusCode: 404, Body: `{"code":"not_found"}`}
			},
		}
		err := adapter.NewInventoryAdapter(client).VoidInventorySale(context.Background(), "sale_missing", "")
		require.NoError(t, err, "404 covers not-found/foreign/mirror/UI-created deals; none should fail a local un-sell")
	})

	t.Run("surfaces a non-404 error", func(t *testing.T) {
		client := &stubInventoryClient{
			voidSaleFn: func(context.Context, string, dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
				return nil, errors.New("network fail")
			},
		}
		require.Error(t, adapter.NewInventoryAdapter(client).VoidInventorySale(context.Background(), "sale_1", ""))
	})
}
