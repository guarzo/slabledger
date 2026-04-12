package dhlisting

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// dhListingService implements DHListingService by coordinating cert resolution,
// inventory push, listing, and persistence operations.
type dhListingService struct {
	purchaseLookup    DHListingPurchaseLookup
	certResolver      DHCertResolver
	pusher            DHInventoryPusher
	lister            DHInventoryLister
	cardIDSaver       DHCardIDSaver
	fieldsUpdater     DHListingFieldsUpdater
	pushStatusUpdater DHListingPushStatusUpdater
	candidatesSaver   DHListingCandidatesSaver
	logger            observability.Logger
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

// DHListingCandidatesSaver stores DH cert resolution candidates on a purchase.
type DHListingCandidatesSaver interface {
	UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error
}

// DHListingServiceOption configures optional dependencies on dhListingService.
type DHListingServiceOption func(*dhListingService)

// WithDHListingCertResolver enables DH cert resolution for inline push.
func WithDHListingCertResolver(c DHCertResolver) DHListingServiceOption {
	return func(s *dhListingService) { s.certResolver = c }
}

// WithDHListingPusher enables inventory push to DH.
func WithDHListingPusher(p DHInventoryPusher) DHListingServiceOption {
	return func(s *dhListingService) { s.pusher = p }
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

// WithDHListingCandidatesSaver enables storing ambiguous DH candidates.
func WithDHListingCandidatesSaver(saver DHListingCandidatesSaver) DHListingServiceOption {
	return func(s *dhListingService) { s.candidatesSaver = saver }
}

// NewDHListingService creates a new DHListingService.
// purchaseLookup and logger are required; all other dependencies are optional.
func NewDHListingService(
	purchaseLookup DHListingPurchaseLookup,
	logger observability.Logger,
	opts ...DHListingServiceOption,
) (DHListingService, error) {
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

// ListPurchases implements DHListingService.
func (s *dhListingService) ListPurchases(ctx context.Context, certNumbers []string) DHListingResult {
	if s.lister == nil || len(certNumbers) == 0 {
		return DHListingResult{}
	}

	purchases, err := s.purchaseLookup.GetPurchasesByCertNumbers(ctx, certNumbers)
	if err != nil {
		s.logger.Warn(ctx, "dh listing: batch cert lookup failed", observability.Err(err))
		return DHListingResult{}
	}

	// Sort cert numbers for deterministic iteration order.
	sortedCerts := make([]string, 0, len(purchases))
	for cn := range purchases {
		sortedCerts = append(sortedCerts, cn)
	}
	sort.Strings(sortedCerts)

	listed, synced := 0, 0
	for _, cn := range sortedCerts {
		p := purchases[cn]
		// If pending DH push, do inline match + push first.
		if p.DHInventoryID == 0 && p.DHPushStatus == inventory.DHPushStatusPending {
			if s.certResolver == nil || s.pusher == nil {
				continue // no DH match client — skip
			}
			invID := s.inlineMatchAndPush(ctx, p)
			if invID == 0 {
				continue // unmatched or failed — skip listing
			}
			p.DHInventoryID = invID
		}

		if p.DHInventoryID == 0 {
			continue // not yet pushed to DH
		}

		if err := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, inventory.DHStatusListed); err != nil {
			s.logger.Warn(ctx, "dh listing: status update failed",
				observability.String("cert", p.CertNumber),
				observability.Int("inventoryID", p.DHInventoryID),
				observability.Err(err))
			continue
		}
		listed++

		defaultChannels := DefaultListingChannels
		if err := s.lister.SyncChannels(ctx, p.DHInventoryID, defaultChannels); err != nil {
			s.logger.Warn(ctx, "dh listing: channel sync failed, reverting to in_stock",
				observability.String("cert", p.CertNumber),
				observability.Int("inventoryID", p.DHInventoryID),
				observability.Err(err))
			// Revert status so the item doesn't stay "listed" without channel sync.
			if revertErr := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, inventory.DHStatusInStock); revertErr != nil {
				s.logger.Error(ctx, "dh listing: failed to revert status after sync failure",
					observability.String("cert", p.CertNumber),
					observability.Int("inventoryID", p.DHInventoryID),
					observability.Err(revertErr))
			} else if s.fieldsUpdater != nil {
				// Persist reverted status locally so readers don't see stale data.
				if persistErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
					DHStatus: inventory.DHStatusInStock,
				}); persistErr != nil {
					s.logger.Warn(ctx, "dh listing: failed to persist reverted status",
						observability.String("cert", p.CertNumber), observability.Err(persistErr))
				}
			}
			listed-- // revert the listed count
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
				listed--
				continue
			}
			if persistErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
				DHStatus:     inventory.DHStatusListed,
				ChannelsJSON: string(channelsJSON),
			}); persistErr != nil {
				s.logger.Error(ctx, "dh listing: failed to persist listed status — decrementing listed count",
					observability.String("cert", p.CertNumber), observability.Err(persistErr))
				listed--
				continue
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

	return DHListingResult{Listed: listed, Synced: synced, Total: len(purchases)}
}

// inlineMatchAndPush resolves a single cert against DH and pushes inventory.
// Returns the inventory ID on success, 0 on failure.
func (s *dhListingService) inlineMatchAndPush(ctx context.Context, p *inventory.Purchase) int {
	if p.CertNumber == "" {
		s.logger.Warn(ctx, "inline dh resolve: purchase has no cert number",
			observability.String("purchaseID", p.ID))
		return 0
	}

	cardName, variant := CleanCardNameForDH(p.CardName)

	resp, err := s.certResolver.ResolveCert(ctx, DHCertResolveRequest{
		CertNumber: p.CertNumber,
		CardName:   cardName,
		SetName:    p.SetName,
		CardNumber: p.CardNumber,
		Year:       p.CardYear,
		Variant:    variant,
	})
	if err != nil {
		s.logger.Warn(ctx, "inline dh cert resolve failed",
			observability.String("cert", p.CertNumber), observability.Err(err))
		return 0
	}

	dhCardID, ok := s.resolveInlineDHCardID(ctx, resp, p)
	if !ok {
		return 0
	}

	if s.cardIDSaver != nil {
		externalID := strconv.Itoa(dhCardID)
		if err := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, SourceDH, externalID); err != nil {
			s.logger.Warn(ctx, "inline dh resolve: failed to save card mapping",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
	}

	marketValue := ResolveMarketValueCents(p)
	if marketValue == 0 {
		return 0
	}

	item := DHInventoryPushItem{
		DHCardID:         dhCardID,
		CertNumber:       p.CertNumber,
		Grade:            p.GradeValue,
		CostBasisCents:   p.BuyCostCents,
		MarketValueCents: marketValue,
	}

	pushResp, pushErr := s.pusher.PushInventory(ctx, []DHInventoryPushItem{item})
	if pushErr != nil {
		s.logger.Warn(ctx, "inline dh push failed",
			observability.String("cert", p.CertNumber), observability.Err(pushErr))
		return 0
	}

	for _, r := range pushResp.Results {
		if r.Status == "failed" || r.DHInventoryID == 0 {
			continue
		}

		if s.fieldsUpdater != nil {
			if err := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
				CardID:            dhCardID,
				InventoryID:       r.DHInventoryID,
				CertStatus:        DHCertStatusMatched,
				ListingPriceCents: r.AssignedPriceCents,
				ChannelsJSON:      r.ChannelsJSON,
				DHStatus:          inventory.DHStatus(r.Status),
			}); err != nil {
				s.logger.Warn(ctx, "inline dh push: failed to persist DH fields",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}

		if s.pushStatusUpdater != nil {
			if err := s.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusMatched); err != nil {
				s.logger.Warn(ctx, "inline dh push: failed to set matched status",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}

		return r.DHInventoryID
	}

	return 0
}

// resolveInlineDHCardID determines the DH card ID from a cert resolution response.
// Returns the card ID and true on success, or 0 and false if unresolvable.
func (s *dhListingService) resolveInlineDHCardID(ctx context.Context, resp *DHCertResolution, p *inventory.Purchase) (int, bool) {
	if resp.Status == DHCertStatusMatched {
		return resp.DHCardID, true
	}

	if resp.Status == DHCertStatusAmbiguous && len(resp.Candidates) > 0 {
		var saveFn func(string) error
		if s.candidatesSaver != nil {
			saveFn = func(j string) error { return s.candidatesSaver.UpdatePurchaseDHCandidates(ctx, p.ID, j) }
		}
		resolved, err := disambiguateCandidates(resp.Candidates, p.CardNumber, saveFn)
		if err != nil {
			s.logger.Warn(ctx, "inline dh resolve: failed to save candidates",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
		if resolved > 0 {
			return resolved, true
		}
	}

	s.markInlineUnmatched(ctx, p, resp.Status)
	return 0, false
}

// markInlineUnmatched sets the push status to unmatched and logs the outcome.
func (s *dhListingService) markInlineUnmatched(ctx context.Context, p *inventory.Purchase, dhStatus string) {
	if s.pushStatusUpdater != nil {
		if err := s.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusUnmatched); err != nil {
			s.logger.Warn(ctx, "inline dh resolve: failed to set unmatched status",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
	}
	s.logger.Warn(ctx, "inline dh cert resolve: unmatched",
		observability.String("cert", p.CertNumber),
		observability.String("dh_status", dhStatus))
}

// disambiguateCandidates tries card-number disambiguation on ambiguous candidates.
// Returns the matched DHCardID (>0) on success. On failure, marshals
// candidates to JSON and passes them to saveFn (if non-nil), then returns 0.
func disambiguateCandidates(candidates []DHCertCandidate, cardNumber string, saveFn func(candidatesJSON string) error) (int, error) {
	if id := disambiguateByCardNumber(candidates, cardNumber); id > 0 {
		return id, nil
	}
	if saveFn != nil {
		b, err := json.Marshal(candidates)
		if err != nil {
			return 0, fmt.Errorf("marshal candidates: %w", err)
		}
		if err := saveFn(string(b)); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// disambiguateByCardNumber selects a single candidate from an ambiguous cert
// resolution using the card_number hint. Returns the matching candidate's
// DHCardID if exactly one candidate matches, or 0 if disambiguation fails.
func disambiguateByCardNumber(candidates []DHCertCandidate, cardNumber string) int {
	normalized := normalizeCardNum(cardNumber)
	if normalized == "" || len(candidates) == 0 {
		return 0
	}

	var matchID int
	matches := 0
	for _, c := range candidates {
		if normalizeCardNum(c.CardNumber) == normalized {
			matchID = c.DHCardID
			matches++
		}
	}

	if matches == 1 {
		return matchID
	}
	return 0
}

// normalizeCardNum strips leading zeros, preserving a single "0" for
// all-zero inputs (e.g. "000" → "0").
func normalizeCardNum(s string) string {
	n := strings.TrimLeft(s, "0")
	if n == "" && len(s) > 0 {
		return "0"
	}
	return n
}
