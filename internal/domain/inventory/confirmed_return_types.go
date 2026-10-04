package inventory

import (
	"context"
	"errors"
	"fmt"
)

// DHReturner is the wire-neutral port for a confirmed external return and its
// initial exact-target preflight. Callers persist key before dispatch; pending
// recovery replays that key rather than short-circuiting on a status read.
type DHReturner interface {
	ReturnInventoryToStock(ctx context.Context, inventoryID int, key string) (*DHReturnResult, error)
	GetReturnInventoryStatus(ctx context.Context, inventoryID int, certNumber string) (string, error)
}

// DHReturnResult carries original sale attribution and CURRENT inventory state.
// Only the coordinating service may validate in_stock and agreement with any
// captured ext-N identity before completion. Parsing a receipt is not consent
// to clear a sale, restore local state or list the slab.
type DHReturnResult struct {
	DHInventoryID  int
	ItemStatus     string
	ExternalSaleID int64
	Restored       bool
}

// DHReturnError preserves an upstream rejection without translating it into a
// generic pending-push error. Uncertain retains preceding failed-attempt evidence
// so a later rejection cannot settle an earlier possibly dispatched mutation.
// Cause preserves diagnostic wrapping without depending on adapter types.
type DHReturnError struct {
	Code      string
	Message   string
	Status    int
	Uncertain bool
	Cause     error
}

func (e *DHReturnError) Error() string {
	return fmt.Sprintf("DH return HTTP %d (%s): %s", e.Status, e.Code, e.Message)
}

func (e *DHReturnError) Unwrap() error { return e.Cause }

// IsDefinitiveDHReturnRejection recognizes ONLY source-verified return endpoint
// guards. It does not authorize retry, clear an incompatible historical attempt,
// or treat every 4xx as safe. Unknown responses, transport failures and all 5xx
// remain uncertain, including lock contention (conservatively).
func IsDefinitiveDHReturnRejection(err error) bool {
	var rejection *DHReturnError
	if !errors.As(err, &rejection) || rejection.Uncertain {
		return false
	}
	switch rejection.Status {
	case 400:
		return rejection.Code == "return_confirmation_required"
	case 422:
		return rejection.Code == "invalid_idempotency_key"
	case 404:
		return rejection.Code == "inventory_not_found"
	case 409:
		switch rejection.Code {
		case "not_sold", "sale_attribution_missing", "sale_attribution_ambiguous",
			"sale_not_active", "unsafe_listing_state", "reversal_would_collide", "idempotency_conflict":
			return true
		}
	}
	return false
}
