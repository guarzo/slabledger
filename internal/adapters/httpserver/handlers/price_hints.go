package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PriceHintsHandler handles CRUD operations for user-provided price hints.
type PriceHintsHandler struct {
	resolver fusion.PriceHintResolver
	logger   observability.Logger
}

// NewPriceHintsHandler creates a new PriceHintsHandler.
func NewPriceHintsHandler(resolver fusion.PriceHintResolver, logger observability.Logger) *PriceHintsHandler {
	return &PriceHintsHandler{resolver: resolver, logger: logger}
}

type priceHintRequest struct {
	CardName   string `json:"cardName"`
	SetName    string `json:"setName"`
	CardNumber string `json:"cardNumber"`
	Provider   string `json:"provider"`
	ExternalID string `json:"externalId"`
}

type priceHintResponse struct {
	CardName        string `json:"cardName"`
	SetName         string `json:"setName"`
	CollectorNumber string `json:"cardNumber"`
	Provider        string `json:"provider"`
	ExternalID      string `json:"externalId"`
}

// HandlePriceHints dispatches GET/POST/DELETE for /api/price-hints.
func (h *PriceHintsHandler) HandlePriceHints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleList(w, r)
	case http.MethodPost:
		h.handleSave(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *PriceHintsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hints, err := h.resolver.ListHints(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to list price hints", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list hints")
		return
	}

	resp := make([]priceHintResponse, len(hints))
	for i, hint := range hints {
		resp[i] = priceHintResponse{
			CardName:        hint.CardName,
			SetName:         hint.SetName,
			CollectorNumber: hint.CollectorNumber,
			Provider:        hint.Provider,
			ExternalID:      hint.ExternalID,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *PriceHintsHandler) handleSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req priceHintRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CardName == "" || req.Provider == "" || req.ExternalID == "" || req.SetName == "" || req.CardNumber == "" {
		writeError(w, http.StatusBadRequest, "cardName, setName, cardNumber, provider, and externalId are required")
		return
	}
	if req.Provider != pricing.SourcePriceCharting && req.Provider != pricing.SourceDH {
		writeError(w, http.StatusBadRequest, "provider must be 'pricecharting' or 'doubleholo'")
		return
	}

	if err := h.resolver.SaveHint(ctx, req.CardName, req.SetName, req.CardNumber, req.Provider, req.ExternalID); err != nil {
		h.logger.Error(ctx, "failed to save price hint",
			observability.String("card", req.CardName),
			observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save hint")
		return
	}

	h.logger.Info(ctx, "price hint saved",
		observability.String("card", req.CardName),
		observability.String("provider", req.Provider),
		observability.String("external_id", req.ExternalID))

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *PriceHintsHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req priceHintRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CardName == "" || req.Provider == "" || req.SetName == "" || req.CardNumber == "" {
		writeError(w, http.StatusBadRequest, "cardName, setName, cardNumber, and provider are required")
		return
	}
	if req.Provider != pricing.SourcePriceCharting && req.Provider != pricing.SourceDH {
		writeError(w, http.StatusBadRequest, "provider must be 'pricecharting' or 'doubleholo'")
		return
	}

	if err := h.resolver.DeleteHint(ctx, req.CardName, req.SetName, req.CardNumber, req.Provider); err != nil {
		h.logger.Error(ctx, "failed to delete price hint",
			observability.String("card", req.CardName),
			observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to delete hint")
		return
	}

	h.logger.Info(ctx, "price hint deleted",
		observability.String("card", req.CardName),
		observability.String("provider", req.Provider))

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
