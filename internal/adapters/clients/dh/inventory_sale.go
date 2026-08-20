package dh

import (
	"context"
	"fmt"
)

// InventorySaleRequest is the body for POST .../inventory/:id/sale.
type InventorySaleRequest struct {
	SalePriceCents   int    `json:"sale_price_cents"`
	Quantity         int    `json:"quantity,omitempty"`
	SoldAt           string `json:"sold_at,omitempty"`
	CounterpartyName string `json:"counterparty_name,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

// InventorySaleResponse is DH's response to a recorded sale. ItemStatus is
// open-ended by contract, never an enum; SoldInventoryID is nullable and, at
// our qty-1 scope, is read but never assumed to equal the addressed id.
type InventorySaleResponse struct {
	SaleID              string `json:"sale_id"`
	DHInventoryID       int    `json:"dh_inventory_id"`
	SoldInventoryID     *int   `json:"sold_inventory_id"`
	ItemStatus          string `json:"item_status"`
	Delisted            bool   `json:"delisted"`
	Replayed            bool   `json:"replayed"`
	RealizedProfitCents int    `json:"realized_profit_cents"`
}

// VoidSaleRequest is the body for POST .../sales/:sale_id/void.
type VoidSaleRequest struct {
	Reason string `json:"reason,omitempty"`
}

// VoidSaleResponse is DH's response to a void. Reversed is false, not an
// error, when the sale was already void.
type VoidSaleResponse struct {
	Reversed bool                `json:"reversed"`
	Items    []InventoryListItem `json:"items"`
}

// RecordInventorySale POSTs .../inventory/:id/sale with the required
// Idempotency-Key header. Quantity is always forced to 1: every SlabLedger
// purchase is a single graded slab (design doc, "Contract hazards"), so
// partial-sale semantics never apply here.
func (c *Client) RecordInventorySale(ctx context.Context, inventoryID int, idempotencyKey string, req InventorySaleRequest) (*InventorySaleResponse, error) {
	req.Quantity = 1
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/sale", c.baseURL, inventoryID)

	var resp InventorySaleResponse
	extraHeaders := map[string]string{"Idempotency-Key": idempotencyKey}
	if err := c.doEnterprise(ctx, "POST", fullURL, req, &resp, extraHeaders); err != nil {
		return nil, err
	}
	return &resp, nil
}

// VoidInventorySale POSTs .../sales/:sale_id/void.
func (c *Client) VoidInventorySale(ctx context.Context, dhSaleID string, req VoidSaleRequest) (*VoidSaleResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/sales/%s/void", c.baseURL, dhSaleID)

	var resp VoidSaleResponse
	if err := c.postEnterprise(ctx, fullURL, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
