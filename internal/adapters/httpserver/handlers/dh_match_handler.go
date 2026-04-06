package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// HandleBulkMatch kicks off an async bulk match of unmatched inventory cards against the DH catalog.
// Returns 202 immediately; progress is visible via the GET /api/dh/status endpoint.
func (h *DHHandler) HandleBulkMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	if !h.bulkMatchMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "already_running"})
		return
	}

	ctx := r.Context()

	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.bulkMatchMu.Unlock()
		h.logger.Error(ctx, "bulk match: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	mappedSet, err := h.cardIDSaver.GetMappedSet(ctx, pricing.SourceDH)
	if err != nil {
		h.bulkMatchMu.Unlock()
		h.logger.Error(ctx, "bulk match: load mapped set", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load mappings")
		return
	}

	h.bulkMatchRunning.Store(true)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})

	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer h.bulkMatchMu.Unlock()
		defer h.bulkMatchRunning.Store(false)
		ctx, cancel := context.WithCancel(h.baseCtx)
		defer cancel()
		h.runBulkMatch(ctx, purchases, mappedSet)
	}()
}

type matchedCard struct {
	identity campaigns.CardIdentity
	dhCardID int
}

// runBulkMatch processes unsold purchases against DH cert resolution, logging results.
func (h *DHHandler) runBulkMatch(ctx context.Context, purchases []campaigns.Purchase, mappedSet map[string]string) {
	var matched, skipped, noCert, notFound, failed int
	var matchedCards []matchedCard

	for _, p := range purchases {
		if ctx.Err() != nil {
			break
		}

		key := p.DHCardKey()

		if mappedSet[key] != "" {
			skipped++
			continue
		}

		if p.CertNumber == "" {
			noCert++
			continue
		}

		cardName, variant := campaigns.CleanCardNameForDH(p.CardName)
		resp, err := h.certResolver.ResolveCert(ctx, dh.CertResolveRequest{
			CertNumber: p.CertNumber,
			CardName:   cardName,
			SetName:    p.SetName,
			CardNumber: p.CardNumber,
			Year:       p.CardYear,
			Variant:    variant,
		})
		if err != nil {
			h.logger.Warn(ctx, "bulk match: DH cert resolve failed",
				observability.String("cert", p.CertNumber), observability.Err(err))
			failed++
			continue
		}

		if resp.Status != dh.CertStatusMatched {
			notFound++
			if h.pushStatusUpdater != nil {
				if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); err != nil {
					h.logger.Warn(ctx, "bulk match: failed to set unmatched status",
						observability.String("purchaseID", p.ID), observability.Err(err))
				}
			}
			continue
		}

		externalID := strconv.Itoa(resp.DHCardID)
		if err := h.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); err != nil {
			h.logger.Error(ctx, "bulk match: save external ID", observability.Err(err),
				observability.String("cert", p.CertNumber))
			failed++
			continue
		}
		matched++
		matchedCards = append(matchedCards, matchedCard{identity: p.ToCardIdentity(), dhCardID: resp.DHCardID})
		mappedSet[key] = externalID
	}

	h.logger.Info(ctx, "bulk match completed",
		observability.Int("total", len(purchases)),
		observability.Int("matched", matched),
		observability.Int("skipped", skipped),
		observability.Int("no_cert", noCert),
		observability.Int("not_found", notFound),
		observability.Int("failed", failed))

	if h.inventoryPusher != nil && len(matchedCards) > 0 {
		h.pushMatchedToDH(ctx, purchases, matchedCards)
	}
}

// pushMatchedToDH uses DH card IDs from the match loop because Purchase.DHCardID
// isn't populated yet — that happens later via the inventory poll scheduler.
func (h *DHHandler) pushMatchedToDH(ctx context.Context, purchases []campaigns.Purchase, matched []matchedCard) {
	dhCardIDs := make(map[string]int, len(matched))
	for _, mc := range matched {
		dhCardIDs[campaigns.DHCardKey(mc.identity.CardName, mc.identity.SetName, mc.identity.CardNumber)] = mc.dhCardID
	}

	var items []dh.InventoryItem
	for _, p := range purchases {
		key := campaigns.DHCardKey(p.CardName, p.SetName, p.CardNumber)
		dhCardID, ok := dhCardIDs[key]
		if !ok {
			continue
		}
		if p.CertNumber == "" || p.DHInventoryID != 0 {
			continue
		}
		items = append(items, dh.InventoryItem{
			DHCardID:       dhCardID,
			CertNumber:     p.CertNumber,
			GradingCompany: dh.GraderPSA,
			Grade:          p.GradeValue,
			CostBasisCents: p.CLValueCents,
			Status:         dh.InventoryStatusInStock,
		})
	}

	if len(items) == 0 {
		return
	}

	resp, err := h.inventoryPusher.PushInventory(ctx, items)
	if err != nil {
		h.logger.Error(ctx, "push to DH failed",
			observability.Int("items", len(items)), observability.Err(err))
		return
	}

	// Build lookups for persisting push results back to local purchases.
	certToPurchaseID := make(map[string]string, len(purchases))
	for _, p := range purchases {
		if p.CertNumber != "" {
			certToPurchaseID[p.CertNumber] = p.ID
		}
	}
	certToDHCardID := make(map[string]int, len(items))
	for _, item := range items {
		certToDHCardID[item.CertNumber] = item.DHCardID
	}

	pushed, failedPush := 0, 0
	for _, r := range resp.Results {
		if r.Status == "failed" {
			failedPush++
			h.logger.Warn(ctx, "push to DH: item failed",
				observability.String("cert", r.CertNumber),
				observability.String("error", r.Error))
			continue
		}
		pushed++

		// Persist the DH fields so duplicate pushes are prevented (DHInventoryID != 0)
		// and cert import can immediately list this card.
		if h.dhFieldsUpdater != nil && r.DHInventoryID != 0 && r.CertNumber != "" {
			purchaseID, ok := certToPurchaseID[r.CertNumber]
			if !ok {
				continue
			}
			if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, purchaseID, campaigns.DHFieldsUpdate{
				CardID:            certToDHCardID[r.CertNumber],
				InventoryID:       r.DHInventoryID,
				CertStatus:        dh.CertStatusMatched,
				ListingPriceCents: r.AssignedPriceCents,
				ChannelsJSON:      dh.MarshalChannels(r.Channels),
				DHStatus:          campaigns.DHStatus(r.Status),
			}); err != nil {
				h.logger.Warn(ctx, "push to DH: failed to persist inventory ID",
					observability.String("cert", r.CertNumber),
					observability.Int("inventoryID", r.DHInventoryID),
					observability.Err(err))
			}
			if h.pushStatusUpdater != nil {
				if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchaseID, campaigns.DHPushStatusMatched); err != nil {
					h.logger.Warn(ctx, "push to DH: failed to set matched status",
						observability.String("purchaseID", purchaseID), observability.Err(err))
				}
			}
		}
	}
	h.logger.Info(ctx, "pushed matched inventory to DH",
		observability.Int("pushed", pushed),
		observability.Int("failed", failedPush),
		observability.Int("total", len(items)))
}
