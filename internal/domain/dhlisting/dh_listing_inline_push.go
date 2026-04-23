package dhlisting

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

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
			s.logger.Error(ctx, "inline dh resolve: failed to save card mapping — cert resolver will be called again next run",
				observability.String("cert", p.CertNumber), observability.String("cardName", p.CardName), observability.Err(err))
		}
	}

	if s.gemRateIDUpdater != nil && resp.GemRateID != "" && p.GemRateID == "" {
		if err := s.gemRateIDUpdater.UpdatePurchaseGemRateID(ctx, p.ID, resp.GemRateID); err != nil {
			s.logger.Error(ctx, "inline dh resolve: failed to save gem_rate_id",
				observability.String("cert", p.CertNumber), observability.String("gemRateID", resp.GemRateID), observability.Err(err))
		} else {
			p.GemRateID = resp.GemRateID
		}
	}

	// Reviewed price becomes the DH listing_price_cents preset. 0 means omit —
	// DH uses catalog fallback. We don't gate the push on having a price:
	// items land in_stock regardless, and the list transition (gated elsewhere)
	// re-sends the price when the user clicks List on DH.
	listingPrice := ResolveListingPriceCents(p)

	item := DHInventoryPushItem{
		DHCardID:          dhCardID,
		CertNumber:        p.CertNumber,
		Grade:             p.GradeValue,
		CostBasisCents:    p.BuyCostCents,
		ListingPriceCents: listingPrice,
		CertImageURLFront: p.FrontImageURL,
		CertImageURLBack:  p.BackImageURL,
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
				s.logger.Error(ctx, "inline dh push: failed to persist DH fields — returning 0 to prevent duplicate push",
					observability.String("cert", p.CertNumber), observability.Err(err))
				return 0
			}
		}

		if s.pushStatusUpdater != nil {
			if err := s.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusMatched); err != nil {
				s.logger.Warn(ctx, "inline dh push: failed to set matched status",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}

		s.recordEvent(ctx, dhevents.Event{
			PurchaseID:    p.ID,
			CertNumber:    p.CertNumber,
			Type:          dhevents.TypePushed,
			NewPushStatus: string(inventory.DHPushStatusMatched),
			NewDHStatus:   r.Status,
			DHInventoryID: r.DHInventoryID,
			DHCardID:      dhCardID,
			Source:        dhevents.SourceDHListing,
		})

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
	s.recordEvent(ctx, dhevents.Event{
		PurchaseID:    p.ID,
		CertNumber:    p.CertNumber,
		Type:          dhevents.TypeUnmatched,
		NewPushStatus: string(inventory.DHPushStatusUnmatched),
		Notes:         dhStatus,
		Source:        dhevents.SourceDHListing,
	})
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
