package csvimport

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func (s *service) markImportedSaleSold(ctx context.Context, p *inventory.Purchase, sa *inventory.Sale) error {
	if s.mutationGuards == nil {
		return s.purchases.UpdatePurchaseDHStatus(ctx, p.ID, "sold")
	}
	at, e := s.mutationGuards.ObservationTime(ctx)
	if e != nil {
		return e
	}
	_, e = s.mutationGuards.ApplyDHSoldObservation(ctx, p.ID, sa.ID, p.DHInventoryID, at, func(c context.Context) error { return s.purchases.UpdatePurchaseDHStatus(c, p.ID, "sold") })
	return e
}
