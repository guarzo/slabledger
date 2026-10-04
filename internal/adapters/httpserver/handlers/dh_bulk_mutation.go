package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"strconv"
)

func bulkPayload(p *inventory.Purchase, mapped string) string {
	b, _ := json.Marshal(struct {
		Purchase *inventory.Purchase
		Mapped   string
		Phases   []string
	}{p, mapped, []string{"resolve", "mapping", "push", "tracking"}})
	return string(b)
}
func (h *DHHandler) runCoordinatedBulkMatch(ctx context.Context, purchases []inventory.Purchase) {
	if !inventory.DHDependenciesPresent(h.mutationCoordinator, h.mutationGuards, h.purchaseLister, h.cardIDSaver, h.inventoryPusher, h.dhFieldsUpdater, h.pushStatusUpdater, h.candidatesSaver) {
		h.bulkMatchError.Store("DH mutation coordination unavailable")
		return
	}
	matched, failed := 0, 0
	for _, cached := range purchases {
		if ctx.Err() != nil {
			break
		}
		// Per-target transactions replace the old untracked batch snapshot. Batching
		// across independently locked targets would require a different core port.
		mappedID := ""
		matchedThis := false
		eligible := false
		err := h.mutationCoordinator.Run(ctx, cached.ID, func(c context.Context) (inventory.DHMutationRequest, error) {
			p, e := h.purchaseLister.GetPurchase(c, cached.ID)
			if e != nil {
				return inventory.DHMutationRequest{}, e
			}
			if e := h.mutationGuards.AssertMutationAllowed(c, p.ID); e != nil {
				return inventory.DHMutationRequest{}, e
			}
			reader, ok := h.mutationGuards.(dhlisting.ReturnStateReader)
			if !ok {
				return inventory.DHMutationRequest{}, inventory.NewReturnConflict("coordination_unavailable", "bulk state reader required")
			}
			state, e := reader.GetReturnState(c, p.ID)
			if e != nil {
				return inventory.DHMutationRequest{}, e
			}
			eligible = state.Sale == nil && p.CertNumber != "" && p.ReceivedAt != nil && p.DHPushStatus != "matched" && p.DHPushStatus != "manual" && p.DHPushStatus != "held" && p.DHPushStatus != "dismissed"
			if e := inventory.AssertPSAIntakeIdentity(p); e != nil {
				return inventory.DHMutationRequest{}, e
			}
			mappedID, e = h.cardIDSaver.GetExternalID(c, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH)
			if e != nil {
				return inventory.DHMutationRequest{}, e
			}
			if eligible && mappedID == "" && !inventory.DHDependenciesPresent(h.certResolver) {
				return inventory.DHMutationRequest{}, inventory.NewReturnConflict("coordination_unavailable", "bulk cert resolver required")
			}
			return inventory.DHMutationRequest{Kind: "link", Phase: "bulk_resolve_push", PayloadIdentity: bulkPayload(p, mappedID)}, nil
		}, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
			p, e := h.purchaseLister.GetPurchase(c, cached.ID)
			if e != nil {
				return nil, e
			}
			currentMapped, e := h.cardIDSaver.GetExternalID(c, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH)
			if e != nil {
				return nil, e
			}
			if currentMapped != mappedID {
				return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:mapping-changed"}, nil
			}
			if bulkPayload(p, mappedID) != a.PayloadIdentity {
				return nil, inventory.NewReturnConflict("identity_conflict", "bulk match request changed")
			}
			if e := h.mutationGuards.AssertMutationAllowed(c, p.ID); e != nil {
				return nil, e
			}
			reader := h.mutationGuards.(dhlisting.ReturnStateReader)
			state, e := reader.GetReturnState(c, p.ID)
			if e != nil {
				return nil, e
			}
			if !eligible || state.Sale != nil || p.CertNumber == "" || p.ReceivedAt == nil || p.DHPushStatus == "matched" || p.DHPushStatus == "manual" || p.DHPushStatus == "held" || p.DHPushStatus == "dismissed" {
				return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:bulk-ineligible"}, nil
			}
			card, _ := strconv.Atoi(mappedID)
			if card <= 0 {
				name, variant := dhlisting.CleanCardNameForDH(p.CardName)
				resp, e := h.certResolver.ResolveCert(c, dh.CertResolveRequest{CertNumber: p.CertNumber, GemRateID: p.GemRateID, CardName: name, SetName: p.SetName, CardNumber: p.CardNumber, Year: p.CardYear, Variant: variant})
				if e != nil {
					return nil, e
				}
				if resp == nil {
					return nil, fmt.Errorf("empty cert resolution")
				}
				switch resp.Status {
				case dh.CertStatusMatched:
					card = resp.DHCardID
				case dh.CertStatusAmbiguous:
					card, e = dh.ResolveAmbiguous(resp.Candidates, p.CardNumber, nil)
					if e != nil {
						return nil, e
					}
				}
			}
			if card <= 0 {
				if h.pushStatusUpdater != nil {
					if e := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(c, p.ID, "unmatched"); e != nil {
						return nil, e
					}
				}
				return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:catalog-unmatched"}, nil
			}
			matchedThis = true
			if e := h.cardIDSaver.SaveExternalID(c, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, strconv.Itoa(card)); e != nil {
				return nil, e
			}
			price := dhlisting.ResolveListingPriceCents(p)
			receipt := "not-dispatched:catalog-match-only"
			if p.DHInventoryID == 0 && price > 0 && p.BuyCostCents > 0 && h.inventoryPusher != nil {
				target, e := h.pushAndPersistDH(c, p, card, price)
				if e != nil {
					return nil, e
				}
				receipt = fmt.Sprintf("bulk-push:%d", target)
			}
			if h.pushStatusUpdater != nil {
				if e := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(c, p.ID, "matched"); e != nil {
					return nil, e
				}
			}
			return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: receipt}, nil
		})
		if err != nil {
			failed++
			h.logger.Warn(ctx, "bulk match remains fenced", observability.String("purchaseID", cached.ID), observability.Err(err))
		} else if matchedThis {
			matched++
		}
	}
	h.bulkMatchMatched.Store(int64(matched))
	h.bulkMatchFailed.Store(int64(failed))
}
