package handlers

import (
	"context"
	"net/http"
	"strconv"

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

	identities, err := h.uniqueCardIdentities(ctx)
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
		h.runBulkMatch(ctx, identities, mappedSet)
	}()
}

// runBulkMatch processes all card identities against DH matching, logging results.
func (h *DHHandler) runBulkMatch(ctx context.Context, identities []campaigns.CardIdentity, mappedSet map[string]string) {
	var matched, skipped, lowConf, failed int

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
	}

	h.logger.Info(ctx, "bulk match completed",
		observability.Int("total", len(identities)),
		observability.Int("matched", matched),
		observability.Int("skipped", skipped),
		observability.Int("low_confidence", lowConf),
		observability.Int("failed", failed))
}

// uniqueCardIdentities returns deduplicated card identities from all unsold purchases.
func (h *DHHandler) uniqueCardIdentities(ctx context.Context) ([]campaigns.CardIdentity, error) {
	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, err
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
	return identities, nil
}
