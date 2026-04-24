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

// DHPushConfig controls the DH push scheduler.
type DHPushConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHPushOption configures optional dependencies on a DHPushScheduler.
type DHPushOption func(*DHPushScheduler)

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

// DHPushScheduler routes pending purchases through DH's psa_import endpoint
// (match + inventory-create in one call) on a fixed interval.
type DHPushScheduler struct {
	StopHandle
	pendingLister DHPushPendingLister
	statusUpdater DHPushStatusUpdater
	psaImporter   DHPushPSAImporter
	fieldsUpdater DHFieldsUpdater
	cardIDSaver   DHPushCardIDSaver
	configLoader  DHPushConfigLoader
	holdSetter    DHPushHoldSetter
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
