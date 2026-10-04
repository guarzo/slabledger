package dhlisting

import (
	"context"
	"encoding/json"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type ReturnStateReader interface {
	GetReturnState(context.Context, string) (*inventory.ConfirmedReturnState, error)
}

// ExplicitListingService must be invoked only by an authenticated operator.
// The operation ID is server-observed, never a provider key or target.
type ExplicitListingService interface {
	ListPurchaseExplicit(context.Context, string, string, string) DHListingResult
}
type explicitListingKey struct{}
type explicitListing struct{ purchaseID, operationID string }

func WithDHListingMutationCoordinator(c *inventory.DHMutationCoordinator, g inventory.DHMutationGuards, r ReturnStateReader) DHListingServiceOption {
	return func(s *dhListingService) {
		s.mutationRequired = true
		s.coordinator = c
		s.guards = g
		s.returnStates = r
	}
}
func (s *dhListingService) ListPurchaseExplicit(ctx context.Context, id, cert, operation string) DHListingResult {
	return s.ListPurchases(context.WithValue(ctx, explicitListingKey{}, explicitListing{id, operation}), []string{cert})
}
func (s *dhListingService) freshListingPurchase(ctx context.Context, p *inventory.Purchase) (*inventory.Purchase, error) {
	state, err := s.returnStates.GetReturnState(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if state.Purchase == nil || state.Purchase.CertNumber != p.CertNumber {
		return nil, inventory.NewReturnConflict("identity_conflict", "listing purchase identity changed")
	}
	if state.Sale != nil {
		return nil, inventory.NewReturnConflict("current_sale_present", "sold inventory cannot be listed")
	}
	return state.Purchase, nil
}

// Capture actual immutable request inputs, not unrelated purchase timestamps
// or analytics fields that background enrichment may update between scopes.
func listingPayload(p *inventory.Purchase) string {
	var intake *DHPSAImportItem
	if p.DHInventoryID == 0 && p.DHPushStatus == inventory.DHPushStatusPending {
		intake = &DHPSAImportItem{CertNumber: p.CertNumber, CostBasisCents: p.BuyCostCents, CardName: p.CardName, SetName: p.SetName, CardNumber: p.CardNumber, Year: p.CardYear, Language: InferDHLanguage(p.SetName, p.CardName)}
	}
	price := ResolveListingPriceCents(p)
	b, _ := json.Marshal(struct {
		InventoryID   int
		Intake        *DHPSAImportItem
		Patch         inventory.DHInventoryStatusUpdate
		Channels      []string
		AlreadyInSync bool
	}{p.DHInventoryID, intake, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed, ListingPriceCents: price, CertImageURLFront: p.FrontImageURL, CertImageURLBack: p.BackImageURL}, DefaultListingChannels, p.DHStatus == inventory.DHStatusListed && p.DHListingPriceCents == price && p.DHChannelsJSON != ""})
	return string(b)
}
func (s *dhListingService) listCoordinated(ctx context.Context, cached *inventory.Purchase) (listOutcome, error) {
	if !s.mutationRequired {
		return s.listOnePurchase(ctx, cached)
	}
	if !inventory.DHDependenciesPresent(s.coordinator, s.guards, s.returnStates, s.fieldsUpdater, s.configLoader, s.lister) {
		return outcomeSkipped, inventory.NewReturnConflict("coordination_unavailable", "listing coordination is required")
	}
	outcome := outcomeSkipped
	err := s.coordinator.Run(ctx, cached.ID, func(c context.Context) (inventory.DHMutationRequest, error) {
		p, e := s.freshListingPurchase(c, cached)
		if e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if p.DHInventoryID == 0 {
			if e := inventory.AssertPSAIntakeIdentity(p); e != nil {
				return inventory.DHMutationRequest{}, e
			}
		}
		if p.DHInventoryID == 0 && !inventory.DHDependenciesPresent(s.psaImporter, s.cardIDSaver, s.pushStatusUpdater) {
			return inventory.DHMutationRequest{}, inventory.NewReturnConflict("coordination_unavailable", "inline intake collaborators required")
		}
		if explicit, ok := ctx.Value(explicitListingKey{}).(explicitListing); ok && explicit.purchaseID == p.ID {
			if p.ReceivedAt == nil || ResolveListingPriceCents(p) <= 0 {
				return inventory.DHMutationRequest{}, inventory.NewReturnConflict("invalid_listing_precondition", "received status and positive committed price are required")
			}
			if s.configLoader != nil {
				cfg, e := s.configLoader.GetDHPushConfig(c)
				if e != nil {
					return inventory.DHMutationRequest{}, e
				}
				if cfg != nil && cfg.ListingsPaused {
					return inventory.DHMutationRequest{}, inventory.NewReturnConflict("listings_paused", "DH listings are paused")
				}
			}
			if explicit.operationID != "" {
				if e := s.guards.AuthorizeReturnedListing(c, p.ID, explicit.operationID); e != nil {
					return inventory.DHMutationRequest{}, e
				}
			}
		}
		// Guard AFTER authorization, still in the preparation transaction.
		if e := s.guards.AssertMutationAllowed(c, p.ID); e != nil {
			return inventory.DHMutationRequest{}, e
		}
		return inventory.DHMutationRequest{Kind: "list", Phase: "intake_patch_sync", PayloadIdentity: listingPayload(p)}, nil
	}, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		p, e := s.freshListingPurchase(c, cached)
		if e != nil {
			return nil, e
		}
		if listingPayload(p) != a.PayloadIdentity {
			return nil, inventory.NewReturnConflict("identity_conflict", "prepared listing request changed")
		}
		if e := s.guards.AssertMutationAllowed(c, p.ID); e != nil {
			return nil, e
		}
		// These noops occur before any dispatch and cannot clear an older marker:
		// Run already refuses every open unkeyed request during preparation.
		if !p.IsReceivedOrShipped() || ResolveListingPriceCents(p) <= 0 || (p.DHInventoryID == 0 && (p.DHPushStatus != inventory.DHPushStatusPending || s.psaImporter == nil)) {
			return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:listing-ineligible"}, nil
		}
		outcome, e = s.listOnePurchase(c, p)
		if e != nil {
			return nil, e
		}
		if outcome != outcomeListed && outcome != outcomeAlreadyInSync {
			return nil, inventory.NewReturnConflict("terminal_receipt_required", "inline intake/listing did not complete")
		}
		receipt := "patch-and-channel-sync-success"
		if outcome == outcomeAlreadyInSync {
			receipt = "not-dispatched:already-in-sync"
		}
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: receipt}, nil
	})
	if err != nil {
		return outcomeSkipped, err
	}
	return outcome, nil
}
