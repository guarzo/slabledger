package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// unmatchedResponse is the JSON shape returned by HandleUnmatched.
type unmatchedResponse struct {
	Unmatched []unmatchedCard `json:"unmatched"`
	Count     int             `json:"count"`
}

type unmatchedCard struct {
	PurchaseID   string                       `json:"purchase_id"`
	CardName     string                       `json:"card_name"`
	SetName      string                       `json:"set_name"`
	CardNumber   string                       `json:"card_number"`
	CertNumber   string                       `json:"cert_number"`
	Grade        float64                      `json:"grade"`
	CLValueCents int                          `json:"cl_value_cents"`
	Candidates   []dh.CertResolutionCandidate `json:"candidates,omitempty"`
}

// HandleUnmatched returns cards that do not yet have a DH mapping.
func (h *DHHandler) HandleUnmatched(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "unmatched: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	var unmatched []unmatchedCard
	for _, p := range purchases {
		if p.DHPushStatus != campaigns.DHPushStatusUnmatched {
			continue
		}
		card := unmatchedCard{
			PurchaseID:   p.ID,
			CardName:     p.CardName,
			SetName:      p.SetName,
			CardNumber:   p.CardNumber,
			CertNumber:   p.CertNumber,
			Grade:        p.GradeValue,
			CLValueCents: p.CLValueCents,
		}
		if p.DHCandidatesJSON != "" {
			var raw []dh.CertResolutionCandidate
			if err := json.Unmarshal([]byte(p.DHCandidatesJSON), &raw); err != nil {
				h.logger.Warn(ctx, "unmatched: failed to parse candidates JSON",
					observability.String("purchaseID", p.ID), observability.Err(err))
			} else {
				card.Candidates = raw
			}
		}
		unmatched = append(unmatched, card)
	}

	if unmatched == nil {
		unmatched = []unmatchedCard{}
	}
	writeJSON(w, http.StatusOK, unmatchedResponse{Unmatched: unmatched, Count: len(unmatched)})
}
