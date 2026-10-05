package dhlisting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Optional capability keeps the existing sale/list constructor compatible with
// narrower clients. A missing return dependency fails closed, never falls back
// to an ordinary void, deletion, push or inventory status PATCH.
type inventoryReturnClient interface {
	ReturnInventoryToStock(context.Context, int, string) (*dh.InventoryReturnResponse, error)
	ListInventory(context.Context, dh.InventoryFilters) (*dh.InventoryListResponse, error)
}

func (a *InventoryAdapter) ReturnInventoryToStock(ctx context.Context, inventoryID int, key string) (*inventory.DHReturnResult, error) {
	client, ok := a.client.(inventoryReturnClient)
	if !ok {
		return nil, fmt.Errorf("DH return client is unavailable")
	}
	resp, err := client.ReturnInventoryToStock(ctx, inventoryID, key)
	if err != nil {
		return nil, classifyDHReturnError(err)
	}
	return &inventory.DHReturnResult{
		DHInventoryID: resp.DHInventoryID, ItemStatus: resp.ItemStatus,
		ExternalSaleID: resp.ExternalSaleID, Restored: resp.Restored,
	}, nil
}

func classifyDHReturnError(err error) error {
	var upstream *httpx.UpstreamError
	if !errors.As(err, &upstream) {
		return err
	}
	var failure *httpx.RequestError
	uncertain := upstream.StatusCode >= 500
	if errors.As(err, &failure) {
		uncertain = uncertain || failure.Uncertain
	}
	return &inventory.DHReturnError{
		Code: upstream.Code, Message: upstream.Message, Status: upstream.StatusCode,
		Uncertain: uncertain, Cause: err,
	}
}

// GetReturnInventoryStatus is ONLY an initial preflight. GET cannot acknowledge
// completion of an earlier dispatched request; pending recovery must replay its
// persisted key. Match inventory AND cert, never just the first filtered row.
func (a *InventoryAdapter) GetReturnInventoryStatus(ctx context.Context, inventoryID int, certNumber string) (string, error) {
	if inventoryID <= 0 || strings.TrimSpace(certNumber) == "" {
		return "", fmt.Errorf("DH return preflight requires a positive inventory ID and cert number")
	}
	client, ok := a.client.(inventoryReturnClient)
	if !ok {
		return "", fmt.Errorf("DH return client is unavailable")
	}
	for page := 1; page <= maxSnapshotPages; page++ {
		resp, err := client.ListInventory(ctx, dh.InventoryFilters{
			CertNumber: certNumber, Page: page, PerPage: snapshotPageSize,
		})
		if err != nil {
			return "", err
		}
		status := ""
		for _, item := range resp.Items {
			if item.DHInventoryID != inventoryID {
				continue
			}
			if status != "" || item.CertNumber != certNumber || strings.TrimSpace(item.Status) == "" {
				return "", fmt.Errorf("DH return preflight identity/status conflict for inventory %d", inventoryID)
			}
			status = item.Status
		}
		if status != "" {
			return status, nil
		}
		if len(resp.Items) < snapshotPageSize {
			return "", fmt.Errorf("DH return preflight inventory %d with exact cert was not found", inventoryID)
		}
	}
	return "", fmt.Errorf("DH return preflight exceeded max pages (%d)", maxSnapshotPages)
}

var _ inventory.DHReturner = (*InventoryAdapter)(nil)
