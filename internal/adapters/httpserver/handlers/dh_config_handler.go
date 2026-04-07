package handlers

import (
	"errors"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	domainobs "github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleApproveDHPush handles POST /api/dh/approve/{purchaseId}.
// It clears the hold on a purchase and re-queues it for DH push.
func (h *DHHandler) HandleApproveDHPush(w http.ResponseWriter, r *http.Request) {
	if h.dhApproveService == nil {
		writeError(w, http.StatusServiceUnavailable, "DH approve service not configured")
		return
	}
	purchaseID := r.PathValue("purchaseId")
	if purchaseID == "" {
		writeError(w, http.StatusBadRequest, "missing purchaseId")
		return
	}
	if err := h.dhApproveService.ApproveDHPush(r.Context(), purchaseID); err != nil {
		if errors.Is(err, campaigns.ErrPurchaseNotFound) {
			h.logger.Error(r.Context(), "approve dh push: purchase not found", domainobs.Err(err))
			writeError(w, http.StatusNotFound, "purchase not found")
			return
		}
		if campaigns.IsValidationError(err) {
			h.logger.Error(r.Context(), "approve dh push: validation error", domainobs.Err(err))
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "approve dh push failed", domainobs.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to approve push")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// HandleGetDHPushConfig handles GET /api/admin/dh-push-config.
// It returns the current DH push safety configuration.
func (h *DHHandler) HandleGetDHPushConfig(w http.ResponseWriter, r *http.Request) {
	if h.dhApproveService == nil {
		writeError(w, http.StatusServiceUnavailable, "DH approve service not configured")
		return
	}
	cfg, err := h.dhApproveService.GetDHPushConfig(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "get dh push config failed", domainobs.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get DH push config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HandleSaveDHPushConfig handles PUT /api/admin/dh-push-config.
// It persists the DH push safety configuration.
func (h *DHHandler) HandleSaveDHPushConfig(w http.ResponseWriter, r *http.Request) {
	if h.dhApproveService == nil {
		writeError(w, http.StatusServiceUnavailable, "DH approve service not configured")
		return
	}
	var cfg campaigns.DHPushConfig
	if !decodeBody(w, r, &cfg) {
		return
	}
	if err := h.dhApproveService.SaveDHPushConfig(r.Context(), &cfg); err != nil {
		h.logger.Error(r.Context(), "save dh push config failed", domainobs.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save DH push config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
