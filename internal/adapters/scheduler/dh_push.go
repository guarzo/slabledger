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
	pending, err := s.pendingLister.GetPurchasesByDHPushStatus(ctx, campaigns.DHPushStatusPending, dhPushBatchLimit)
	if err != nil {
		s.logger.Warn(ctx, "dh push: failed to list pending purchases", observability.Err(err))
		return
	}

	if len(pending) == 0 {
		s.logger.Debug(ctx, "dh push: no pending purchases")
		return
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

	for _, p := range pending {
		switch s.processPurchase(ctx, p, mappedSet) {
		case processMatched:
			matched++
		case processUnmatched:
			unmatched++
		case processSkipped:
			skipped++
		}
	}

	s.logger.Info(ctx, "dh push completed",
		observability.Int("total", len(pending)),
		observability.Int("matched", matched),
		observability.Int("unmatched", unmatched),
		observability.Int("skipped", skipped),
	)
}

func (s *DHPushScheduler) processPurchase(ctx context.Context, p campaigns.Purchase, mappedSet map[string]string) processResult {
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

	// Attempt to reuse an existing DH card ID mapping.
	dhCardIDStr, alreadyMapped := mappedSet[key]
	var dhCardID int

	if alreadyMapped && dhCardIDStr != "" {
		parsed, err := strconv.Atoi(dhCardIDStr)
		if err != nil || parsed <= 0 {
			alreadyMapped = false
		} else {
			dhCardID = parsed
		}
	} else {
		alreadyMapped = false
	}

	if !alreadyMapped {
		cardName, variant := campaigns.CleanCardNameForDH(p.CardName)
		resp, err := s.certResolver.ResolveCert(ctx, dh.CertResolveRequest{
			CertNumber: p.CertNumber,
			GemRateID:  p.GemRateID,
			CardName:   cardName,
			SetName:    p.SetName,
			CardNumber: p.CardNumber,
			Year:       p.CardYear,
			Variant:    variant,
		})
		if err != nil {
			s.logger.Warn(ctx, "dh push: cert resolve error, leaving as pending",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			return processSkipped
		}

		if resp.Status != dh.CertStatusMatched {
			if resp.Status == dh.CertStatusAmbiguous && len(resp.Candidates) > 0 {
				var saveFn func(string) error
				if s.candidatesSaver != nil {
					saveFn = func(j string) error { return s.candidatesSaver.UpdatePurchaseDHCandidates(ctx, p.ID, j) }
				}
				resolved, resolveErr := dh.ResolveAmbiguous(resp.Candidates, p.CardNumber, saveFn)
				if resolveErr != nil {
					s.logger.Warn(ctx, "dh push: failed to resolve/save candidates, will retry",
						observability.String("purchaseID", p.ID), observability.Err(resolveErr))
					return processSkipped
				}
				if resolved > 0 {
					dhCardID = resolved
					externalID := strconv.Itoa(dhCardID)
					if saveErr := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); saveErr != nil {
						s.logger.Warn(ctx, "dh push: failed to save disambiguated ID",
							observability.String("purchaseID", p.ID), observability.Err(saveErr))
					} else {
						mappedSet[key] = externalID
					}
					// fall through to inventory push
				} else {
					s.logger.Debug(ctx, "dh push: cert ambiguous, marking unmatched",
						observability.String("purchaseID", p.ID),
						observability.String("cert", p.CertNumber))
					if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); updateErr != nil {
						s.logger.Warn(ctx, "dh push: failed to set unmatched status",
							observability.String("purchaseID", p.ID),
							observability.Err(updateErr))
					}
					return processUnmatched
				}
			} else {
				s.logger.Debug(ctx, "dh push: cert not matched, marking unmatched",
					observability.String("purchaseID", p.ID),
					observability.String("cert", p.CertNumber),
					observability.String("status", resp.Status))
				if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); updateErr != nil {
					s.logger.Warn(ctx, "dh push: failed to set unmatched status",
						observability.String("purchaseID", p.ID),
						observability.Err(updateErr))
				}
				return processUnmatched
			}
		} else {
			dhCardID = resp.DHCardID

			externalID := strconv.Itoa(dhCardID)
			if saveErr := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); saveErr != nil {
				s.logger.Warn(ctx, "dh push: failed to save external ID mapping",
					observability.String("purchaseID", p.ID),
					observability.Err(saveErr))
			} else {
				mappedSet[key] = externalID
			}
		}
	}

	item := dh.InventoryItem{
		DHCardID:       dhCardID,
		CertNumber:     p.CertNumber,
		GradingCompany: dh.GraderPSA,
		Grade:          p.GradeValue,
		CostBasisCents: p.CLValueCents,
		Status:         dh.InventoryStatusInStock,
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
		s.logger.Warn(ctx, "dh push: failed to set matched status",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
		return processSkipped
	}

	s.logger.Debug(ctx, "dh push: purchase matched and pushed",
		observability.String("purchaseID", p.ID),
		observability.String("cert", p.CertNumber),
		observability.Int("dhCardID", dhCardID),
		observability.Int("dhInventoryID", result.DHInventoryID),
	)

	return processMatched
}

// Compile-time checks that dh.Client satisfies the push client interfaces.
var _ DHPushCertResolver = (*dh.Client)(nil)
var _ DHPushInventoryPusher = (*dh.Client)(nil)
