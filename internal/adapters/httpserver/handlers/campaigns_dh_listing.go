package handlers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// triggerDHListing runs in the background so it doesn't delay the HTTP response.
func (h *CampaignsHandler) triggerDHListing(certNumbers []string) {
	if h.dhLister == nil || len(certNumbers) == 0 {
		return
	}

	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error(h.baseCtx, "panic in triggerDHListing",
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		ctx, cancel := context.WithTimeout(h.baseCtx, 5*time.Minute)
		defer cancel()

		purchases, err := h.service.GetPurchasesByCertNumbers(ctx, certNumbers)
		if err != nil {
			h.logger.Warn(ctx, "dh listing: batch cert lookup failed", observability.Err(err))
			return
		}

		listed, synced := 0, 0
		for _, p := range purchases {
			// If pending DH push, do inline match + push first
			if p.DHInventoryID == 0 && p.DHPushStatus == campaigns.DHPushStatusPending {
				if h.dhCertResolver == nil || h.dhPusher == nil {
					continue // no DH match client — skip
				}
				invID := h.inlineMatchAndPush(ctx, p)
				if invID == 0 {
					continue // unmatched or failed — skip listing
				}
				p.DHInventoryID = invID
			}

			if p.DHInventoryID == 0 {
				continue // not yet pushed to DH
			}

			_, err := h.dhLister.UpdateInventory(ctx, p.DHInventoryID, dh.InventoryUpdate{
				Status: dh.InventoryStatusListed,
			})
			if err != nil {
				h.logger.Warn(ctx, "dh listing: status update failed",
					observability.String("cert", p.CertNumber),
					observability.Int("inventoryID", p.DHInventoryID),
					observability.Err(err))
				continue
			}
			listed++

			_, err = h.dhLister.SyncChannels(ctx, p.DHInventoryID, []string{dh.ChannelEbay, dh.ChannelShopify})
			if err != nil {
				h.logger.Warn(ctx, "dh listing: channel sync failed, reverting to in_stock",
					observability.String("cert", p.CertNumber),
					observability.Int("inventoryID", p.DHInventoryID),
					observability.Err(err))
				// Revert status so the item doesn't stay "listed" without channel sync
				if _, revertErr := h.dhLister.UpdateInventory(ctx, p.DHInventoryID, dh.InventoryUpdate{
					Status: dh.InventoryStatusInStock,
				}); revertErr != nil {
					h.logger.Error(ctx, "dh listing: failed to revert status after sync failure",
						observability.String("cert", p.CertNumber),
						observability.Int("inventoryID", p.DHInventoryID),
						observability.Err(revertErr))
				}
				listed-- // revert the listed count
				continue
			}
			synced++
		}

		if listed > 0 || synced > 0 {
			h.logger.Info(ctx, "dh listing completed",
				observability.Int("listed", listed),
				observability.Int("synced", synced),
				observability.Int("certs", len(certNumbers)))
		} else if len(purchases) > 0 {
			h.logger.Warn(ctx, "dh listing completed with no successful operations",
				observability.Int("certs", len(certNumbers)))
		}
	}()
}

// inlineMatchAndPush resolves a single cert against DH and pushes inventory.
// Returns the inventory ID on success, 0 on failure.
func (h *CampaignsHandler) inlineMatchAndPush(ctx context.Context, p *campaigns.Purchase) int {
	if p.CertNumber == "" {
		h.logger.Warn(ctx, "inline dh resolve: purchase has no cert number",
			observability.String("purchaseID", p.ID))
		return 0
	}

	cardName, variant := campaigns.CleanCardNameForDH(p.CardName)

	resp, err := h.dhCertResolver.ResolveCert(ctx, dh.CertResolveRequest{
		CertNumber: p.CertNumber,
		CardName:   cardName,
		SetName:    p.SetName,
		CardNumber: p.CardNumber,
		Year:       p.CardYear,
		Variant:    variant,
	})
	if err != nil {
		h.logger.Warn(ctx, "inline dh cert resolve failed",
			observability.String("cert", p.CertNumber), observability.Err(err))
		return 0
	}

	dhCardID, ok := h.resolveInlineDHCardID(ctx, resp, p)
	if !ok {
		return 0
	}

	if h.dhCardIDSaver != nil {
		externalID := strconv.Itoa(dhCardID)
		if err := h.dhCardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); err != nil {
			h.logger.Warn(ctx, "inline dh resolve: failed to save card mapping",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
	}

	marketValue := campaigns.ResolveMarketValueCents(p)
	if marketValue == 0 {
		return 0
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

	pushResp, pushErr := h.dhPusher.PushInventory(ctx, []dh.InventoryItem{item})
	if pushErr != nil {
		h.logger.Warn(ctx, "inline dh push failed",
			observability.String("cert", p.CertNumber), observability.Err(pushErr))
		return 0
	}

	for _, r := range pushResp.Results {
		if r.Status == "failed" || r.DHInventoryID == 0 {
			continue
		}

		if h.dhFieldsUpdater != nil {
			if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, campaigns.DHFieldsUpdate{
				CardID:            dhCardID,
				InventoryID:       r.DHInventoryID,
				CertStatus:        dh.CertStatusMatched,
				ListingPriceCents: r.AssignedPriceCents,
				ChannelsJSON:      dh.MarshalChannels(r.Channels),
				DHStatus:          campaigns.DHStatus(r.Status),
			}); err != nil {
				h.logger.Warn(ctx, "inline dh push: failed to persist DH fields",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}

		if h.pushStatusUpdater != nil {
			if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusMatched); err != nil {
				h.logger.Warn(ctx, "inline dh push: failed to set matched status",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}

		return r.DHInventoryID
	}

	return 0
}

// resolveInlineDHCardID determines the DH card ID from a cert resolution response.
// Returns the card ID and true on success, or 0 and false if unresolvable.
func (h *CampaignsHandler) resolveInlineDHCardID(ctx context.Context, resp *dh.CertResolution, p *campaigns.Purchase) (int, bool) {
	if resp.Status == dh.CertStatusMatched {
		return resp.DHCardID, true
	}

	if resp.Status == dh.CertStatusAmbiguous && len(resp.Candidates) > 0 {
		var saveFn func(string) error
		if h.dhCandidatesSaver != nil {
			saveFn = func(j string) error { return h.dhCandidatesSaver.UpdatePurchaseDHCandidates(ctx, p.ID, j) }
		}
		resolved, err := dh.ResolveAmbiguous(resp.Candidates, p.CardNumber, saveFn)
		if err != nil {
			h.logger.Warn(ctx, "inline dh resolve: failed to save candidates",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
		if resolved > 0 {
			return resolved, true
		}
	}

	h.markInlineUnmatched(ctx, p, resp.Status)
	return 0, false
}

// markInlineUnmatched sets the push status to unmatched and logs the outcome.
func (h *CampaignsHandler) markInlineUnmatched(ctx context.Context, p *campaigns.Purchase, dhStatus string) {
	if h.pushStatusUpdater != nil {
		if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); err != nil {
			h.logger.Warn(ctx, "inline dh resolve: failed to set unmatched status",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
	}
	h.logger.Warn(ctx, "inline dh cert resolve: unmatched",
		observability.String("cert", p.CertNumber),
		observability.String("dh_status", dhStatus))
}
