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

	// Gather identities and mappings synchronously so we can report errors to the caller.
	ctx := r.Context()

	purchases, identities, err := h.uniqueCardIdentities(ctx)
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

	// Run the actual matching in the background using a context derived from the server lifecycle.
	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer h.bulkMatchMu.Unlock()
		defer h.bulkMatchRunning.Store(false)
		ctx, cancel := context.WithCancel(h.baseCtx)
		defer cancel()
		h.runBulkMatch(ctx, purchases, identities, mappedSet)
	}()
}

type matchedCard struct {
	identity campaigns.CardIdentity
	dhCardID int
}

// runBulkMatch processes all card identities against DH matching, logging results.
func (h *DHHandler) runBulkMatch(ctx context.Context, purchases []campaigns.Purchase, identities []campaigns.CardIdentity, mappedSet map[string]string) {
	var matched, skipped, lowConf, failed int
	var matchedCards []matchedCard

	for _, ci := range identities {
		if ctx.Err() != nil {
			break
		}

		if mappedSet[dhCardKey(ci.CardName, ci.SetName, ci.CardNumber)] != "" {
			skipped++
			continue
		}

		title := ci.PSAListingTitle
		if title == "" {
			title = buildMatchTitle(ci.CardName, ci.SetName, ci.CardNumber)
		}

		matchResp, err := h.matchClient.Match(ctx, title, "")
		if err != nil {
			h.logger.Warn(ctx, "bulk match: DH match failed",
				observability.String("card", ci.CardName), observability.Err(err))
			failed++
			continue
		}

		if !matchResp.Success || matchResp.Confidence < 0.90 {
			lowConf++
			continue
		}

		externalID := strconv.Itoa(matchResp.CardID)
		if err := h.cardIDSaver.SaveExternalID(ctx, ci.CardName, ci.SetName, ci.CardNumber, pricing.SourceDH, externalID); err != nil {
			h.logger.Error(ctx, "bulk match: save external ID", observability.Err(err),
				observability.String("card", ci.CardName))
			failed++
			continue
		}
		matched++
		matchedCards = append(matchedCards, matchedCard{identity: ci, dhCardID: matchResp.CardID})
	}

	h.logger.Info(ctx, "bulk match completed",
		observability.Int("total", len(identities)),
		observability.Int("matched", matched),
		observability.Int("skipped", skipped),
		observability.Int("low_confidence", lowConf),
		observability.Int("failed", failed))

	// Push newly matched cards to DH inventory as in_stock.
	if h.inventoryPusher != nil && len(matchedCards) > 0 {
		h.pushMatchedToDH(ctx, purchases, matchedCards)
	}
}

// pushMatchedToDH uses DH card IDs from the match loop because Purchase.DHCardID
// isn't populated yet — that happens later via the inventory poll scheduler.
func (h *DHHandler) pushMatchedToDH(ctx context.Context, purchases []campaigns.Purchase, matched []matchedCard) {
	dhCardIDs := make(map[string]int, len(matched))
	for _, mc := range matched {
		dhCardIDs[dhCardKey(mc.identity.CardName, mc.identity.SetName, mc.identity.CardNumber)] = mc.dhCardID
	}

	var items []dh.InventoryItem
	for _, p := range purchases {
		key := dhCardKey(p.CardName, p.SetName, p.CardNumber)
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
			CostBasisCents: p.BuyCostCents,
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

	pushed := 0
	for _, r := range resp.Results {
		if r.Status != "failed" {
			pushed++
		}
	}
	h.logger.Info(ctx, "pushed matched inventory to DH",
		observability.Int("pushed", pushed),
		observability.Int("total", len(items)))
}

// uniqueCardIdentities returns all unsold purchases and their deduplicated card identities.
func (h *DHHandler) uniqueCardIdentities(ctx context.Context) ([]campaigns.Purchase, []campaigns.CardIdentity, error) {
	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, nil, err
	}

	seen := make(map[string]bool, len(purchases))
	var identities []campaigns.CardIdentity
	for _, p := range purchases {
		key := dhCardKey(p.CardName, p.SetName, p.CardNumber)
		if seen[key] {
			continue
		}
		seen[key] = true
		identities = append(identities, p.ToCardIdentity())
	}
	return purchases, identities, nil
}
