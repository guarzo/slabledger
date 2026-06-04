package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// AdvisorHandler handles AI advisor analysis endpoints.
type AdvisorHandler struct {
	service advisor.Service
	logger  observability.Logger
}

// NewAdvisorHandler creates a new advisor handler.
func NewAdvisorHandler(
	service advisor.Service,
	logger observability.Logger,
) *AdvisorHandler {
	return &AdvisorHandler{
		service: service,
		logger:  logger,
	}
}

// HandleDigest generates a weekly intelligence digest via SSE.
func (h *AdvisorHandler) HandleDigest(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	h.streamAnalysis(w, r, func(stream func(advisor.StreamEvent)) error {
		return h.service.GenerateDigest(r.Context(), stream)
	})
}

// HandleCampaignAnalysis generates a campaign health and tuning narrative via SSE.
func (h *AdvisorHandler) HandleCampaignAnalysis(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	var req struct {
		CampaignID string `json:"campaignId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CampaignID == "" {
		writeError(w, http.StatusBadRequest, "campaignId required")
		return
	}

	h.streamAnalysis(w, r, func(stream func(advisor.StreamEvent)) error {
		return h.service.AnalyzeCampaign(r.Context(), req.CampaignID, stream)
	})
}

// HandleLiquidationAnalysis generates liquidation candidates via SSE.
func (h *AdvisorHandler) HandleLiquidationAnalysis(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	h.streamAnalysis(w, r, func(stream func(advisor.StreamEvent)) error {
		return h.service.AnalyzeLiquidation(r.Context(), stream)
	})
}

// streamAnalysis sets up SSE headers and runs an analysis function, streaming events to the client.
func (h *AdvisorHandler) streamAnalysis(w http.ResponseWriter, r *http.Request, fn func(func(advisor.StreamEvent)) error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Disable the server's write deadline for this SSE connection.
	// Reasoning models can take 60-90s per round before streaming begins.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		h.logger.Warn(r.Context(), "failed to disable SSE write deadline; long analyses may timeout", observability.Err(err))
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	stream := func(evt advisor.StreamEvent) {
		data, err := json.Marshal(evt)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck // SSE write
		flusher.Flush()
	}

	if err := fn(stream); err != nil {
		h.logger.Error(r.Context(), "advisor analysis failed", observability.Err(err))
		errMsg := "Analysis failed. Please try again."
		if isRateLimitError(err) {
			errMsg = "Rate limited by AI provider. Please wait a few minutes and try again."
		}
		stream(advisor.StreamEvent{Type: advisor.EventError, Content: errMsg})
	}

	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n") //nolint:errcheck // SSE write
	flusher.Flush()
}

// isRateLimitError checks if an error chain contains a rate limit signal.
func isRateLimitError(err error) bool {
	status, _ := ai.ClassifyAIError(err)
	return status == ai.AIStatusRateLimited
}
