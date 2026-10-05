package dh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// InventoryReturnResponse is a receipt for the original returned sale. ItemStatus
// is CURRENT state on replay, not proof that local completion is still safe.
type InventoryReturnResponse struct {
	DHInventoryID  int    `json:"dh_inventory_id"`
	ItemStatus     string `json:"item_status"`
	ExternalSaleID int64  `json:"external_sale_id"`
	Restored       bool   `json:"restored"`
}

func (r *InventoryReturnResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		DHInventoryID  *int    `json:"dh_inventory_id"`
		ItemStatus     *string `json:"item_status"`
		ExternalSaleID *int64  `json:"external_sale_id"`
		Restored       *bool   `json:"restored"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.DHInventoryID == nil || *wire.DHInventoryID <= 0 ||
		wire.ExternalSaleID == nil || *wire.ExternalSaleID <= 0 ||
		wire.ItemStatus == nil || strings.TrimSpace(*wire.ItemStatus) == "" ||
		wire.Restored == nil {
		return fmt.Errorf("DH return response requires positive inventory/sale IDs, item_status and a restored boolean")
	}
	*r = InventoryReturnResponse{
		DHInventoryID: *wire.DHInventoryID, ItemStatus: *wire.ItemStatus,
		ExternalSaleID: *wire.ExternalSaleID, Restored: *wire.Restored,
	}
	return nil
}

// ReturnInventoryToStock requires the caller's already-persisted key. It never
// generates or normalizes a key, and always sends literal physical confirmation.
// Only receipt-backed keyed retries retain the client's default retry policy.
func (c *Client) ReturnInventoryToStock(ctx context.Context, inventoryID int, key string) (*InventoryReturnResponse, error) {
	if inventoryID <= 0 || !validReturnKey(key) {
		return nil, apperrors.ProviderInvalidRequest(providerName,
			fmt.Errorf("DH return requires a positive inventory ID and an unchanged nonempty ASCII key of at most 255 bytes"))
	}
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/return-to-stock", c.baseURL, inventoryID)
	body := struct {
		ReturnConfirmed bool `json:"return_confirmed"`
	}{ReturnConfirmed: true}
	var resp InventoryReturnResponse
	if err := c.doEnterprise(ctx, "POST", fullURL, body, &resp, map[string]string{"Idempotency-Key": key}); err != nil {
		return nil, err
	}
	if resp.DHInventoryID != inventoryID {
		return nil, &httpx.RequestError{Phase: httpx.PhaseResponse, Uncertain: true,
			Err: apperrors.ProviderInvalidResponse(providerName,
				fmt.Errorf("DH return inventory ID %d does not match requested ID %d", resp.DHInventoryID, inventoryID))}
	}
	return &resp, nil
}

func validReturnKey(key string) bool {
	if key == "" || len(key) > 255 || strings.TrimSpace(key) != key {
		return false
	}
	for _, b := range []byte(key) {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}
