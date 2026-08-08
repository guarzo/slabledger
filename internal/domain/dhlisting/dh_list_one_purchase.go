package dhlisting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// listOutcome is the per-purchase result of listOnePurchase. Each value maps
// to a fixed set of DHListingResult counter increments, applied by the caller
// so the counting rules live in one place instead of being scattered through
// the state machine as ++/-- pairs.
type listOutcome int

const (
	// outcomeSkipped: the purchase never reached DH, or its list attempt
	// failed and left no lasting DH-side change. Counts as Skipped.
	outcomeSkipped listOutcome = iota
	// outcomeAlreadyInSync: status, price, and channels were already in the
	// target state, so the DH call was elided. Counts as Synced.
	outcomeAlreadyInSync
	// outcomeListed: full success — status updated and channels synced.
	// Counts as both Listed and Synced.
	outcomeListed
	// outcomeSyncedNotPersisted: DH accepted both the list transition and the
	// channel sync, but persisting the result locally failed. Counts as both
	// Synced and Skipped — the DH-side change is real, the local record is
	// not. This preserves the pre-extraction counting, where synced++ ran
	// before the persist attempt and only listed was rolled back.
	outcomeSyncedNotPersisted
	// outcomeAborted: a batch-fatal failure (PSA key exhaustion). The caller
	// stops iterating and returns immediately.
	outcomeAborted
)

// listOnePurchase runs the full list state machine for a single purchase:
// eligibility gating, inline match+push, no-op detection, the DH status
// transition, channel sync with revert-on-failure, and local persistence plus
// event recording.
//
// The returned error is the per-cert failure reason recorded in
// DHListingResult.FailedCerts, except for outcomeAborted, where it is the
// batch-fatal error. A nil error with outcomeSkipped means the purchase was
// skipped without a recorded reason (no psa_importer configured).
//
// p may be mutated: a successful inline push writes back the new DHInventoryID.
func (s *dhListingService) listOnePurchase(ctx context.Context, p *inventory.Purchase) (listOutcome, error) {
	// Hard gate: never put an item live on DH unless it has at least left
	// PSA (received in hand, or PSA-shipped). The push scheduler enforces
	// this via GetPurchasesByDHPushStatus, but the inline/manual/auto-list
	// paths reach ListPurchases directly and previously bypassed it — a
	// price review on a not-yet-received card would inline-push and list it.
	if !p.IsReceivedOrShipped() {
		s.logger.Warn(ctx, "dh listing: purchase not received or shipped; refusing to list",
			observability.String("cert", p.CertNumber),
			observability.String("purchaseID", p.ID))
		return outcomeSkipped, errors.New("not received or shipped by PSA; cannot list on DH")
	}

	// If pending DH push, do inline match + push first.
	if p.DHInventoryID == 0 && p.DHPushStatus == inventory.DHPushStatusPending {
		if s.psaImporter == nil {
			return outcomeSkipped, nil // no DH match client — skip
		}
		invID := s.inlineMatchAndPush(ctx, p)
		if invID == 0 {
			return outcomeSkipped, errors.New("inline match/push failed") // unmatched or failed — skip listing
		}
		p.DHInventoryID = invID
	}

	if p.DHInventoryID == 0 {
		// Not yet pushed to DH and not eligible for inline push. This used
		// to silently drop the row; log so stranded purchases are visible.
		s.logger.Warn(ctx, "dh listing: purchase not enrolled in push pipeline; skipping",
			observability.String("cert", p.CertNumber),
			observability.String("purchaseID", p.ID),
			observability.String("dhPushStatus", string(p.DHPushStatus)))
		return outcomeSkipped, fmt.Errorf("not enrolled in DH push pipeline (status %s)", p.DHPushStatus)
	}

	// Gate: require a human-committed price (reviewed or override)
	// before flipping an item to listed on DH. DH honors
	// listing_price_cents as-is, so sending anything not human-approved
	// (e.g. a stale CL value) risks listing at the wrong price. Without
	// one, skip and leave the item in_stock.
	listingPrice := ResolveListingPriceCents(p)
	if listingPrice == 0 {
		s.logger.Warn(ctx, "dh listing: no committed price; skipping list transition",
			observability.String("cert", p.CertNumber),
			observability.String("purchaseID", p.ID))
		return outcomeSkipped, errors.New("no committed price; skipped list transition")
	}

	// DH's inventory_upsert_service cancels+recreates MarketOrders on
	// every PATCH regardless of whether values changed, so skip the call
	// when status/price/channels are already in the target state. Empty
	// DHChannelsJSON means no prior successful sync on record — fall
	// through in that case even if status/price look correct.
	if p.DHStatus == inventory.DHStatusListed &&
		p.DHListingPriceCents == listingPrice &&
		p.DHChannelsJSON != "" {
		return outcomeAlreadyInSync, nil
	}

	dhListingPrice, err := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, inventory.DHInventoryStatusUpdate{
		Status:            inventory.DHStatusListed,
		ListingPriceCents: listingPrice,
		CertImageURLFront: p.FrontImageURL,
		CertImageURLBack:  p.BackImageURL,
	})
	if err != nil {
		return s.handleStatusUpdateFailure(ctx, p, err)
	}

	defaultChannels := DefaultListingChannels
	if syncErr := s.lister.SyncChannels(ctx, p.DHInventoryID, defaultChannels); syncErr != nil {
		s.revertAfterChannelSyncFailure(ctx, p, syncErr)
		return outcomeSkipped, syncErr
	}

	// Persist listed status and channel info locally.
	if s.fieldsUpdater != nil {
		if persistErr := s.persistListed(ctx, p, defaultChannels, dhListingPrice); persistErr != nil {
			return outcomeSyncedNotPersisted, persistErr
		}
	}
	return outcomeListed, nil
}

// handleStatusUpdateFailure classifies a failed UpdateInventoryStatus call into
// its three distinct branches: a stale DH inventory ID (reset for re-push), PSA
// key exhaustion (batch-fatal), or a generic failure.
func (s *dhListingService) handleStatusUpdateFailure(ctx context.Context, p *inventory.Purchase, err error) (listOutcome, error) {
	switch {
	case apperrors.HasErrorCode(err, apperrors.ErrCodeProviderNotFound):
		s.logger.Error(ctx, "dh listing: stale inventory ID — resetting for re-push",
			observability.String("cert", p.CertNumber),
			observability.String("purchaseID", p.ID),
			observability.Int("staleDHInventoryID", p.DHInventoryID),
			observability.Err(err))
		if s.resetter != nil {
			if resetErr := s.resetter.ResetDHFieldsForRepush(ctx, p.ID); resetErr != nil {
				s.logger.Warn(ctx, "dh listing: failed to reset stale DH fields",
					observability.String("cert", p.CertNumber),
					observability.Err(resetErr))
			}
		}
		return outcomeSkipped, err

	case errors.Is(err, ErrPSAKeysExhausted):
		s.logger.Warn(ctx, "dh listing: PSA keys exhausted — deferring list",
			observability.String("cert", p.CertNumber),
			observability.String("purchaseID", p.ID),
			observability.Err(err))
		s.recordEvent(ctx, dhevents.Event{
			PurchaseID:    p.ID,
			CertNumber:    p.CertNumber,
			Type:          dhevents.TypeListDeferred,
			DHInventoryID: p.DHInventoryID,
			DHCardID:      p.DHCardID,
			Source:        dhevents.SourceDHListing,
			Notes:         "psa_auth_exhausted",
		})
		// Rotation state is shared across all purchases in this call, so
		// retrying subsequent items would just re-exhaust and spam events.
		return outcomeAborted, err

	default:
		s.logger.Warn(ctx, "dh listing: status update failed",
			observability.String("cert", p.CertNumber),
			observability.Int("inventoryID", p.DHInventoryID),
			observability.Err(err))
		return outcomeSkipped, err
	}
}

// revertAfterChannelSyncFailure puts the item back to in_stock on DH after a
// channel sync failure, so it doesn't stay "listed" without synced channels,
// and mirrors the revert locally when a fields updater is configured.
func (s *dhListingService) revertAfterChannelSyncFailure(ctx context.Context, p *inventory.Purchase, syncErr error) {
	s.logger.Warn(ctx, "dh listing: channel sync failed, reverting to in_stock",
		observability.String("cert", p.CertNumber),
		observability.Int("inventoryID", p.DHInventoryID),
		observability.Err(syncErr))
	if _, revertErr := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, inventory.DHInventoryStatusUpdate{
		Status: inventory.DHStatusInStock,
	}); revertErr != nil {
		s.logger.Error(ctx, "dh listing: failed to revert status after sync failure",
			observability.String("cert", p.CertNumber),
			observability.Int("inventoryID", p.DHInventoryID),
			observability.Err(revertErr))
		return
	}
	if s.fieldsUpdater == nil {
		return
	}
	// Persist reverted status locally so readers don't see stale data.
	if persistErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
		CardID:      p.DHCardID,
		InventoryID: p.DHInventoryID,
		CertStatus:  DHCertStatusMatched,
		DHStatus:    inventory.DHStatusInStock,
	}); persistErr != nil {
		s.logger.Warn(ctx, "dh listing: failed to persist reverted status",
			observability.String("cert", p.CertNumber), observability.Err(persistErr))
	}
}

// persistListed writes the listed status and channel info to the local
// purchase, then records the listed/channel-synced events and clears the
// "unlisted on DH" badge. Returns non-nil only when the local write failed.
func (s *dhListingService) persistListed(ctx context.Context, p *inventory.Purchase, channels []string, dhListingPrice int) error {
	channelsJSON, marshalErr := json.Marshal(channels)
	if marshalErr != nil {
		s.logger.Error(ctx, "dh listing: failed to marshal channels",
			observability.String("cert", p.CertNumber),
			observability.Err(marshalErr))
		return marshalErr
	}
	if persistErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
		CardID:            p.DHCardID,
		InventoryID:       p.DHInventoryID,
		CertStatus:        DHCertStatusMatched,
		DHStatus:          inventory.DHStatusListed,
		ChannelsJSON:      string(channelsJSON),
		ListingPriceCents: dhListingPrice,
	}); persistErr != nil {
		s.logger.Error(ctx, "dh listing: failed to persist listed status — decrementing listed count",
			observability.String("cert", p.CertNumber), observability.Err(persistErr))
		return persistErr
	}
	s.recordEvent(ctx, dhevents.Event{
		PurchaseID:    p.ID,
		CertNumber:    p.CertNumber,
		Type:          dhevents.TypeListed,
		NewDHStatus:   string(inventory.DHStatusListed),
		DHInventoryID: p.DHInventoryID,
		DHCardID:      p.DHCardID,
		Source:        dhevents.SourceDHListing,
	})
	s.recordEvent(ctx, dhevents.Event{
		PurchaseID:    p.ID,
		CertNumber:    p.CertNumber,
		Type:          dhevents.TypeChannelSynced,
		Notes:         string(channelsJSON),
		DHInventoryID: p.DHInventoryID,
		Source:        dhevents.SourceDHListing,
	})

	// Best-effort: clear the "unlisted on DH" badge now that the item
	// is listed again. A failure here must not abort the listing.
	if s.unlistedClearer != nil {
		if clearErr := s.unlistedClearer.ClearDHUnlistedDetectedAt(ctx, p.ID); clearErr != nil {
			s.logger.Warn(ctx, "dh listing: failed to clear dh_unlisted_detected_at",
				observability.String("purchaseID", p.ID),
				observability.Err(clearErr))
		}
	}
	return nil
}
