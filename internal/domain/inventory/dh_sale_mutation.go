package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type DHSalePurchaseReader interface {
	GetPurchase(context.Context, string) (*Purchase, error)
}
type DHSaleMutationStore interface {
	GetSaleByPurchaseID(context.Context, string) (*Sale, error)
	SetSaleIdempotencyKeyIfAbsent(context.Context, string, string) (string, error)
	SetSaleDHSaleID(context.Context, string, string, time.Time) error
}

var errNoDHSaleDispatch = errors.New("no off-platform DH sale dispatch required")

func persistedDHSaleRequest(sa *Sale, p *Purchase, key string) DHSaleRequest {
	return DHSaleRequest{DHInventoryID: p.DHInventoryID, IdempotencyKey: key, SalePriceCents: sa.SalePriceCents, SoldAt: DeriveDHSoldAt(sa.SaleDate, p.PurchaseDate, sa.CreatedAt)}
}
func salePayload(id string, r DHSaleRequest) string {
	b, _ := json.Marshal(struct {
		SaleID  string
		Request DHSaleRequest
	}{id, r})
	return string(b)
}

// RecordCoordinatedDHSale is shared by inline recording and both reconciler
// passes. Discovery snapshots only supply IDs. Mint+marker commit independently
// of remote execution/handle persistence, so same-key recovery survives rollback.
func RecordCoordinatedDHSale(ctx context.Context, c *DHMutationCoordinator, purchases DHSalePurchaseReader, sales DHSaleMutationStore, remote DHSaleRecorder, purchaseID, saleID string, idGen func() string) error {
	if !DHDependenciesPresent(c, purchases, sales, remote, idGen) {
		return NewReturnConflict("coordination_unavailable", "sale mutation dependencies required")
	}
	load := func(ctx context.Context) (*Purchase, *Sale, error) {
		p, e := purchases.GetPurchase(ctx, purchaseID)
		if e != nil {
			return nil, nil, e
		}
		sa, e := sales.GetSaleByPurchaseID(ctx, purchaseID)
		if e != nil {
			return nil, nil, e
		}
		if sa == nil || sa.ID != saleID {
			return nil, nil, NewReturnConflict("sale_precondition_failed", "current sale changed before DH recording")
		}
		if sa.OrderID != "" || p.DHInventoryID == 0 {
			return nil, nil, errNoDHSaleDispatch
		}
		return p, sa, nil
	}
	err := c.Run(ctx, purchaseID, func(ctx context.Context) (DHMutationRequest, error) {
		p, sa, e := load(ctx)
		if e != nil {
			return DHMutationRequest{}, e
		}
		key := sa.DHIdempotencyKey
		if key == "" {
			key, e = sales.SetSaleIdempotencyKeyIfAbsent(ctx, sa.ID, NewDHIdempotencyKey(idGen))
			if e != nil {
				return DHMutationRequest{}, e
			}
		}
		req := persistedDHSaleRequest(sa, p, key)
		return DHMutationRequest{Kind: "sale", Phase: "record", Key: key, PayloadIdentity: salePayload(sa.ID, req)}, nil
	}, func(ctx context.Context, a *DHMutationAttempt) (*DHMutationSettlement, error) {
		p, sa, e := load(ctx)
		if e != nil {
			return nil, e
		}
		req := persistedDHSaleRequest(sa, p, a.Key)
		if sa.DHIdempotencyKey != a.Key || salePayload(sa.ID, req) != a.PayloadIdentity {
			return nil, NewReturnConflict("identity_conflict", "prepared sale request changed")
		}
		result, e := remote.RecordInventorySale(ctx, req)
		if e != nil {
			return nil, e
		}
		if result == nil || result.DHSaleID == "" || !result.Delisted || (result.Replayed && (result.SoldInventoryID == nil || *result.SoldInventoryID != p.DHInventoryID)) || (result.SoldInventoryID != nil && *result.SoldInventoryID != p.DHInventoryID) {
			return nil, NewReturnConflict("invalid_sale_receipt", "DH sale did not confirm the captured target was delisted")
		}
		if e := sales.SetSaleDHSaleID(ctx, sa.ID, result.DHSaleID, time.Now()); e != nil {
			return nil, e
		}
		return &DHMutationSettlement{Outcome: "succeeded", Receipt: result.DHSaleID}, nil
	})
	if errors.Is(err, errNoDHSaleDispatch) {
		return nil
	}
	return err
}
func WithDHMutationCoordinator(c *DHMutationCoordinator, scope PurchaseMutationScope, repo DHMutationRepository, g DHMutationGuards) ServiceOption {
	return func(s *service) {
		s.mutationRequired = true
		s.mutationCoordinator = c
		s.mutationScope = scope
		s.mutationRepo = repo
		s.mutationGuards = g
	}
}
