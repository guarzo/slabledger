package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

const dhPushBatchLimit = 50
const dhPushConfidenceThreshold = 0.90

// DHPushPendingLister returns purchases pending DH push.
type DHPushPendingLister interface {
	GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error)
}

// DHPushStatusUpdater updates the DH push status on a purchase.
type DHPushStatusUpdater interface {
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
}

// DHPushMatchClient matches cards against DH catalog.
type DHPushMatchClient interface {
	Match(ctx context.Context, title, sku string) (*dh.MatchResponse, error)
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

// DHPushConfig controls the DH push scheduler.
type DHPushConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHPushScheduler matches pending purchases against DH and pushes inventory.
type DHPushScheduler struct {
	StopHandle
	pendingLister  DHPushPendingLister
	statusUpdater  DHPushStatusUpdater
	matchClient    DHPushMatchClient
	inventoryPush  DHPushInventoryPusher
	fieldsUpdater  DHFieldsUpdater
	cardIDSaver    DHPushCardIDSaver
	logger         observability.Logger
	config         DHPushConfig
}

// NewDHPushScheduler creates a new DH push scheduler.
func NewDHPushScheduler(
	pendingLister DHPushPendingLister,
	statusUpdater DHPushStatusUpdater,
	matchClient DHPushMatchClient,
	inventoryPush DHPushInventoryPusher,
	fieldsUpdater DHFieldsUpdater,
	cardIDSaver DHPushCardIDSaver,
	logger observability.Logger,
	config DHPushConfig,
) *DHPushScheduler {
	if config.Interval <= 0 {
		config.Interval = 5 * time.Minute
	}
	return &DHPushScheduler{
		StopHandle:    NewStopHandle(),
		pendingLister: pendingLister,
		statusUpdater: statusUpdater,
		matchClient:   matchClient,
		inventoryPush: inventoryPush,
		fieldsUpdater: fieldsUpdater,
		cardIDSaver:   cardIDSaver,
		logger:        logger.With(context.Background(), observability.String("component", "dh-push")),
		config:        config,
	}
}

// Start begins the DH push loop.
func (s *DHPushScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
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
		result := s.processPurchase(ctx, p, mappedSet)
		switch result {
		case "matched":
			matched++
		case "unmatched":
			unmatched++
		default:
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

// processPurchase handles a single pending purchase. Returns "matched", "unmatched", or "skipped".
func (s *DHPushScheduler) processPurchase(ctx context.Context, p campaigns.Purchase, mappedSet map[string]string) string {
	key := dhPushCardKey(p.CardName, p.SetName, p.CardNumber)

	// Attempt to reuse an existing DH card ID mapping.
	dhCardIDStr, alreadyMapped := mappedSet[key]
	var dhCardID int

	if alreadyMapped && dhCardIDStr != "" {
		if _, err := fmt.Sscanf(dhCardIDStr, "%d", &dhCardID); err != nil || dhCardID <= 0 {
			alreadyMapped = false
		}
	} else {
		alreadyMapped = false
	}

	if !alreadyMapped {
		// Call DH Match API.
		title := dhPushMatchTitle(p)
		resp, err := s.matchClient.Match(ctx, title, p.CertNumber)
		if err != nil {
			s.logger.Warn(ctx, "dh push: match API error, leaving as pending",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			return "skipped"
		}

		if !resp.Success || resp.Confidence < dhPushConfidenceThreshold {
			s.logger.Debug(ctx, "dh push: low confidence match, marking unmatched",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.Float64("confidence", resp.Confidence))
			if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); updateErr != nil {
				s.logger.Warn(ctx, "dh push: failed to set unmatched status",
					observability.String("purchaseID", p.ID),
					observability.Err(updateErr))
			}
			return "unmatched"
		}

		dhCardID = resp.CardID

		// Persist the mapping so future runs skip the Match call.
		externalID := fmt.Sprintf("%d", dhCardID)
		if saveErr := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); saveErr != nil {
			s.logger.Warn(ctx, "dh push: failed to save external ID mapping",
				observability.String("purchaseID", p.ID),
				observability.Err(saveErr))
			// Non-fatal: continue with the push.
		} else {
			mappedSet[key] = externalID
		}
	}

	// Push to DH inventory.
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
		return "skipped"
	}

	if len(pushResp.Results) == 0 {
		s.logger.Warn(ctx, "dh push: inventory push returned empty results",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber))
		return "skipped"
	}

	result := pushResp.Results[0]

	channelsJSON := dhPushMarshalChannels(result.Channels)

	update := campaigns.DHFieldsUpdate{
		CardID:            dhCardID,
		InventoryID:       result.DHInventoryID,
		CertStatus:        dh.CertStatusMatched,
		ListingPriceCents: result.AssignedPriceCents,
		ChannelsJSON:      channelsJSON,
		DHStatus:          campaigns.DHStatus(result.Status),
	}

	if updateErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, update); updateErr != nil {
		s.logger.Warn(ctx, "dh push: failed to update DH fields",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
		return "skipped"
	}

	if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusMatched); updateErr != nil {
		s.logger.Warn(ctx, "dh push: failed to set matched status",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
		return "skipped"
	}

	s.logger.Debug(ctx, "dh push: purchase matched and pushed",
		observability.String("purchaseID", p.ID),
		observability.String("cert", p.CertNumber),
		observability.Int("dhCardID", dhCardID),
		observability.Int("dhInventoryID", result.DHInventoryID),
	)

	return "matched"
}

// dhPushCardKey builds the pipe-delimited identity key for a card.
func dhPushCardKey(cardName, setName, cardNumber string) string {
	return cardName + "|" + setName + "|" + cardNumber
}

// dhPushMatchTitle returns the best title to use for DH matching.
// If PSAListingTitle is set, it is used directly; otherwise the card name, set, and number are concatenated.
func dhPushMatchTitle(p campaigns.Purchase) string {
	if p.PSAListingTitle != "" {
		return p.PSAListingTitle
	}
	parts := []string{p.CardName}
	if p.SetName != "" {
		parts = append(parts, p.SetName)
	}
	if p.CardNumber != "" {
		parts = append(parts, p.CardNumber)
	}
	return strings.Join(parts, " ")
}

// dhPushMarshalChannels serializes channel statuses to JSON, defaulting to "[]".
func dhPushMarshalChannels(channels []dh.InventoryChannelStatus) string {
	if len(channels) == 0 {
		return "[]"
	}
	b, err := json.Marshal(channels)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// Compile-time checks that dh.Client satisfies the push client interfaces.
var _ DHPushMatchClient = (*dh.Client)(nil)
var _ DHPushInventoryPusher = (*dh.Client)(nil)
