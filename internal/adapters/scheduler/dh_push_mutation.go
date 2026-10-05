package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

func WithDHPushMutationCoordinator(c *inventory.DHMutationCoordinator, scope inventory.PurchaseMutationScope, g inventory.DHMutationGuards, p inventory.DHSalePurchaseReader) DHPushOption {
	return func(s *DHPushScheduler) {
		s.mutationRequired = true
		s.coordinator = c
		s.scope = scope
		s.guards = g
		s.purchaseReader = p
	}
}
func (s *DHPushScheduler) processCoordinatedPurchase(ctx context.Context, cached inventory.Purchase, cfg inventory.DHPushConfig) processResult {
	if !inventory.DHDependenciesPresent(s.coordinator, s.scope, s.guards, s.purchaseReader, s.psaImporter, s.fieldsUpdater, s.statusUpdater, s.cardIDSaver, s.configLoader) {
		return processSkipped
	}
	outcome := processSkipped
	push, relist := false, false
	var cert string
	err := s.scope.WithPurchaseMutation(ctx, cached.ID, func(c context.Context) error {
		p, e := s.purchaseReader.GetPurchase(c, cached.ID)
		if e != nil {
			return e
		}
		if e := s.guards.AssertMutationAllowed(c, p.ID); e != nil {
			return e
		}
		freshConfig, e := s.configLoader.GetDHPushConfig(c)
		if e != nil {
			return e
		}
		if freshConfig == nil {
			return inventory.NewReturnConflict("coordination_unavailable", "push config required")
		}
		cfg = *freshConfig
		reader, ok := s.guards.(returnStateReader)
		if !ok {
			return inventory.NewReturnConflict("coordination_unavailable", "push state reader required")
		}
		state, e := reader.GetReturnState(c, p.ID)
		if e != nil {
			return e
		}
		if state.Sale != nil {
			return nil
		}
		cert = p.CertNumber
		if p.DHInventoryID != 0 {
			if p.DHUnlistedDetectedAt != nil && s.relister != nil && cert != "" && dhlisting.ResolveListingPriceCents(p) > 0 {
				relist = !cfg.ListingsPaused
				return nil
			}
			if e := s.statusUpdater.UpdatePurchaseDHPushStatus(c, p.ID, "matched"); e != nil {
				return e
			}
			outcome = processMatched
			return nil
		}
		if e := inventory.AssertPSAIntakeIdentity(p); e != nil {
			return e
		}
		if p.CertNumber == "" {
			if s.markUnmatched(c, *p, "purchase has no cert number") {
				outcome = processUnmatched
			}
			return nil
		}
		if hold := dhlisting.EvaluateHoldTriggers(p, cfg); hold != "" {
			outcome = s.setHeld(c, *p, hold)
			return nil
		}
		if !p.IsReceivedOrShipped() || p.DHPushStatus != inventory.DHPushStatusPending {
			return nil
		}
		push = true
		return nil
	})
	if err != nil {
		return processSkipped
	}
	if relist {
		result := s.relister.ListPurchases(ctx, []string{cert})
		if result.Listed > 0 || (result.Total > 0 && result.Synced == result.Total) {
			return processMatched
		}
		return processSkipped
	}
	if !push {
		return outcome
	}
	return s.pushPSACoordinated(ctx, cached.ID, cfg)
}
func pushPayload(p *inventory.Purchase) string {
	b, _ := json.Marshal(buildPSAImportItem(*p))
	return string(b)
}
func (s *DHPushScheduler) pushPSACoordinated(ctx context.Context, id string, cfg inventory.DHPushConfig) processResult {
	err := s.coordinator.Run(ctx, id, func(c context.Context) (inventory.DHMutationRequest, error) {
		p, e := s.purchaseReader.GetPurchase(c, id)
		if e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if e := inventory.AssertPSAIntakeIdentity(p); e != nil {
			return inventory.DHMutationRequest{}, e
		}
		reader, ok := s.guards.(returnStateReader)
		if !ok {
			return inventory.DHMutationRequest{}, inventory.NewReturnConflict("coordination_unavailable", "push state reader required")
		}
		state, e := reader.GetReturnState(c, p.ID)
		if e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if state.Sale != nil {
			return inventory.DHMutationRequest{}, inventory.NewReturnConflict("push_precondition_failed", "purchase has a current sale")
		}
		if e := s.guards.AssertMutationAllowed(c, id); e != nil {
			return inventory.DHMutationRequest{}, e
		}
		freshConfig, e := s.configLoader.GetDHPushConfig(c)
		if e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if freshConfig == nil {
			return inventory.DHMutationRequest{}, inventory.NewReturnConflict("coordination_unavailable", "push config required")
		}
		cfg = *freshConfig
		if p.DHInventoryID != 0 || p.DHPushStatus != "pending" || !p.IsReceivedOrShipped() || dhlisting.EvaluateHoldTriggers(p, cfg) != "" {
			return inventory.DHMutationRequest{}, inventory.NewReturnConflict("push_precondition_failed", "purchase no longer eligible for push")
		}
		return inventory.DHMutationRequest{Kind: "push", Phase: "psa_import", PayloadIdentity: pushPayload(p)}, nil
	}, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		p, e := s.purchaseReader.GetPurchase(c, id)
		if e != nil {
			return nil, e
		}
		if pushPayload(p) != a.PayloadIdentity {
			return nil, inventory.NewReturnConflict("identity_conflict", "prepared PSA import changed")
		}
		if e := s.guards.AssertMutationAllowed(c, id); e != nil {
			return nil, e
		}
		reader, ok := s.guards.(returnStateReader)
		if !ok {
			return nil, inventory.NewReturnConflict("coordination_unavailable", "push state reader required")
		}
		state, e := reader.GetReturnState(c, p.ID)
		if e != nil {
			return nil, e
		}
		if state.Sale != nil {
			return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:current-sale"}, nil
		}
		freshConfig, e := s.configLoader.GetDHPushConfig(c)
		if e != nil {
			return nil, e
		}
		if freshConfig == nil || p.DHInventoryID != 0 || p.DHPushStatus != "pending" || !p.IsReceivedOrShipped() || dhlisting.EvaluateHoldTriggers(p, *freshConfig) != "" {
			return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:push-ineligible"}, nil
		}
		resp, e := s.psaImporter.PSAImport(c, []dh.PSAImportItem{buildPSAImportItem(*p)})
		if e != nil {
			return nil, e
		}
		if resp == nil || !resp.Success || len(resp.Results) != 1 {
			return nil, fmt.Errorf("PSA import did not return one successful target")
		}
		r := resp.Results[0]
		switch r.Resolution {
		case dh.PSAImportStatusMatched, dh.PSAImportStatusUnmatchedCreated, dh.PSAImportStatusOverrideCorrected, dh.PSAImportStatusAlreadyListed:
		default:
			return nil, fmt.Errorf("PSA import unresolved: %s", r.Resolution)
		}
		if r.CertNumber != p.CertNumber || r.DHInventoryID <= 0 || r.DHCardID <= 0 {
			return nil, inventory.NewReturnConflict("identity_conflict", "PSA import receipt target invalid")
		}
		if e := s.fieldsUpdater.UpdatePurchaseDHFields(c, id, inventory.DHFieldsUpdate{CardID: r.DHCardID, InventoryID: r.DHInventoryID, CertStatus: dh.CertStatusMatched, DHStatus: inventory.DHStatusForPush(r.Status)}); e != nil {
			return nil, e
		}
		if e := s.statusUpdater.UpdatePurchaseDHPushStatus(c, id, "matched"); e != nil {
			return nil, e
		}
		if e := s.cardIDSaver.SaveExternalID(c, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, fmt.Sprint(r.DHCardID)); e != nil {
			return nil, e
		}
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: fmt.Sprintf("psa-import:%s:%d", r.Resolution, r.DHInventoryID)}, nil
	})
	if err != nil {
		s.logger.Warn(ctx, "DH push remains fenced", observability.String("purchaseID", id), observability.Err(err))
		return processSkipped
	}
	return processMatchedComplete
}
