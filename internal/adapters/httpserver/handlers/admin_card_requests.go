package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CardRequestBody contains the fields for a card request submission.
type CardRequestBody struct {
	Player     string `json:"player"`
	Set        string `json:"set"`
	CardNumber string `json:"cardNumber"`
	ImageURL   string `json:"imageUrl"`
	ExternalID string `json:"externalId"`
	Token      string `json:"token"`
	Variant    string `json:"variant,omitempty"`
}

// CardRequestResponse contains the response from a card request submission.
type CardRequestResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CardRequestRepo abstracts the card request storage operations.
type CardRequestRepo interface {
	EnrichPendingFromPurchases(ctx context.Context) error
	ListAll(ctx context.Context) ([]sqlite.CardRequestSubmission, error)
	GetByID(ctx context.Context, id int64) (*sqlite.CardRequestSubmission, error)
	ClaimForProcessing(ctx context.Context, id int64) error
	RevertClaim(ctx context.Context, id int64) error
	UpdateSubmitted(ctx context.Context, id int64, requestID string) error
}

// CardRequester submits card requests to an external provider.
type CardRequester interface {
	CardRequest(ctx context.Context, req CardRequestBody) (*CardRequestResponse, int, http.Header, error)
}

// CardRequestHandlers handles admin card request endpoints.
type CardRequestHandlers struct {
	repo     CardRequestRepo
	client   CardRequester
	clientID string
	logger   observability.Logger
}

// NewCardRequestHandlers creates a new CardRequestHandlers.
// client and clientID are optional — when nil/empty the list endpoint still works
// but submit endpoints return 503.
func NewCardRequestHandlers(repo CardRequestRepo, client CardRequester, clientID string, logger observability.Logger) *CardRequestHandlers {
	return &CardRequestHandlers{
		repo:     repo,
		client:   client,
		clientID: clientID,
		logger:   logger,
	}
}

// HandleListCardRequests returns all card request submissions after enriching pending ones.
func (h *CardRequestHandlers) HandleListCardRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.repo.EnrichPendingFromPurchases(ctx); err != nil {
		h.logger.Warn(ctx, "failed to enrich pending card requests", observability.Err(err))
	}

	items, err := h.repo.ListAll(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to list card requests", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSONList(w, http.StatusOK, items)
}

// HandleSubmitCardRequest submits a single card request.
func (h *CardRequestHandlers) HandleSubmitCardRequest(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.clientID == "" {
		writeError(w, http.StatusServiceUnavailable, "Card request client is not configured")
		return
	}
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	// Enrich before building the payload so metadata is up-to-date.
	if err := h.repo.EnrichPendingFromPurchases(ctx); err != nil {
		h.logger.Warn(ctx, "failed to enrich before submit", observability.Err(err))
	}

	// Atomically claim the row so concurrent requests cannot double-submit.
	if err := h.repo.ClaimForProcessing(ctx, id); err != nil {
		if errors.Is(err, sqlite.ErrCardRequestAlreadyClaimed) {
			writeError(w, http.StatusConflict, "Card request already claimed or not pending")
			return
		}
		h.logger.Error(ctx, "failed to claim card request", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	record, err := h.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sqlite.ErrCardRequestNotFound) {
			writeError(w, http.StatusNotFound, "Card request not found")
			return
		}
		h.logger.Error(ctx, "failed to get card request", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	reqBody := CardRequestBody{
		Player:     record.CardName,
		Set:        record.SetName,
		CardNumber: record.CardNumber,
		ImageURL:   record.FrontImageURL,
		ExternalID: record.CertNumber,
		Token:      h.clientID,
		Variant:    record.Variant,
	}

	resp, _, _, err := h.client.CardRequest(ctx, reqBody)
	if err != nil {
		h.logger.Error(ctx, "card-request submission failed",
			observability.String("cert", record.CertNumber),
			observability.Err(err))
		// Revert the claim so the row can be retried.
		if revertErr := h.repo.RevertClaim(ctx, id); revertErr != nil {
			h.logger.Warn(ctx, "failed to revert claim after API error",
				observability.String("cert", record.CertNumber),
				observability.Err(revertErr))
		}
		writeError(w, http.StatusBadGateway, "Card request submission failed")
		return
	}

	requestID := resp.ID
	if requestID == "" {
		requestID = resp.Status
	}

	if err := h.repo.UpdateSubmitted(ctx, id, requestID); err != nil {
		h.logger.Error(ctx, "failed to update card request status", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	h.logger.Info(ctx, "card request submitted",
		observability.String("cert", record.CertNumber),
		observability.String("request_id", requestID))

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "submitted",
		"requestId": requestID,
	})
}

// HandleSubmitAllCardRequests submits all pending card requests.
func (h *CardRequestHandlers) HandleSubmitAllCardRequests(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.clientID == "" {
		writeError(w, http.StatusServiceUnavailable, "Card request client is not configured")
		return
	}
	ctx := r.Context()

	// Enrich before building payloads so metadata is up-to-date.
	if err := h.repo.EnrichPendingFromPurchases(ctx); err != nil {
		h.logger.Warn(ctx, "failed to enrich before submit-all", observability.Err(err))
	}

	items, err := h.repo.ListAll(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to list card requests for submit-all", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var submitted, errCount int
	for _, record := range items {
		if record.Status != "pending" {
			continue
		}

		// Atomically claim the row before calling the external API.
		if err := h.repo.ClaimForProcessing(ctx, record.ID); err != nil {
			if errors.Is(err, sqlite.ErrCardRequestAlreadyClaimed) {
				continue // another request already claimed it
			}
			h.logger.Warn(ctx, "failed to claim during submit-all",
				observability.String("cert", record.CertNumber),
				observability.Err(err))
			errCount++
			continue
		}

		reqBody := CardRequestBody{
			Player:     record.CardName,
			Set:        record.SetName,
			CardNumber: record.CardNumber,
			ImageURL:   record.FrontImageURL,
			ExternalID: record.CertNumber,
			Token:      h.clientID,
			Variant:    record.Variant,
		}

		resp, _, _, err := h.client.CardRequest(ctx, reqBody)
		if err != nil {
			h.logger.Warn(ctx, "card-request failed during submit-all",
				observability.String("cert", record.CertNumber),
				observability.Err(err))
			if revertErr := h.repo.RevertClaim(ctx, record.ID); revertErr != nil {
				h.logger.Warn(ctx, "failed to revert claim during submit-all",
					observability.String("cert", record.CertNumber),
					observability.Err(revertErr))
			}
			errCount++
			continue
		}

		requestID := resp.ID
		if requestID == "" {
			requestID = resp.Status
		}

		if err := h.repo.UpdateSubmitted(ctx, record.ID, requestID); err != nil {
			h.logger.Warn(ctx, "failed to update status during submit-all",
				observability.String("cert", record.CertNumber),
				observability.Err(err))
			errCount++
			continue
		}
		submitted++
	}

	h.logger.Info(ctx, "submit-all completed",
		observability.Int("submitted", submitted),
		observability.Int("errors", errCount))

	writeJSON(w, http.StatusOK, map[string]int{
		"submitted": submitted,
		"errors":    errCount,
	})
}
