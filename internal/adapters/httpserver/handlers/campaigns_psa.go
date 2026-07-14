package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// HandleListPSACampaigns handles GET /api/psa-campaigns, returning the most
// recent PSA portal campaign snapshot.
func (h *CampaignsHandler) HandleListPSACampaigns(w http.ResponseWriter, r *http.Request) {
	if h.psaSnapshots == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA campaign sync not enabled")
		return
	}
	campaigns, fetchedAt, err := h.psaSnapshots.GetSnapshot(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get PSA campaign snapshot", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"campaigns": campaigns,
		"fetchedAt": fetchedAt,
	})
}

// psaLinkRequest is the body for HandlePSALink.
type psaLinkRequest struct {
	PSACampaignRequestID string `json:"psaCampaignRequestId"`
}

// HandlePSALink handles POST /api/campaigns/{id}/psa-link, linking an
// internal campaign to a PSA portal campaign by request ID.
func (h *CampaignsHandler) HandlePSALink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	var req psaLinkRequest
	if !decodeBody(w, r, &req) {
		return
	}

	c, err := h.service.GetCampaign(r.Context(), id)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	c.PSACampaignRequestID = req.PSACampaignRequestID
	if err := h.service.UpdateCampaign(r.Context(), c); err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		if inventory.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid campaign data")
			return
		}
		h.logger.Error(r.Context(), "failed to update campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	c.SetKind()
	writeJSON(w, http.StatusOK, c)
}

// psaProposeResponse is the response for HandlePSAPropose.
type psaProposeResponse struct {
	PushID string                   `json:"pushId,omitempty"`
	Diff   psacampaign.ProposedDiff `json:"diff"`
}

// HandlePSAPropose handles POST /api/campaigns/{id}/psa-propose, computing the
// diff between the internal campaign and its linked PSA portal campaign and
// enqueueing it for human approval.
func (h *CampaignsHandler) HandlePSAPropose(w http.ResponseWriter, r *http.Request) {
	if h.psaSnapshots == nil || h.psaQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA campaign sync not enabled")
		return
	}
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}

	c, err := h.service.GetCampaign(r.Context(), id)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if c.PSACampaignRequestID == "" {
		writeError(w, http.StatusBadRequest, "campaign is not linked to a PSA portal campaign")
		return
	}

	campaigns, _, err := h.psaSnapshots.GetSnapshot(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get PSA campaign snapshot", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var portal *psacampaign.PortalCampaign
	for i := range campaigns {
		if campaigns[i].CampaignRequestID == c.PSACampaignRequestID {
			portal = &campaigns[i]
			break
		}
	}
	if portal == nil {
		writeError(w, http.StatusNotFound, "linked PSA campaign not found in snapshot")
		return
	}

	diff, err := psacampaign.TranslateToDiff(*c, *portal)
	if err != nil {
		h.logger.Error(r.Context(), "failed to translate campaign diff", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if len(diff.Changes) == 0 {
		writeJSON(w, http.StatusOK, psaProposeResponse{Diff: diff})
		return
	}

	requestedBy := "analysis"
	if user, authOK := middleware.GetUserFromContext(r.Context()); authOK && user != nil && user.Username != "" {
		requestedBy = user.Username
	}

	row := psacampaign.PushRow{
		ID:                 uuid.New().String(),
		PSACampaignID:      c.PSACampaignRequestID,
		InternalCampaignID: c.ID,
		RequestedBy:        requestedBy,
		Diff:               diff,
		Status:             psacampaign.PushPending,
	}
	if err := h.psaQueue.Enqueue(r.Context(), row); err != nil {
		h.logger.Error(r.Context(), "failed to enqueue PSA push", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, psaProposeResponse{PushID: row.ID, Diff: diff})
}

// psaPublishRequest is the body for HandlePSAPublish.
type psaPublishRequest struct {
	PushID string `json:"pushId"`
}

// HandlePSAPublish handles POST /api/campaigns/{id}/psa-publish, approving a
// pending push-queue row. It never contacts psacard.com directly — the
// actual portal push happens out-of-band once approved.
func (h *CampaignsHandler) HandlePSAPublish(w http.ResponseWriter, r *http.Request) {
	if h.psaQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA campaign sync not enabled")
		return
	}
	var req psaPublishRequest
	if !decodeBody(w, r, &req) {
		return
	}

	user, authOK := middleware.GetUserFromContext(r.Context())
	if !authOK || user == nil || user.Username == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	approvedBy := user.Username

	if err := h.psaQueue.Approve(r.Context(), req.PushID, approvedBy); err != nil {
		if errors.Is(err, psacampaign.ErrPushNotPending) {
			writeError(w, http.StatusConflict, "push row is not pending")
			return
		}
		h.logger.Error(r.Context(), "failed to approve PSA push", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"pushId": req.PushID, "status": string(psacampaign.PushApproved)})
}

// psaProposeCreateResponse is the response for HandlePSAProposeCreate.
type psaProposeCreateResponse struct {
	PushID   string                       `json:"pushId"`
	FormData psacampaign.CampaignFormData `json:"formData"`
}

// HandlePSAProposeCreate handles POST /api/campaigns/{id}/psa-propose-create,
// building the full portal formData for an UNLINKED internal campaign and
// enqueueing a create for human approval. The portal campaign is created
// paused; approval goes through the existing psa-publish endpoint.
func (h *CampaignsHandler) HandlePSAProposeCreate(w http.ResponseWriter, r *http.Request) {
	if h.psaQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA campaign sync not enabled")
		return
	}
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}

	c, err := h.service.GetCampaign(r.Context(), id)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if c.PSACampaignRequestID != "" {
		writeError(w, http.StatusBadRequest, "campaign is already linked to a PSA portal campaign")
		return
	}

	// Dedupe guard: reject if a create for this campaign already exists. Pending/
	// approved/pushing rows mean a double-submit; a pushed-but-unlinked row means
	// the portal campaign was already created but its link-back failed — the
	// operator should link it manually (psa-link) rather than create a duplicate.
	existing, err := h.existingCreateStatus(r.Context(), c.ID)
	if err != nil {
		h.logger.Error(r.Context(), "failed to check for existing PSA create", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	switch existing {
	case psacampaign.PushPushed:
		writeError(w, http.StatusConflict, "a PSA campaign was already created for this campaign but is not linked — link it manually instead of creating another")
		return
	case psacampaign.PushPending, psacampaign.PushApproved, psacampaign.PushPushing:
		writeError(w, http.StatusConflict, "a PSA create is already queued for this campaign")
		return
	}

	fd, err := psacampaign.TranslateToCreate(*c)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	requestedBy := "analysis"
	if user, authOK := middleware.GetUserFromContext(r.Context()); authOK && user != nil && user.Username != "" {
		requestedBy = user.Username
	}

	row := psacampaign.PushRow{
		ID:                 uuid.New().String(),
		Operation:          psacampaign.OpCreate,
		InternalCampaignID: c.ID,
		RequestedBy:        requestedBy,
		Diff:               psacampaign.ProposedDiff{Create: &fd},
		Status:             psacampaign.PushPending,
	}
	if err := h.psaQueue.Enqueue(r.Context(), row); err != nil {
		h.logger.Error(r.Context(), "failed to enqueue PSA create", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, psaProposeCreateResponse{PushID: row.ID, FormData: fd})
}

// existingCreateStatus returns the status of an existing OpCreate push row for
// internalCampaignID, or "" if none exists. It checks unresolved rows (pending,
// approved, pushing) plus pushed rows — a pushed-but-unlinked create means the
// portal campaign exists and a re-create would duplicate it.
func (h *CampaignsHandler) existingCreateStatus(ctx context.Context, internalCampaignID string) (psacampaign.PushStatus, error) {
	for _, status := range []psacampaign.PushStatus{psacampaign.PushPending, psacampaign.PushApproved, psacampaign.PushPushing, psacampaign.PushPushed} {
		rows, err := h.psaQueue.ListByStatus(ctx, status)
		if err != nil {
			return "", err
		}
		for _, row := range rows {
			if row.Operation == psacampaign.OpCreate && row.InternalCampaignID == internalCampaignID {
				return status, nil
			}
		}
	}
	return "", nil
}
