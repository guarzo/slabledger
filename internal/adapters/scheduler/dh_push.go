package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const dhPushBatchLimit = 50

type processResult int

const (
	processMatched processResult = iota
	processUnmatched
	processSkipped
	processHeld
	// processMatchedComplete indicates the purchase matched AND was fully
	// persisted inline (psa_import creates inventory server-side). Callers
	// treat it like processMatched for counting.
	processMatchedComplete
)

// DHPushPendingLister returns purchases pending DH push.
type DHPushPendingLister interface {
	GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
}

// DHPushStatusUpdater updates the DH push status on a purchase.
type DHPushStatusUpdater interface {
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
}

// DHPushAttemptIncrementer atomically increments dh_push_attempts and returns
// the new value. Used to auto-dismiss certs that keep hitting partner_card_error.
type DHPushAttemptIncrementer interface {
	IncrementDHPushAttempts(ctx context.Context, id string) (int, error)
}

// DHPushCardIDSaver persists DH card ID mappings to the card_id_mappings
// table so other consumers of external IDs (e.g. price sync) can reuse them
// without re-hitting psa_import.
type DHPushCardIDSaver interface {
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
}

// DHPushConfigLoader loads DH push safety config.
type DHPushConfigLoader interface {
	GetDHPushConfig(ctx context.Context) (*inventory.DHPushConfig, error)
}

// DHPushHoldSetter atomically sets a purchase to held status with a reason.
type DHPushHoldSetter interface {
	SetHeldWithReason(ctx context.Context, purchaseID string, reason string) error
}

// DHPushRelister re-runs the list pipeline for a set of cert numbers. Used by
// processPurchase when the reconciler has flagged a row as unlisted on DH
// (dh_unlisted_detected_at set) and the inventory poll has re-discovered it
// with a new dh_inventory_id but left dh_push_status='pending'. Without this,
// the scheduler's "already has inventory ID" guard short-circuits to matched
// and the row never re-lists.
//
// Minimal subset of dhlisting.Service so the scheduler doesn't depend on the
// full Service surface — same pattern as DHPushStatusUpdater.
type DHPushRelister interface {
	ListPurchases(ctx context.Context, certNumbers []string) dhlisting.DHListingResult
}

// DHPushConfig controls the DH push scheduler.
type DHPushConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHPushOption configures optional dependencies on a DHPushScheduler.
type DHPushOption func(*DHPushScheduler)

// WithDHPushAttemptIncrementer wires the dh_push_attempts incrementer so the
// scheduler can auto-dismiss certs that repeatedly fail partner_card_error.
func WithDHPushAttemptIncrementer(inc DHPushAttemptIncrementer) DHPushOption {
	return func(s *DHPushScheduler) { s.attemptInc = inc }
}

// WithDHPushConfigLoader injects a config loader for DH push safety thresholds.
func WithDHPushConfigLoader(loader DHPushConfigLoader) DHPushOption {
	return func(s *DHPushScheduler) { s.configLoader = loader }
}

// WithDHPushHoldSetter injects a setter for atomically holding purchases with a reason.
func WithDHPushHoldSetter(setter DHPushHoldSetter) DHPushOption {
	return func(s *DHPushScheduler) { s.holdSetter = setter }
}

// WithDHPushEventRecorder enables DH state-event recording for hold and
// unmatched transitions driven by the push scheduler's safety gates.
func WithDHPushEventRecorder(r dhevents.Recorder) DHPushOption {
	return func(s *DHPushScheduler) { s.eventRec = r }
}

// WithDHPushRelister injects the listing service used to re-list rows that
// the reconciler flagged as unlisted and the inventory poll re-discovered
// (DHInventoryID set, DHUnlistedDetectedAt set, dh_push_status still pending).
// Without it, processPurchase falls back to the legacy "flip to matched"
// behavior — which strands those rows in_stock indefinitely.
func WithDHPushRelister(r DHPushRelister) DHPushOption {
	return func(s *DHPushScheduler) { s.relister = r }
}

// DHPushScheduler routes pending purchases through DH's psa_import endpoint
// (match + inventory-create in one call) on a fixed interval.
type DHPushScheduler struct {
	StopHandle
	pendingLister DHPushPendingLister
	statusUpdater DHPushStatusUpdater
	attemptInc    DHPushAttemptIncrementer
	psaImporter   DHPushPSAImporter
	fieldsUpdater DHFieldsUpdater
	cardIDSaver   DHPushCardIDSaver
	configLoader  DHPushConfigLoader
	holdSetter    DHPushHoldSetter
	relister      DHPushRelister
	eventRec      dhevents.Recorder
	logger        observability.Logger
	config        DHPushConfig
}

// recordEvent emits an event to the recorder if present. Failures are logged but do not abort.
func (s *DHPushScheduler) recordEvent(ctx context.Context, e dhevents.Event) {
	if s.eventRec == nil {
		return
	}
	if err := s.eventRec.Record(ctx, e); err != nil {
		s.logger.Warn(ctx, "dh push: record event failed",
			observability.String("type", string(e.Type)),
			observability.Err(err))
	}
}

// NewDHPushScheduler creates a new DH push scheduler.
func NewDHPushScheduler(
	pendingLister DHPushPendingLister,
	statusUpdater DHPushStatusUpdater,
	psaImporter DHPushPSAImporter,
	fieldsUpdater DHFieldsUpdater,
	cardIDSaver DHPushCardIDSaver,
	logger observability.Logger,
	config DHPushConfig,
	opts ...DHPushOption,
) *DHPushScheduler {
	if config.Interval <= 0 {
		config.Interval = 5 * time.Minute
	}
	s := &DHPushScheduler{
		StopHandle:    NewStopHandle(),
		pendingLister: pendingLister,
		statusUpdater: statusUpdater,
		psaImporter:   psaImporter,
		fieldsUpdater: fieldsUpdater,
		cardIDSaver:   cardIDSaver,
		logger:        logger.With(context.Background(), observability.String("component", "dh-push")),
		config:        config,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Start begins the DH push loop.
func (s *DHPushScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "dh push scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-push",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.push)
}

// push processes pending purchases, routing each through DH's psa_import.
func (s *DHPushScheduler) push(ctx context.Context) {
	// Reset PSA key rotation at the start of each cycle so previously
	// rate-limited keys are retried — PSA's per-key rate limits are typically
	// hourly/daily so they clear between cycles.
	if rotator, ok := s.psaImporter.(dh.PSAKeyRotator); ok {
		rotator.ResetPSAKeyRotation()
	}

	pending, err := s.pendingLister.GetPurchasesByDHPushStatus(ctx, inventory.DHPushStatusPending, dhPushBatchLimit)
	if err != nil {
		s.logger.Warn(ctx, "dh push: failed to list pending purchases", observability.Err(err))
		return
	}

	if len(pending) == 0 {
		s.logger.Debug(ctx, "dh push: no pending purchases")
		return
	}

	// Load push safety config; fall back to defaults if unavailable.
	pushCfg := inventory.DefaultDHPushConfig()
	if s.configLoader != nil {
		if loaded, loadErr := s.configLoader.GetDHPushConfig(ctx); loadErr != nil {
			s.logger.Warn(ctx, "dh push: failed to load push config, using defaults", observability.Err(loadErr))
		} else if loaded != nil {
			pushCfg = *loaded
		}
	}

	matched := 0
	unmatched := 0
	skipped := 0
	held := 0

	for _, p := range pending {
		switch s.processPurchase(ctx, p, pushCfg) {
		case processMatched, processMatchedComplete:
			matched++
		case processUnmatched:
			unmatched++
		case processSkipped:
			skipped++
		case processHeld:
			held++
		}
	}

	s.logger.Info(ctx, "dh push completed",
		observability.Int("total", len(pending)),
		observability.Int("matched", matched),
		observability.Int("unmatched", unmatched),
		observability.Int("skipped", skipped),
		observability.Int("held", held),
	)
}

func (s *DHPushScheduler) processPurchase(ctx context.Context, p inventory.Purchase, pushCfg inventory.DHPushConfig) processResult {
	// Guard: if a previous cycle pushed successfully (inventory ID set) but the
	// status update failed, just fix the status rather than re-pushing.
	if p.DHInventoryID != 0 {
		// Auto-relist path: the reconciler flagged this row as unlisted on DH
		// (dh_unlisted_detected_at set) and the inventory poll re-discovered it
		// with a fresh inventory ID, but never wrote dh_push_status. Without
		// this branch the row sits in_stock forever. Only relist when the
		// operator has committed a listing price and the row carries a cert
		// number; otherwise fall through to the legacy "fix status to matched"
		// branch and wait for a price commit or cert backfill.
		if p.DHUnlistedDetectedAt != nil && s.relister != nil && p.CertNumber != "" && dhlisting.ResolveListingPriceCents(&p) > 0 {
			// Honor the global ListingsPaused toggle for scheduler-driven
			// re-lists. Manual UI-initiated lists bypass this — see
			// handlers/campaigns_dh_listing.go. During card-show liquidation
			// windows we don't want the scheduler quietly re-listing rows
			// the operator just unlisted on DH.
			if pushCfg.ListingsPaused {
				s.logger.Info(ctx, "dh push: listings paused — skipping scheduler re-list",
					observability.String("purchaseID", p.ID),
					observability.String("cert", p.CertNumber))
				return processSkipped
			}
			s.logger.Info(ctx, "dh push: re-listing previously-unlisted purchase via dhlisting service",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.Int("dhInventoryID", p.DHInventoryID))
			result := s.relister.ListPurchases(ctx, []string{p.CertNumber})
			if result.Error != nil {
				s.logger.Warn(ctx, "dh push: re-list failed; next cycle will retry",
					observability.String("purchaseID", p.ID),
					observability.String("cert", p.CertNumber),
					observability.Err(result.Error))
				return processSkipped
			}
			// The listing service is responsible for clearing
			// dh_unlisted_detected_at and flipping dh_push_status. If Listed=0
			// but Synced covers the full batch, the row is already in the
			// target state on DH (already listed + price + channels match);
			// treat as terminal so we don't re-process it every cycle.
			// Otherwise the row didn't transition (paused, inline-push failure,
			// etc.) — leave it as-is and log so the no-op is diagnosable.
			if result.Listed == 0 {
				if result.Synced > 0 && result.Synced == result.Total {
					return processMatched
				}
				s.logger.Warn(ctx, "dh push: re-list did not transition row; will retry next cycle",
					observability.String("purchaseID", p.ID),
					observability.String("cert", p.CertNumber),
					observability.Int("listed", result.Listed),
					observability.Int("synced", result.Synced),
					observability.Int("skipped", result.Skipped),
					observability.Int("total", result.Total))
				return processSkipped
			}
			return processMatched
		}

		s.logger.Info(ctx, "dh push: purchase already has inventory ID, fixing status to matched",
			observability.String("purchaseID", p.ID),
			observability.Int("dhInventoryID", p.DHInventoryID))
		if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusMatched); updateErr != nil {
			s.logger.Warn(ctx, "dh push: failed to fix status on already-pushed purchase",
				observability.String("purchaseID", p.ID), observability.Err(updateErr))
		}
		return processMatched
	}

	if p.CertNumber == "" {
		s.logger.Warn(ctx, "dh push: purchase has no cert number, marking unmatched",
			observability.String("purchaseID", p.ID))
		if !s.markUnmatched(ctx, p, "purchase has no cert number") {
			return processSkipped
		}
		return processUnmatched
	}

	// Honor the global ListingsPaused toggle for scheduler-driven pushes
	// so card-show liquidation windows aren't undercut by fresh DH listings.
	// Manual UI-initiated lists bypass this in the handler. Evaluated before
	// EvaluateHoldTriggers so a paused row is just skipped and not mutated to
	// `held` (which would survive after the toggle is turned back off and
	// require operator review).
	if pushCfg.ListingsPaused {
		s.logger.Info(ctx, "dh push: listings paused — skipping psa_import",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber))
		return processSkipped
	}

	// Safety gates (capital-at-risk, unknown campaign, etc.) hold the row
	// before we hit DH. Evaluated here so a push the operator wants to
	// review never reaches psa_import.
	if holdReason := dhlisting.EvaluateHoldTriggers(&p, pushCfg); holdReason != "" {
		return s.setHeld(ctx, p, holdReason)
	}

	return s.pushViaPSAImport(ctx, p)
}

// setHeld atomically flips a purchase to held status with a reason, records
// the transition, and returns processHeld on success. If the DB write fails,
// returns processSkipped so the batch counters (and the caller's view of the
// row's state) don't falsely attribute a held transition that didn't happen;
// the row stays pending and the next scheduler cycle retries.
func (s *DHPushScheduler) setHeld(ctx context.Context, p inventory.Purchase, reason string) processResult {
	s.logger.Info(ctx, "dh push: holding re-push for review",
		observability.String("purchaseID", p.ID),
		observability.String("cert", p.CertNumber),
		observability.String("reason", reason))

	held := false
	if s.holdSetter != nil {
		if updateErr := s.holdSetter.SetHeldWithReason(ctx, p.ID, reason); updateErr != nil {
			s.logger.Warn(ctx, "dh push: failed to set held status+reason",
				observability.String("purchaseID", p.ID), observability.Err(updateErr))
		} else {
			held = true
		}
	} else if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusHeld); updateErr != nil {
		s.logger.Warn(ctx, "dh push: failed to set held status",
			observability.String("purchaseID", p.ID), observability.Err(updateErr))
	} else {
		held = true
	}

	if !held {
		return processSkipped
	}

	s.recordEvent(ctx, dhevents.Event{
		PurchaseID:    p.ID,
		CertNumber:    p.CertNumber,
		Type:          dhevents.TypeHeld,
		NewPushStatus: inventory.DHPushStatusHeld,
		Notes:         reason,
		Source:        dhevents.SourceDHPush,
	})
	return processHeld
}

// markUnmatched sets a purchase's DH push status to unmatched and emits a
// dh_state_events row documenting the transition. Used only for cases that
// are genuinely unresolvable from the scheduler's perspective — today that
// means "no cert number on the purchase." Transient DH/PSA errors stay
// pending; there is no consecutive-skip cap.
//
// Returns true if the status update succeeded. Callers should treat a false
// return as processSkipped.
func (s *DHPushScheduler) markUnmatched(ctx context.Context, p inventory.Purchase, reason string) bool {
	prev := string(p.DHPushStatus)
	if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusUnmatched); updateErr != nil {
		s.logger.Warn(ctx, "dh push: failed to set unmatched status",
			observability.String("purchaseID", p.ID), observability.Err(updateErr))
		return false
	}
	s.recordEvent(ctx, dhevents.Event{
		PurchaseID:     p.ID,
		CertNumber:     p.CertNumber,
		Type:           dhevents.TypeUnmatched,
		PrevPushStatus: prev,
		NewPushStatus:  string(inventory.DHPushStatusUnmatched),
		Source:         dhevents.SourceDHPush,
		Notes:          reason,
	})
	return true
}

// Compile-time check that dh.Client satisfies the primary intake interface.
var _ DHPushPSAImporter = (*dh.Client)(nil)
