package dhlisting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// dhListingService implements Service by coordinating inline cert intake
// (via DH's psa_import), list transitions, channel sync, and persistence.
type dhListingService struct {
	purchaseLookup    DHListingPurchaseLookup
	psaImporter       DHPSAImporter
	lister            DHInventoryLister
	cardIDSaver       DHCardIDSaver
	fieldsUpdater     DHListingFieldsUpdater
	pushStatusUpdater DHListingPushStatusUpdater
	resetter          DHReconcileResetter      // optional: auto-resets stale DH inventory IDs inline
	unlistedClearer   DHListingUnlistedClearer // optional: clears dh_unlisted_detected_at on successful list
	configLoader      DHListingConfigLoader    // optional: gates listing on the global ListingsPaused toggle
	logger            observability.Logger
	eventRec          dhevents.Recorder // may be nil
}

// DHListingConfigLoader loads the DH push safety config so the listing service
// can honor the global ListingsPaused toggle. Mirrors the push scheduler's
// DHPushConfigLoader; defined here (not imported) so the domain service does
// not depend on the scheduler adapter.
type DHListingConfigLoader interface {
	GetDHPushConfig(ctx context.Context) (*inventory.DHPushConfig, error)
}

// DHListingPurchaseLookup retrieves purchases by cert numbers.
type DHListingPurchaseLookup interface {
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
}

// DHListingFieldsUpdater persists DH tracking fields on local purchases.
type DHListingFieldsUpdater interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error
}

// DHListingPushStatusUpdater sets the DH push pipeline status on a purchase.
type DHListingPushStatusUpdater interface {
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
}

// DHListingUnlistedClearer clears the dh_unlisted_detected_at timestamp on a
// purchase after it successfully transitions back to `listed` on DH. The
// column is a UI badge marker set by the reconciler when a DH-side delisting
// is detected; clearing it removes the "unlisted on DH" indicator.
type DHListingUnlistedClearer interface {
	ClearDHUnlistedDetectedAt(ctx context.Context, purchaseID string) error
}

// DHListingServiceOption configures optional dependencies on dhListingService.
type DHListingServiceOption func(*dhListingService)

// WithDHListingPSAImporter enables inline cert intake via DH's psa_import
// endpoint (match + inventory-create in one call).
func WithDHListingPSAImporter(imp DHPSAImporter) DHListingServiceOption {
	return func(s *dhListingService) { s.psaImporter = imp }
}

// WithDHListingLister enables DH inventory status updates and channel sync.
func WithDHListingLister(l DHInventoryLister) DHListingServiceOption {
	return func(s *dhListingService) { s.lister = l }
}

// WithDHListingCardIDSaver enables persisting DH card ID mappings.
func WithDHListingCardIDSaver(saver DHCardIDSaver) DHListingServiceOption {
	return func(s *dhListingService) { s.cardIDSaver = saver }
}

// WithDHListingFieldsUpdater enables persisting DH fields after push.
func WithDHListingFieldsUpdater(u DHListingFieldsUpdater) DHListingServiceOption {
	return func(s *dhListingService) { s.fieldsUpdater = u }
}

// WithDHListingPushStatusUpdater enables setting dh_push_status.
func WithDHListingPushStatusUpdater(u DHListingPushStatusUpdater) DHListingServiceOption {
	return func(s *dhListingService) { s.pushStatusUpdater = u }
}

// WithDHListingResetter enables inline reset of stale DH inventory IDs.
// When UpdateInventoryStatus returns ERR_PROV_NOT_FOUND, the purchase is
// reset to pending so the push pipeline re-enrolls it on the next run.
func WithDHListingResetter(r DHReconcileResetter) DHListingServiceOption {
	return func(s *dhListingService) { s.resetter = r }
}

// WithDHListingUnlistedClearer enables best-effort clearing of the
// dh_unlisted_detected_at timestamp when a purchase successfully re-lists on
// DH. If unset, the "unlisted on DH" badge persists until the reconciler
// clears it through its own path.
func WithDHListingUnlistedClearer(c DHListingUnlistedClearer) DHListingServiceOption {
	return func(s *dhListingService) { s.unlistedClearer = c }
}

// WithEventRecorder injects a DH event recorder. Optional — if nil, no
// events are written but listing behavior is unchanged.
func WithEventRecorder(r dhevents.Recorder) DHListingServiceOption {
	return func(s *dhListingService) { s.eventRec = r }
}

// WithDHListingConfigLoader injects the DH push config loader so ListPurchases
// honors the global ListingsPaused toggle. Without it, the listing service
// lists unconditionally (the toggle only gates the push scheduler).
func WithDHListingConfigLoader(loader DHListingConfigLoader) DHListingServiceOption {
	return func(s *dhListingService) { s.configLoader = loader }
}

// NewDHListingService creates a new Service.
// purchaseLookup and logger are required; all other dependencies are optional.
func NewDHListingService(
	purchaseLookup DHListingPurchaseLookup,
	logger observability.Logger,
	opts ...DHListingServiceOption,
) (Service, error) {
	if purchaseLookup == nil {
		return nil, fmt.Errorf("purchaseLookup is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	s := &dhListingService{
		purchaseLookup: purchaseLookup,
		logger:         logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// recordEvent writes an event to the recorder if available.
// If recording fails, the error is logged but does not abort the operation.
func (s *dhListingService) recordEvent(ctx context.Context, e dhevents.Event) {
	if s.eventRec == nil {
		return
	}
	if err := s.eventRec.Record(ctx, e); err != nil {
		s.logger.Warn(ctx, "dh listing: record event failed",
			observability.String("type", string(e.Type)),
			observability.Err(err))
	}
}

// ListPurchases implements Service.
func (s *dhListingService) ListPurchases(ctx context.Context, certNumbers []string) DHListingResult {
	if s.lister == nil || len(certNumbers) == 0 {
		return DHListingResult{}
	}

	// Honor the global ListingsPaused toggle. Every non-scheduler listing path
	// (cert import, scan-cert, reviewed-price/override auto-list, the manual
	// "List on DH" button) funnels through here, so this is the single gate
	// that makes the admin "Pause DH Listings" switch effective for them — the
	// push scheduler enforces the same toggle independently. When paused we
	// short-circuit before any DH contact: no inline psa_import, no status
	// update, no channel sync. Items are left in their current state and resume
	// listing once the toggle is cleared; the Paused flag lets callers (e.g. the
	// manual-list handler) report the pause distinctly instead of as a failure.
	//
	// Fail closed: a config-load error is treated as paused. The toggle exists
	// for card-show liquidation windows where listing on DH would undercut live
	// in-person sales — an irreversible action — whereas skipping merely defers
	// listing until the next trigger or scheduler cycle. So on a transient DB
	// error we prefer the recoverable outcome (skip) over the irreversible one.
	if s.configLoader != nil {
		cfg, cfgErr := s.configLoader.GetDHPushConfig(ctx)
		switch {
		case cfgErr != nil:
			s.logger.Warn(ctx, "dh listing: failed to load push config; treating as paused (fail closed)",
				observability.Err(cfgErr))
			return DHListingResult{Skipped: len(certNumbers), Total: len(certNumbers), Paused: true}
		case cfg != nil && cfg.ListingsPaused:
			s.logger.Info(ctx, "dh listing: listings paused — skipping list run",
				observability.Int("certs", len(certNumbers)))
			return DHListingResult{Skipped: len(certNumbers), Total: len(certNumbers), Paused: true}
		}
	}

	// Reset PSA key rotation so a previously-exhausted rotation index from a
	// push cycle doesn't silently fail every list attempt in this call.
	if rotator, ok := s.lister.(PSAKeyRotator); ok {
		rotator.ResetPSAKeyRotation()
	}

	purchases, err := s.purchaseLookup.GetPurchasesByCertNumbers(ctx, certNumbers)
	if err != nil {
		s.logger.Warn(ctx, "dh listing: batch cert lookup failed", observability.Err(err))
		return DHListingResult{Error: err}
	}

	// Sort cert numbers for deterministic iteration order.
	sortedCerts := make([]string, 0, len(purchases))
	for cn := range purchases {
		sortedCerts = append(sortedCerts, cn)
	}
	sort.Strings(sortedCerts)

	listed, synced, skipped := 0, 0, 0
	failedCerts := map[string]error{}
	for _, cn := range sortedCerts {
		p := purchases[cn]
		// Hard gate: never put an item live on DH unless it has at least left
		// PSA (received in hand, or PSA-shipped). The push scheduler enforces
		// this via GetPurchasesByDHPushStatus, but the inline/manual/auto-list
		// paths reach ListPurchases directly and previously bypassed it — a
		// price review on a not-yet-received card would inline-push and list it.
		if !p.IsReceivedOrShipped() {
			s.logger.Warn(ctx, "dh listing: purchase not received or shipped; refusing to list",
				observability.String("cert", p.CertNumber),
				observability.String("purchaseID", p.ID))
			failedCerts[cn] = errors.New("not received or shipped by PSA; cannot list on DH")
			skipped++
			continue
		}
		// If pending DH push, do inline match + push first.
		if p.DHInventoryID == 0 && p.DHPushStatus == inventory.DHPushStatusPending {
			if s.psaImporter == nil {
				skipped++
				continue // no DH match client — skip
			}
			invID := s.inlineMatchAndPush(ctx, p)
			if invID == 0 {
				failedCerts[cn] = errors.New("inline match/push failed")
				skipped++
				continue // unmatched or failed — skip listing
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
			failedCerts[cn] = fmt.Errorf("not enrolled in DH push pipeline (status %s)", p.DHPushStatus)
			skipped++
			continue
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
			failedCerts[cn] = errors.New("no committed price; skipped list transition")
			skipped++
			continue
		}

		// DH's inventory_upsert_service cancels+recreates MarketOrders on
		// every PATCH regardless of whether values changed, so skip the call
		// when status/price/channels are already in the target state. Empty
		// DHChannelsJSON means no prior successful sync on record — fall
		// through in that case even if status/price look correct.
		if p.DHStatus == inventory.DHStatusListed &&
			p.DHListingPriceCents == listingPrice &&
			p.DHChannelsJSON != "" {
			synced++
			continue
		}

		dhListingPrice, err := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, DHInventoryStatusUpdate{
			Status:            inventory.DHStatusListed,
			ListingPriceCents: listingPrice,
			CertImageURLFront: p.FrontImageURL,
			CertImageURLBack:  p.BackImageURL,
		})
		if err != nil {
			if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderNotFound) {
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
				failedCerts[cn] = err
			} else if errors.Is(err, ErrPSAKeysExhausted) {
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
				// Short-circuit the batch: rotation state is shared across
				// all purchases in this call, so retrying subsequent items
				// would just re-exhaust and spam events. Count the current
				// purchase plus every untouched one as skipped so the result
				// invariant Listed + Synced + Skipped == Total holds.
				return DHListingResult{
					Listed:      listed,
					Synced:      synced,
					Skipped:     len(purchases) - listed - synced,
					Total:       len(purchases),
					Error:       err,
					FailedCerts: nilIfEmpty(failedCerts),
				}
			} else {
				s.logger.Warn(ctx, "dh listing: status update failed",
					observability.String("cert", p.CertNumber),
					observability.Int("inventoryID", p.DHInventoryID),
					observability.Err(err))
				failedCerts[cn] = err
			}
			skipped++
			continue
		}
		listed++

		defaultChannels := DefaultListingChannels
		if err := s.lister.SyncChannels(ctx, p.DHInventoryID, defaultChannels); err != nil {
			s.logger.Warn(ctx, "dh listing: channel sync failed, reverting to in_stock",
				observability.String("cert", p.CertNumber),
				observability.Int("inventoryID", p.DHInventoryID),
				observability.Err(err))
			failedCerts[cn] = err
			// Revert status so the item doesn't stay "listed" without channel sync.
			if _, revertErr := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, DHInventoryStatusUpdate{
				Status: inventory.DHStatusInStock,
			}); revertErr != nil {
				s.logger.Error(ctx, "dh listing: failed to revert status after sync failure",
					observability.String("cert", p.CertNumber),
					observability.Int("inventoryID", p.DHInventoryID),
					observability.Err(revertErr))
			} else if s.fieldsUpdater != nil {
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
			listed-- // revert the listed count
			skipped++
			continue
		}
		synced++

		// Persist listed status and channel info locally.
		if s.fieldsUpdater != nil {
			channelsJSON, marshalErr := json.Marshal(defaultChannels)
			if marshalErr != nil {
				s.logger.Error(ctx, "dh listing: failed to marshal channels",
					observability.String("cert", p.CertNumber),
					observability.Err(marshalErr))
				failedCerts[cn] = marshalErr
				listed--
				skipped++
				continue
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
				failedCerts[cn] = persistErr
				listed--
				skipped++
				continue
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
		}
	}

	if listed > 0 || synced > 0 {
		s.logger.Info(ctx, "dh listing completed",
			observability.Int("listed", listed),
			observability.Int("synced", synced),
			observability.Int("certs", len(certNumbers)))
	} else if len(purchases) > 0 {
		s.logger.Warn(ctx, "dh listing completed with no successful operations",
			observability.Int("certs", len(certNumbers)))
	}

	return DHListingResult{Listed: listed, Synced: synced, Skipped: skipped, Total: len(purchases), FailedCerts: nilIfEmpty(failedCerts)}
}

// nilIfEmpty returns nil when the map is empty so FailedCerts stays nil on
// fully-successful runs (avoids a non-nil empty map in the result).
func nilIfEmpty(m map[string]error) map[string]error {
	if len(m) == 0 {
		return nil
	}
	return m
}
