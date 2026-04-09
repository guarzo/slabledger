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
	h.bulkMatchError.Store("")
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
	// Reset PSA key rotation at the start of each run so a previously exhausted
	// index doesn't prevent newly added keys from being tried.
	var rotateFn func() bool
	if rotator, ok := h.certResolver.(dh.PSAKeyRotator); ok {
		rotator.ResetPSAKeyRotation()
		rotateFn = rotator.RotatePSAKey
	}

	var matched, skipped, noCert, notFound, failed int
	var matchedCards []matchedCard

	for _, p := range purchases {
		if ctx.Err() != nil {
			break
		}

		// Only skip purchases that have already been fully matched and pushed
		// to DH inventory. Purchases with a card_id_mappings entry but still
		// marked "unmatched" should be re-tried.
		if p.DHPushStatus == campaigns.DHPushStatusMatched || p.DHPushStatus == campaigns.DHPushStatusManual {
			skipped++
			continue
		}

		if p.CertNumber == "" {
			noCert++
			continue
		}

		key := p.DHCardKey()

		// If this card identity already has a mapping, skip the API call
		// and reuse the existing DH card ID.
		if existingID := mappedSet[key]; existingID != "" {
			if parsed, parseErr := strconv.Atoi(existingID); parseErr == nil && parsed > 0 {
				matched++
				matchedCards = append(matchedCards, matchedCard{identity: p.ToCardIdentity(), dhCardID: parsed})
				h.logger.Debug(ctx, "bulk match: reusing existing mapping",
					observability.String("cert", p.CertNumber),
					observability.Int("dh_card_id", parsed))
				continue
			}
		}

		cardName, variant := campaigns.CleanCardNameForDH(p.CardName)
		req := dh.CertResolveRequest{
			CertNumber: p.CertNumber,
			GemRateID:  p.GemRateID,
			CardName:   cardName,
			SetName:    p.SetName,
			CardNumber: p.CardNumber,
			Year:       p.CardYear,
			Variant:    variant,
		}
		h.logger.Debug(ctx, "bulk match: resolving cert",
			observability.String("cert", p.CertNumber),
			observability.String("raw_name", p.CardName),
			observability.String("clean_name", cardName),
			observability.String("set_name", p.SetName),
			observability.String("card_number", p.CardNumber),
			observability.String("year", p.CardYear),
			observability.String("variant", variant),
			observability.String("gemrate_id", p.GemRateID))
		resp, err := dh.ResolveCertWithRotation(ctx, req, h.certResolver.ResolveCert, rotateFn, h.logger, "bulk match")
		if err != nil {
			if dh.IsPSARateLimitError(err) {
				// All PSA keys exhausted — abort the entire batch.
				failed++
				errMsg := "DH cert resolution stopped: PSA API daily rate limit reached. All configured PSA keys exhausted."
				h.logger.Error(ctx, errMsg, observability.Err(err))
				h.bulkMatchError.Store(errMsg)
				break
			}
			h.logger.Warn(ctx, "bulk match: DH cert resolve failed",
				observability.String("cert", p.CertNumber), observability.Err(err))
			failed++
			if h.pushStatusUpdater != nil {
				if statusErr := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); statusErr != nil {
					h.logger.Warn(ctx, "bulk match: failed to set unmatched status",
						observability.String("purchaseID", p.ID), observability.Err(statusErr))
				}
			}
			continue
		}

		h.logger.Debug(ctx, "bulk match: cert resolve result",
			observability.String("cert", p.CertNumber),
			observability.String("status", resp.Status),
			observability.Int("dh_card_id", resp.DHCardID),
			observability.Int("candidates", len(resp.Candidates)))

		if resp.Status != dh.CertStatusMatched {
			dhCardID, saveErr := h.resolveAmbiguousMatch(ctx, resp, p, mappedSet)
			if saveErr != nil {
				failed++
				continue
			}
			if dhCardID > 0 {
				matched++
				matchedCards = append(matchedCards, matchedCard{identity: p.ToCardIdentity(), dhCardID: dhCardID})
				continue
			}
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

// resolveAmbiguousMatch attempts to disambiguate an ambiguous cert response by
// card number. Returns the DH card ID on success, or 0 if unresolvable.
// Returns a non-nil error only when the card was resolved but saving failed.
func (h *DHHandler) resolveAmbiguousMatch(ctx context.Context, resp *dh.CertResolution, p campaigns.Purchase, mappedSet map[string]string) (int, error) {
	if resp.Status != dh.CertStatusAmbiguous || len(resp.Candidates) == 0 {
		return 0, nil
	}

	var saveFn func(string) error
	if h.candidatesSaver != nil {
		saveFn = func(j string) error { return h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, p.ID, j) }
	}

	dhCardID, resolveErr := dh.ResolveAmbiguous(resp.Candidates, p.CardNumber, saveFn)
	if resolveErr != nil {
		h.logger.Warn(ctx, "bulk match: failed to save candidates",
			observability.String("cert", p.CertNumber), observability.Err(resolveErr))
	}

	if dhCardID == 0 {
		return 0, nil
	}

	externalID := strconv.Itoa(dhCardID)
	if err := h.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); err != nil {
		h.logger.Error(ctx, "bulk match: save disambiguated ID", observability.Err(err),
			observability.String("cert", p.CertNumber))
		return 0, err
	}

	mappedSet[p.DHCardKey()] = externalID
	return dhCardID, nil
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
		if p.CertNumber == "" || p.DHInventoryID != 0 || campaigns.ResolveMarketValueCents(&p) == 0 || p.BuyCostCents <= 0 {
			continue
		}
		items = append(items, dh.InventoryItem{
			DHCardID:         dhCardID,
			CertNumber:       p.CertNumber,
			GradingCompany:   dh.GraderPSA,
			Grade:            p.GradeValue,
			CostBasisCents:   p.BuyCostCents,
			MarketValueCents: dh.IntPtr(campaigns.ResolveMarketValueCents(&p)),
			Status:           dh.InventoryStatusInStock,
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
