package dhlisting

import (
	"context"
	"errors"
	"fmt"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// WithMutationReceipts enables target/status validation and one-call mutation
// dispatch for coordinated callers without changing legacy rotation behavior.
func (a *InventoryAdapter) WithMutationReceipts() *InventoryAdapter {
	a.mutationReceipts = true
	return a
}
func (a *InventoryAdapter) VoidInventorySaleVerified(ctx context.Context, handle string, target int, reason string) error {
	receipt, err := a.client.VoidInventorySale(ctx, handle, dh.VoidSaleRequest{Reason: reason})
	if err != nil {
		return classifyDHSaleError(err)
	}
	if receipt == nil {
		return fmt.Errorf("DH void receipt missing")
	}
	for _, item := range receipt.Items {
		if item.DHInventoryID == target && item.Status == inventory.DHStatusInStock {
			return nil
		}
	}
	return fmt.Errorf("DH void did not verify captured inventory target %d", target)
}
func classifyInitialPatchRejection(err error) error {
	var upstream *httpx.UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != 422 || upstream.Message != "Cannot update item with status 'sold'" {
		return err
	}
	// Source-verified InventoryUpsertService initial sold guard. This is NOT
	// a general 422 classifier and must not clear any prior successful phase.
	rejection := inventory.NewDHNonMutationError("dh_patch_sold_guard", upstream.Message, err)
	var request *httpx.RequestError
	if errors.As(err, &request) {
		rejection.Uncertain = request.Uncertain
	}
	return rejection
}
