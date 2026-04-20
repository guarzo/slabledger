package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

type retryMatchRequest struct {
	PurchaseID string `json:"purchaseId"`
}

type retryMatchResponse struct {
	Status        string `json:"status"`
	DHCardID      int    `json:"dhCardId"`
	DHInventoryID int    `json:"dhInventoryId"`
}

// HandleRetryMatch re-runs the full DH match pipeline for a single unmatched
// purchase: ResolveCert first, then PSAImport as fallback if cert not found.
func (h *DHHandler) HandleRetryMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	var req retryMatchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.PurchaseID == "" {
		writeError(w, http.StatusBadRequest, "purchaseId is required")
		return
	}

	purchase, err := h.purchaseLister.GetPurchase(ctx, req.PurchaseID)
	if err != nil {
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "purchase not found")
			return
		}
		h.logger.Error(ctx, "retry match: get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to look up purchase")
		return
	}

	if purchase.DHPushStatus != inventory.DHPushStatusUnmatched {
		writeError(w, http.StatusBadRequest, "purchase is not in unmatched status")
		return
	}

	// Step 1: attempt standard cert resolution.
	if h.certResolver != nil && purchase.CertNumber != "" {
		resolution, resolveErr := h.certResolver.ResolveCert(ctx, dh.CertResolveRequest{
			CertNumber: purchase.CertNumber,
			CardName:   purchase.CardName,
			SetName:    purchase.SetName,
			CardNumber: purchase.CardNumber,
		})
		if resolveErr != nil {
			h.logger.Error(ctx, "retry match: cert resolver error", observability.Err(resolveErr))
			writeError(w, http.StatusBadGateway, "cert resolver failed")
			return
		}
		switch resolution.Status {
		case dh.CertStatusMatched:
			dhCardID := resolution.DHCardID
			if h.cardIDSaver != nil {
				if err := h.cardIDSaver.SaveExternalID(ctx, purchase.CardName, purchase.SetName, purchase.CardNumber, pricing.SourceDH, fmt.Sprintf("%d", dhCardID)); err != nil {
					h.logger.Warn(ctx, "retry match: save external card ID", observability.Err(err),
						observability.String("cardName", purchase.CardName), observability.String("setName", purchase.SetName))
				}
			}
			listingPrice := dhlisting.ResolveListingPriceCents(purchase)
			inventoryID, pushErr := h.pushAndPersistDH(ctx, purchase, dhCardID, listingPrice)
			if pushErr != nil {
				h.logger.Error(ctx, "retry match: push inventory after cert resolve", observability.Err(pushErr))
				writeError(w, http.StatusBadGateway, "DH push failed")
				return
			}
			if h.pushStatusUpdater != nil {
				if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, inventory.DHPushStatusMatched); err != nil {
					h.logger.Error(ctx, "retry match: update push status", observability.Err(err))
					writeError(w, http.StatusInternalServerError, "failed to update push status")
					return
				}
			}
			if h.candidatesSaver != nil {
				if err := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, ""); err != nil {
					h.logger.Warn(ctx, "retry match: clear candidates after cert resolve", observability.Err(err),
						observability.String("purchaseID", purchase.ID))
				}
			}
			writeJSON(w, http.StatusOK, retryMatchResponse{Status: "ok", DHCardID: dhCardID, DHInventoryID: inventoryID})
			return

		case dh.CertStatusAmbiguous:
			if len(resolution.Candidates) > 0 {
				// Save fresh candidates so the user can pick via Select.
				candidatesSaved := false
				if h.candidatesSaver != nil {
					candidatesJSON, marshalErr := json.Marshal(resolution.Candidates)
					if marshalErr != nil {
						h.logger.Warn(ctx, "retry match: failed to marshal candidates", observability.Err(marshalErr))
					} else if saveErr := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, string(candidatesJSON)); saveErr != nil {
						h.logger.Warn(ctx, "retry match: failed to persist candidates", observability.Err(saveErr))
					} else {
						candidatesSaved = true
					}
				}
				if candidatesSaved {
					writeError(w, http.StatusUnprocessableEntity, "ambiguous match — candidates updated, use Select to pick one")
				} else {
					writeError(w, http.StatusUnprocessableEntity, "ambiguous match — use Select to pick one")
				}
				return
			}
			// Ambiguous with no candidates — fall through to PSA import.
		}
		// CertStatusNotFound or ambiguous with no candidates: fall through.
	}

	// Step 2: PSA import fallback.
	if h.psaImporter == nil {
		writeError(w, http.StatusUnprocessableEntity, "PSA import not available")
		return
	}

	lang := dhlisting.InferDHLanguage(purchase.SetName, purchase.CardName)
	item := dh.PSAImportItem{
		CertNumber:     purchase.CertNumber,
		CostBasisCents: purchase.BuyCostCents,
		Status:         "in_stock",
		Overrides: &dh.PSAImportOverrides{
			Name:       purchase.CardName,
			SetName:    purchase.SetName,
			CardNumber: purchase.CardNumber,
			Language:   lang,
		},
	}

	importResp, importErr := h.psaImporter.PSAImport(ctx, []dh.PSAImportItem{item})
	if importErr != nil {
		h.logger.Error(ctx, "retry match: PSA import", observability.Err(importErr))
		writeError(w, http.StatusBadGateway, "DH API error")
		return
	}

	if len(importResp.Results) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no results from DH")
		return
	}

	result := importResp.Results[0]
	switch result.Resolution {
	case dh.PSAImportStatusMatched, dh.PSAImportStatusUnmatchedCreated:
		if h.dhFieldsUpdater != nil {
			if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, purchase.ID, inventory.DHFieldsUpdate{
				CardID:      result.DHCardID,
				InventoryID: result.DHInventoryID,
				CertStatus:  dh.CertStatusMatched,
				DHStatus:    inventory.DHStatusInStock,
			}); err != nil {
				h.logger.Error(ctx, "retry match: persist DH fields after PSA import", observability.Err(err))
				writeError(w, http.StatusInternalServerError, "failed to persist DH fields")
				return
			}
		}
		if h.pushStatusUpdater != nil {
			if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, inventory.DHPushStatusMatched); err != nil {
				h.logger.Error(ctx, "retry match: update push status after PSA import", observability.Err(err))
				writeError(w, http.StatusInternalServerError, "failed to update push status")
				return
			}
		}
		if h.cardIDSaver != nil && result.DHCardID != 0 {
			if err := h.cardIDSaver.SaveExternalID(ctx, purchase.CardName, purchase.SetName, purchase.CardNumber, pricing.SourceDH, fmt.Sprintf("%d", result.DHCardID)); err != nil {
				h.logger.Warn(ctx, "retry match: save external card ID after PSA import", observability.Err(err),
					observability.String("cardName", purchase.CardName), observability.String("setName", purchase.SetName))
			}
		}
		if h.candidatesSaver != nil {
			if err := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, ""); err != nil {
				h.logger.Warn(ctx, "retry match: clear candidates after PSA import", observability.Err(err),
					observability.String("purchaseID", purchase.ID))
			}
		}
		writeJSON(w, http.StatusOK, retryMatchResponse{
			Status:        "ok",
			DHCardID:      result.DHCardID,
			DHInventoryID: result.DHInventoryID,
		})
	default:
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("DH could not create listing: %s", result.Resolution))
	}
}
