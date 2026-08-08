package dhlisting

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
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
		outcome, failErr := s.listOnePurchase(ctx, purchases[cn])
		switch outcome {
		case outcomeListed:
			listed++
			synced++
		case outcomeAlreadyInSync:
			synced++
		case outcomeSyncedNotPersisted:
			synced++
			skipped++
		case outcomeSkipped:
			skipped++
		case outcomeAborted:
			// Short-circuit the batch: rotation state is shared across all
			// purchases in this call, so retrying subsequent items would just
			// re-exhaust and spam events. The aborting purchase plus every
			// untouched one are folded into Skipped by subtraction, and the
			// aborting error is reported in Error, not FailedCerts.
			return DHListingResult{
				Listed:      listed,
				Synced:      synced,
				Skipped:     len(purchases) - listed - synced,
				Total:       len(purchases),
				Error:       failErr,
				FailedCerts: nilIfEmpty(failedCerts),
			}
		}
		if failErr != nil {
			failedCerts[cn] = failErr
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
