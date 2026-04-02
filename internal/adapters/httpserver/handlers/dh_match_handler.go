package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// bulkMatchResponse is the JSON shape returned by HandleBulkMatch.
type bulkMatchResponse struct {
	Total         int `json:"total"`
	Matched       int `json:"matched"`
	Skipped       int `json:"skipped"`
	LowConfidence int `json:"low_confidence"`
	Failed        int `json:"failed"`
}

// HandleBulkMatch matches all unmatched inventory cards against the DH catalog.
func (h *DHHandler) HandleBulkMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	identities, err := h.uniqueCardIdentities(ctx)
	if err != nil {
		h.logger.Error(ctx, "bulk match: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	// Pre-load all existing DH mappings in a single query.
	mappedSet, err := h.cardIDSaver.GetMappedSet(ctx, pricing.SourceDH)
	if err != nil {
		h.logger.Error(ctx, "bulk match: load mapped set", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load mappings")
		return
	}

	result := bulkMatchResponse{Total: len(identities)}

	for _, ci := range identities {
		if ctx.Err() != nil {
			break
		}

		if mappedSet[dhCardKey(ci.CardName, ci.SetName, ci.CardNumber)] != "" {
			result.Skipped++
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
			result.Failed++
			continue
		}

		if !matchResp.Success || matchResp.Confidence < 0.90 {
			result.LowConfidence++
			continue
		}

		externalID := strconv.Itoa(matchResp.CardID)
		if err := h.cardIDSaver.SaveExternalID(ctx, ci.CardName, ci.SetName, ci.CardNumber, pricing.SourceDH, externalID); err != nil {
			h.logger.Error(ctx, "bulk match: save external ID", observability.Err(err),
				observability.String("card", ci.CardName))
			result.Failed++
			continue
		}
		result.Matched++
	}

	writeJSON(w, http.StatusOK, result)
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
