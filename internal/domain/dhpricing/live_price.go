package dhpricing

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// LivePricePurchaseLookup finds unresolved operations separately from current
// drift: a historical receipt can still need settlement after a sale or delist.
type LivePricePurchaseLookup interface {
	PurchaseLookup
	ListPendingDHLivePrices(context.Context) ([]string, error)
}

type livePriceService struct {
	lookup      LivePricePurchaseLookup
	remote      inventory.DHLivePriceUpdater
	writer      DHPriceWriter
	coordinator *inventory.DHMutationCoordinator
	guards      inventory.DHMutationGuards
	states      ReturnStateReader
	logger      observability.Logger
}

// NewLivePriceService has no legacy PATCH fallback. Runtime construction is
// gated until the provider's receipt-backed endpoint has been deployed.
func NewLivePriceService(l LivePricePurchaseLookup, u inventory.DHLivePriceUpdater, w DHPriceWriter, c *inventory.DHMutationCoordinator, g inventory.DHMutationGuards, r ReturnStateReader, logger observability.Logger) Service {
	return &livePriceService{lookup: l, remote: u, writer: w, coordinator: c, guards: g, states: r, logger: logger}
}

type livePricePayload struct {
	ID    int
	Price int
}

func decodeLivePrice(a *inventory.DHMutationAttempt) (livePricePayload, error) {
	var p livePricePayload
	if a == nil || a.Kind != "price" || a.Phase != "live_price" || a.Key == "" || a.ReturnOperationID != "" {
		return p, inventory.NewReturnConflict("preceding_dh_mutation_uncertain", "only a keyed live-price attempt can replay")
	}
	if err := json.Unmarshal([]byte(a.PayloadIdentity), &p); err != nil {
		return p, inventory.NewReturnConflict("identity_conflict", "invalid persisted live-price payload")
	}
	canonical, _ := json.Marshal(p)
	if string(canonical) != a.PayloadIdentity || p.ID != a.DHInventoryID || p.ID <= 0 || p.Price <= 0 || int64(p.Price) > 9999999999 {
		return p, inventory.NewReturnConflict("identity_conflict", "persisted live-price target or cents changed")
	}
	return p, nil
}

func (s *livePriceService) state(ctx context.Context, id string) (*inventory.ConfirmedReturnState, error) {
	state, err := s.states.GetReturnState(ctx, id)
	if err != nil {
		return nil, err
	}
	if state == nil || state.Purchase == nil || state.Purchase.ID != id {
		return nil, inventory.NewReturnConflict("identity_conflict", "live-price purchase identity changed")
	}
	return state, nil
}

func livePriceSkip(p *inventory.Purchase) Outcome {
	if p.DHInventoryID == 0 {
		return OutcomeSkippedNoInventory
	}
	if p.DHStatus == inventory.DHStatusInStock {
		return OutcomeSkippedUnlisted
	}
	if p.DHStatus != inventory.DHStatusListed {
		return OutcomeSkippedBadStatus
	}
	desired := inventory.ResolveListingPriceCents(p)
	if desired <= 0 {
		return OutcomeSkippedZeroReviewed
	}
	if desired == p.DHListingPriceCents {
		return OutcomeSkippedNoDrift
	}
	return ""
}

func (s *livePriceService) SyncPurchasePrice(ctx context.Context, id string) SyncResult {
	result := SyncResult{PurchaseID: id, Outcome: OutcomeError}
	if !inventory.DHDependenciesPresent(s.lookup, s.remote, s.writer, s.coordinator, s.guards, s.states, s.logger) {
		result.Err = inventory.NewReturnConflict("coordination_unavailable", "live-price dependencies required")
		return result
	}
	err := s.coordinator.Run(ctx, id, func(c context.Context) (inventory.DHMutationRequest, error) {
		state, e := s.state(c, id)
		if e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if a := state.PrecedingAttempt; a != nil {
			if _, e := decodeLivePrice(a); e != nil {
				return inventory.DHMutationRequest{}, e
			}
			if a.RetryNotBefore != nil && time.Now().Before(*a.RetryNotBefore) {
				return inventory.DHMutationRequest{}, &inventory.DHMutationRetryError{NotBefore: *a.RetryNotBefore, Cause: errors.New("live-price retry cooldown active")}
			}
			return inventory.DHMutationRequest{Kind: a.Kind, Phase: a.Phase, Key: a.Key, PayloadIdentity: a.PayloadIdentity}, nil
		}
		if state.Sale != nil {
			return inventory.DHMutationRequest{}, inventory.NewReturnConflict("current_sale_present", "sold inventory cannot be repriced")
		}
		if e := s.guards.AssertMutationAllowed(c, id); e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if skip := livePriceSkip(state.Purchase); skip != "" {
			result.Outcome = skip
			return inventory.DHMutationRequest{}, errPriceNotListed
		}
		price := inventory.ResolveListingPriceCents(state.Purchase)
		if int64(price) > 9999999999 {
			return inventory.DHMutationRequest{}, inventory.NewReturnConflict("invalid_live_price", "committed price exceeds DH live-price maximum")
		}
		payload, _ := json.Marshal(livePricePayload{ID: state.Purchase.DHInventoryID, Price: price})
		return inventory.DHMutationRequest{Kind: "price", Phase: "live_price", Key: "dh-live-price-" + rand.Text(), PayloadIdentity: string(payload)}, nil
	}, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		frozen, e := decodeLivePrice(a)
		if e != nil {
			return nil, e
		}
		state, e := s.state(c, id)
		if e != nil {
			return nil, e
		}
		// Another prepared caller can persist a cooldown before we acquire
		// execution ownership; the preparation snapshot is not authoritative.
		pending := state.PrecedingAttempt
		if pending == nil || pending.ID != a.ID {
			return nil, inventory.NewReturnConflict("identity_conflict", "owned live-price attempt changed")
		}
		if pending.RetryNotBefore != nil && time.Now().Before(*pending.RetryNotBefore) {
			return nil, &inventory.DHMutationRetryError{NotBefore: *pending.RetryNotBefore, Cause: errors.New("live-price retry cooldown active")}
		}
		// A new, never-dispatched intent can reject changed lifecycle eligibility.
		// An older keyed intent must replay its frozen request to obtain its receipt.
		if !a.Replaying && (state.Sale != nil || state.Purchase.DHStatus != inventory.DHStatusListed) {
			result.Err = inventory.NewReturnConflict("live_price_ineligible", "purchase no longer eligible before dispatch")
			return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:live-price-ineligible"}, nil
		}
		receipt, e := s.remote.UpdateLivePrice(c, frozen.ID, a.Key, frozen.Price)
		if e != nil {
			var retry *inventory.DHMutationRetryError
			if errors.As(e, &retry) {
				// This price-only call made no local writes. Commit its cooldown
				// before releasing ownership; rollback-then-persist admits racers.
				result.Err = e
				return &inventory.DHMutationSettlement{RetryNotBefore: &retry.NotBefore}, nil
			}
			var rejection *inventory.DHLivePriceRejection
			if errors.As(e, &rejection) {
				// These endpoint-specific refusals are serialized behind prior same-key
				// execution and receipt recheck. Unlike generic errors, they resolve replay.
				result.Err = e
				return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "live-price-rejected:" + rejection.Code}, nil
			}
			return nil, e
		}
		if receipt == nil || receipt.DHInventoryID != frozen.ID || receipt.ListingPriceCents != frozen.Price {
			return nil, inventory.NewReturnConflict("invalid_live_price_receipt", "receipt does not match captured target and cents")
		}
		p := state.Purchase
		result.OldListingCents = p.DHListingPriceCents
		result.NewListingCents = frozen.Price
		if state.Sale == nil && p.DHStatus == inventory.DHStatusListed {
			if e := s.writer.UpdatePurchaseDHPriceSync(c, id, frozen.Price, time.Now()); e != nil {
				return nil, e
			}
		}
		result.Outcome = OutcomeSynced
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: fmt.Sprintf("live-price:%d:%d", frozen.ID, frozen.Price)}, nil
	})
	if errors.Is(err, errPriceNotListed) {
		return result
	}
	if err != nil {
		result.Outcome = OutcomeError
		result.Err = errors.Join(result.Err, err)
	}
	if result.Err != nil {
		s.logger.Warn(ctx, "DH live-price operation remains retryable or was refused", observability.String("purchaseID", id), observability.Err(result.Err))
	}
	return result
}

func (s *livePriceService) SyncDriftedPurchases(ctx context.Context) SyncBatchResult {
	result := SyncBatchResult{ByOutcome: map[Outcome]int{}}
	if !inventory.DHDependenciesPresent(s.lookup) {
		result.ListErr = inventory.NewReturnConflict("coordination_unavailable", "live-price lookup required")
		return result
	}
	pending, err := s.lookup.ListPendingDHLivePrices(ctx)
	if err != nil {
		s.logger.Warn(ctx, "DH live-price discovery failed", observability.String("discoveryPhase", "pending"), observability.Err(err))
		result.ListErr = err
		return result
	}
	seen := map[string]bool{}
	sync := func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		r := s.SyncPurchasePrice(ctx, id)
		result.Total++
		result.ByOutcome[r.Outcome]++
	}
	for _, id := range pending {
		sync(id)
	}
	drift, err := s.lookup.ListDHPriceDrift(ctx)
	if err != nil {
		s.logger.Warn(ctx, "DH live-price discovery failed", observability.String("discoveryPhase", "drift"), observability.Err(err))
		result.ListErr = err
		return result
	}
	for _, p := range drift {
		sync(p.ID)
	}
	return result
}
