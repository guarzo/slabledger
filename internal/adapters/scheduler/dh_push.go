package scheduler

import (
	"context"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

const dhPushBatchLimit = 50

type processResult int

const (
	processMatched processResult = iota
	processUnmatched
	processSkipped
	processHeld
)

// DHPushPendingLister returns purchases pending DH push.
type DHPushPendingLister interface {
	GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error)
}

// DHPushStatusUpdater updates the DH push status on a purchase.
type DHPushStatusUpdater interface {
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
}

// DHPushCertResolver resolves PSA certs to DH card IDs.
type DHPushCertResolver interface {
	ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}

// DHPushInventoryPusher pushes inventory items to DH.
type DHPushInventoryPusher interface {
	PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}

// DHPushCardIDSaver persists DH card ID mappings.
type DHPushCardIDSaver interface {
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	GetMappedSet(ctx context.Context, provider string) (map[string]string, error)
}

// DHPushCandidatesSaver stores DH cert resolution candidates on a purchase.
type DHPushCandidatesSaver interface {
	UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error
}

// DHPushConfigLoader loads DH push safety config.
type DHPushConfigLoader interface {
	GetDHPushConfig(ctx context.Context) (*campaigns.DHPushConfig, error)
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

// WithDHPushCandidatesSaver injects a candidates saver for storing ambiguous
// DH cert resolution candidates on purchases.
func WithDHPushCandidatesSaver(saver DHPushCandidatesSaver) DHPushOption {
	return func(s *DHPushScheduler) { s.candidatesSaver = saver }
}

// WithDHPushConfigLoader injects a config loader for DH push safety thresholds.
func WithDHPushConfigLoader(loader DHPushConfigLoader) DHPushOption {
	return func(s *DHPushScheduler) { s.configLoader = loader }
}

// WithDHPushHoldSetter injects a setter for atomically holding purchases with a reason.
func WithDHPushHoldSetter(setter DHPushHoldSetter) DHPushOption {
	return func(s *DHPushScheduler) { s.holdSetter = setter }
}

// DHPushScheduler matches pending purchases against DH and pushes inventory.
type DHPushScheduler struct {
	StopHandle
	pendingLister   DHPushPendingLister
	statusUpdater   DHPushStatusUpdater
	certResolver    DHPushCertResolver
	inventoryPush   DHPushInventoryPusher
	fieldsUpdater   DHFieldsUpdater
	cardIDSaver     DHPushCardIDSaver
	candidatesSaver DHPushCandidatesSaver
	configLoader    DHPushConfigLoader
	holdSetter      DHPushHoldSetter
	logger          observability.Logger
	config          DHPushConfig
}

// NewDHPushScheduler creates a new DH push scheduler.
// Optional dependencies (e.g. candidates saver) are injected via DHPushOption.
func NewDHPushScheduler(
	pendingLister DHPushPendingLister,
	statusUpdater DHPushStatusUpdater,
	certResolver DHPushCertResolver,
	inventoryPush DHPushInventoryPusher,
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
		certResolver:  certResolver,
		inventoryPush: inventoryPush,
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

// push processes pending purchases, matching them against DH and pushing inventory.
func (s *DHPushScheduler) push(ctx context.Context) {
	// Reset PSA key rotation at the start of each cycle so previously
	// rate-limited keys are retried (rate limits are typically hourly/daily).
	if rotator, ok := s.certResolver.(dh.PSAKeyRotator); ok {
		rotator.ResetPSAKeyRotation()
	}

	pending, err := s.pendingLister.GetPurchasesByDHPushStatus(ctx, campaigns.DHPushStatusPending, dhPushBatchLimit)
	if err != nil {
		s.logger.Warn(ctx, "dh push: failed to list pending purchases", observability.Err(err))
		return
	}

	if len(pending) == 0 {
		s.logger.Debug(ctx, "dh push: no pending purchases")
		return
	}

	// Load push safety config; fall back to defaults if unavailable.
	pushCfg := campaigns.DefaultDHPushConfig()
	if s.configLoader != nil {
		if loaded, loadErr := s.configLoader.GetDHPushConfig(ctx); loadErr != nil {
			s.logger.Warn(ctx, "dh push: failed to load push config, using defaults", observability.Err(loadErr))
		} else if loaded != nil {
			pushCfg = *loaded
		}
	}

	// Load existing DH card ID mappings to avoid redundant Match calls.
	mappedSet, err := s.cardIDSaver.GetMappedSet(ctx, pricing.SourceDH)
	if err != nil {
		s.logger.Warn(ctx, "dh push: failed to load mapped set, proceeding without cache",
			observability.Err(err))
		mappedSet = make(map[string]string)
	}

	matched := 0
	unmatched := 0
	skipped := 0
	held := 0

	for _, p := range pending {
		switch s.processPurchase(ctx, p, mappedSet, pushCfg) {
		case processMatched:
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

func (s *DHPushScheduler) processPurchase(ctx context.Context, p campaigns.Purchase, mappedSet map[string]string, pushCfg campaigns.DHPushConfig) processResult {
	// Guard: if a previous cycle pushed successfully (inventory ID set) but the
	// status update failed, just fix the status rather than re-pushing.
	if p.DHInventoryID != 0 {
		s.logger.Info(ctx, "dh push: purchase already has inventory ID, fixing status to matched",
			observability.String("purchaseID", p.ID),
			observability.Int("dhInventoryID", p.DHInventoryID))
		if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusMatched); updateErr != nil {
			s.logger.Warn(ctx, "dh push: failed to fix status on already-pushed purchase",
				observability.String("purchaseID", p.ID), observability.Err(updateErr))
		}
		return processMatched
	}

	if p.CertNumber == "" {
		s.logger.Warn(ctx, "dh push: purchase has no cert number, marking unmatched",
			observability.String("purchaseID", p.ID))
		if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); updateErr != nil {
			s.logger.Warn(ctx, "dh push: failed to set unmatched status for cert-less purchase",
				observability.String("purchaseID", p.ID), observability.Err(updateErr))
		}
		return processUnmatched
	}

	key := p.DHCardKey()

	// Attempt to reuse an existing DH card ID — either from the purchase itself
	// (re-push after CL value change) or from the mapping cache.
	var dhCardID int
	alreadyMapped := false

	if p.DHCardID != 0 {
		dhCardID = p.DHCardID
		alreadyMapped = true
	} else if dhCardIDStr := mappedSet[key]; dhCardIDStr != "" {
		if parsed, err := strconv.Atoi(dhCardIDStr); err == nil && parsed > 0 {
			dhCardID = parsed
			alreadyMapped = true
		}
	}

	// Never push before we have a market value — leave as pending for retry.
	marketValue := campaigns.ResolveMarketValueCents(&p)
	if marketValue == 0 {
		s.logger.Debug(ctx, "dh push: no market value yet, leaving as pending",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber))
		return processSkipped
	}

	if !alreadyMapped {
		resolved, result := s.resolveCert(ctx, p, mappedSet)
		if result != processMatched {
			return result
		}
		dhCardID = resolved
	}

	if holdReason := campaigns.EvaluateHoldTriggers(&p, pushCfg); holdReason != "" {
		s.logger.Info(ctx, "dh push: holding re-push for review",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.String("reason", holdReason))
		if s.holdSetter != nil {
			if updateErr := s.holdSetter.SetHeldWithReason(ctx, p.ID, holdReason); updateErr != nil {
				s.logger.Warn(ctx, "dh push: failed to set held status+reason",
					observability.String("purchaseID", p.ID), observability.Err(updateErr))
			}
		} else {
			if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusHeld); updateErr != nil {
				s.logger.Warn(ctx, "dh push: failed to set held status",
					observability.String("purchaseID", p.ID), observability.Err(updateErr))
			}
		}
		return processHeld
	}

	item := dh.InventoryItem{
		DHCardID:         dhCardID,
		CertNumber:       p.CertNumber,
		GradingCompany:   dh.GraderPSA,
		Grade:            p.GradeValue,
		CostBasisCents:   p.BuyCostCents,
		MarketValueCents: dh.IntPtr(marketValue),
		Status:           dh.InventoryStatusInStock,
	}

	pushResp, err := s.inventoryPush.PushInventory(ctx, []dh.InventoryItem{item})
	if err != nil {
		s.logger.Warn(ctx, "dh push: inventory push API error, leaving as pending",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.Err(err))
		return processSkipped
	}

	if len(pushResp.Results) == 0 {
		s.logger.Warn(ctx, "dh push: inventory push returned empty results",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber))
		return processSkipped
	}

	result := pushResp.Results[0]

	if result.Status == "failed" || result.DHInventoryID == 0 {
		s.logger.Warn(ctx, "dh push: push result indicates failure, will retry",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.String("resultStatus", result.Status),
			observability.Int("dhInventoryID", result.DHInventoryID))
		return processSkipped
	}

	update := campaigns.DHFieldsUpdate{
		CardID:            dhCardID,
		InventoryID:       result.DHInventoryID,
		CertStatus:        dh.CertStatusMatched,
		ListingPriceCents: result.AssignedPriceCents,
		ChannelsJSON:      dh.MarshalChannels(result.Channels),
		DHStatus:          campaigns.DHStatus(result.Status),
	}

	if updateErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, update); updateErr != nil {
		s.logger.Warn(ctx, "dh push: failed to update DH fields",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
		return processSkipped
	}

	if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusMatched); updateErr != nil {
		// Fields are already saved with the inventory ID, so the purchase won't
		// be re-pushed (NeedsDHPush checks DHInventoryID). Log at Error level
		// since the status is inconsistent but won't cause duplicate pushes.
		s.logger.Error(ctx, "dh push: DH fields saved but status update failed — purchase has inventory ID but status is not 'matched'",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
	}

	s.logger.Debug(ctx, "dh push: purchase matched and pushed",
		observability.String("purchaseID", p.ID),
		observability.String("cert", p.CertNumber),
		observability.Int("dhCardID", dhCardID),
		observability.Int("dhInventoryID", result.DHInventoryID),
	)

	return processMatched
}

// resolveCert resolves a purchase's cert number to a DH card ID, saving the
// mapping on success. Returns the card ID and processMatched, or 0 and a
// non-matched result indicating what happened.
func (s *DHPushScheduler) resolveCert(ctx context.Context, p campaigns.Purchase, mappedSet map[string]string) (int, processResult) {
	cardName, variant := campaigns.CleanCardNameForDH(p.CardName)
	var rotateFn func() bool
	if rotator, ok := s.certResolver.(dh.PSAKeyRotator); ok {
		rotateFn = rotator.RotatePSAKey
	}

	resp, err := dh.ResolveCertWithRotation(ctx, dh.CertResolveRequest{
		CertNumber: p.CertNumber,
		GemRateID:  p.GemRateID,
		CardName:   cardName,
		SetName:    p.SetName,
		CardNumber: p.CardNumber,
		Year:       p.CardYear,
		Variant:    variant,
	}, s.certResolver.ResolveCert, rotateFn, s.logger, "dh push")
	if err != nil {
		s.logger.Warn(ctx, "dh push: cert resolve error, leaving as pending",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.Err(err))
		return 0, processSkipped
	}

	switch {
	case resp.Status == dh.CertStatusMatched:
		dhCardID := resp.DHCardID
		s.saveCardIDMapping(ctx, p, dhCardID, mappedSet)
		return dhCardID, processMatched

	case resp.Status == dh.CertStatusAmbiguous && len(resp.Candidates) > 0:
		return s.resolveAmbiguousCert(ctx, p, resp.Candidates, mappedSet)

	default:
		s.logger.Debug(ctx, "dh push: cert not matched, marking unmatched",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.String("status", resp.Status))
		s.markUnmatched(ctx, p.ID)
		return 0, processUnmatched
	}
}

// resolveAmbiguousCert attempts to disambiguate candidates by card number.
func (s *DHPushScheduler) resolveAmbiguousCert(ctx context.Context, p campaigns.Purchase, candidates []dh.CertResolutionCandidate, mappedSet map[string]string) (int, processResult) {
	var saveFn func(string) error
	if s.candidatesSaver != nil {
		saveFn = func(j string) error { return s.candidatesSaver.UpdatePurchaseDHCandidates(ctx, p.ID, j) }
	}

	resolved, resolveErr := dh.ResolveAmbiguous(candidates, p.CardNumber, saveFn)
	if resolveErr != nil {
		s.logger.Warn(ctx, "dh push: failed to resolve/save candidates, will retry",
			observability.String("purchaseID", p.ID), observability.Err(resolveErr))
		return 0, processSkipped
	}

	if resolved == 0 {
		s.logger.Debug(ctx, "dh push: cert ambiguous, marking unmatched",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber))
		s.markUnmatched(ctx, p.ID)
		return 0, processUnmatched
	}

	s.saveCardIDMapping(ctx, p, resolved, mappedSet)
	return resolved, processMatched
}

// saveCardIDMapping persists a DH card ID mapping and updates the in-memory cache.
func (s *DHPushScheduler) saveCardIDMapping(ctx context.Context, p campaigns.Purchase, dhCardID int, mappedSet map[string]string) {
	externalID := strconv.Itoa(dhCardID)
	if saveErr := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); saveErr != nil {
		s.logger.Warn(ctx, "dh push: failed to save external ID mapping",
			observability.String("purchaseID", p.ID), observability.Err(saveErr))
		return
	}
	mappedSet[p.DHCardKey()] = externalID
}

// markUnmatched sets a purchase's DH push status to unmatched.
func (s *DHPushScheduler) markUnmatched(ctx context.Context, purchaseID string) {
	if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, purchaseID, campaigns.DHPushStatusUnmatched); updateErr != nil {
		s.logger.Warn(ctx, "dh push: failed to set unmatched status",
			observability.String("purchaseID", purchaseID), observability.Err(updateErr))
	}
}

// Compile-time checks that dh.Client satisfies the push client interfaces.
var _ DHPushCertResolver = (*dh.Client)(nil)
var _ DHPushInventoryPusher = (*dh.Client)(nil)
