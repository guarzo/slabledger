package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// AdvisorHandler handles AI advisor analysis endpoints.
type AdvisorHandler struct {
	service      advisor.Service
	campaignsSvc interface {
		GetCampaign(ctx context.Context, id string) (*campaigns.Campaign, error)
	}
	cacheStore advisor.CacheStore // may be nil if caching is not configured
	logger     observability.Logger
	wg         sync.WaitGroup // tracks background analysis goroutines
}

// NewAdvisorHandler creates a new advisor handler.
func NewAdvisorHandler(
	service advisor.Service,
	campaignsSvc interface {
		GetCampaign(ctx context.Context, id string) (*campaigns.Campaign, error)
	},
	cacheStore advisor.CacheStore,
	logger observability.Logger,
) *AdvisorHandler {
	return &AdvisorHandler{
		service:      service,
		campaignsSvc: campaignsSvc,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Wait blocks until all background analysis goroutines have completed.
// Call during graceful shutdown to avoid writing to a closed database.
func (h *AdvisorHandler) Wait() { h.wg.Wait() }

// HandleGetCached returns the cached analysis result for the given type.
func (h *AdvisorHandler) HandleGetCached(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	if h.cacheStore == nil {
		writeError(w, http.StatusServiceUnavailable, "Analysis caching not configured")
		return
	}

	analysisType := advisor.AnalysisType(r.PathValue("type"))
	if analysisType != advisor.AnalysisDigest && analysisType != advisor.AnalysisLiquidation {
		writeError(w, http.StatusBadRequest, "Invalid analysis type")
		return
	}

	cached, ok := serviceCall(w, r.Context(), h.logger, "failed to get cached analysis", func() (*advisor.CachedAnalysis, error) {
		return h.cacheStore.Get(r.Context(), analysisType)
	})
	if !ok {
		return
	}

	if cached == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": string(advisor.StatusEmpty)})
		return
	}

	resp := map[string]any{
		"status":       string(cached.Status),
		"content":      cached.Content,
		"errorMessage": cached.ErrorMessage,
	}
	if !cached.UpdatedAt.IsZero() {
		resp["updatedAt"] = cached.UpdatedAt.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleRefreshTrigger starts a background analysis refresh and returns immediately.
func (h *AdvisorHandler) HandleRefreshTrigger(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	if h.cacheStore == nil {
		writeError(w, http.StatusServiceUnavailable, "Analysis caching not configured")
		return
	}

	analysisType := advisor.AnalysisType(r.PathValue("type"))
	if analysisType != advisor.AnalysisDigest && analysisType != advisor.AnalysisLiquidation {
		writeError(w, http.StatusBadRequest, "Invalid analysis type")
		return
	}

	// Atomically acquire the refresh lock. If already running, check for stale entries.
	type acquireResult struct {
		lease    string
		acquired bool
	}
	ar, ok := serviceCall(w, r.Context(), h.logger, "failed to acquire analysis refresh", func() (acquireResult, error) {
		lease, acquired, err := h.cacheStore.AcquireRefresh(r.Context(), analysisType)
		return acquireResult{lease, acquired}, err
	})
	if !ok {
		return
	}
	lease, acquired := ar.lease, ar.acquired
	if !acquired {
		// Already running — atomically force-restart if stale (> 15 minutes).
		sar, ok2 := serviceCall(w, r.Context(), h.logger, "failed to check stale analysis", func() (acquireResult, error) {
			staleLease, staleAcquired, staleErr := h.cacheStore.ForceAcquireStale(r.Context(), analysisType, 15*time.Minute)
			return acquireResult{staleLease, staleAcquired}, staleErr
		})
		if !ok2 {
			return
		}
		if !sar.acquired {
			writeJSON(w, http.StatusOK, map[string]string{"status": "running"})
			return
		}
		lease = sar.lease
		h.logger.Warn(r.Context(), "stale running entry detected, restarting",
			observability.String("type", string(analysisType)),
		)
	}

	// Run the analysis in a background goroutine with an independent context.
	bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer cancel()
		defer func() {
			if rv := recover(); rv != nil {
				h.logger.Error(bgCtx, "background analysis panicked",
					observability.String("type", string(analysisType)),
					observability.String("panic", fmt.Sprintf("%v", rv)),
				)
				if saveErr := h.cacheStore.SaveResult(bgCtx, analysisType, lease, "", fmt.Sprintf("panic: %v", rv)); saveErr != nil {
					h.logger.Error(bgCtx, "failed to save panic result", observability.Err(saveErr))
				}
			}
		}()

		var content string
		var analysisErr error
		switch analysisType {
		case advisor.AnalysisDigest:
			content, analysisErr = h.service.CollectDigest(bgCtx)
		case advisor.AnalysisLiquidation:
			content, analysisErr = h.service.CollectLiquidation(bgCtx)
		}

		errMsg := ""
		if analysisErr != nil {
			errMsg = analysisErr.Error()
			h.logger.Error(bgCtx, "background analysis failed",
				observability.String("type", string(analysisType)),
				observability.Err(analysisErr),
			)
		}

		if saveErr := h.cacheStore.SaveResult(bgCtx, analysisType, lease, content, errMsg); saveErr != nil {
			h.logger.Error(bgCtx, "failed to save analysis result", observability.Err(saveErr))
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "running"})
}

// HandleDigest generates a weekly intelligence digest via SSE.
func (h *AdvisorHandler) HandleDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if requireUser(w, r) == nil {
		return
	}

	h.streamAnalysis(w, r, func(stream func(advisor.StreamEvent)) error {
		return h.service.GenerateDigest(r.Context(), stream)
	})
}

// HandleCampaignAnalysis generates a campaign health and tuning narrative via SSE.
func (h *AdvisorHandler) HandleCampaignAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
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
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if requireUser(w, r) == nil {
		return
	}

	h.streamAnalysis(w, r, func(stream func(advisor.StreamEvent)) error {
		return h.service.AnalyzeLiquidation(r.Context(), stream)
	})
}

// HandlePurchaseAssessment evaluates a potential purchase via SSE.
func (h *AdvisorHandler) HandlePurchaseAssessment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if requireUser(w, r) == nil {
		return
	}

	var dto struct {
		CampaignID   string  `json:"campaignId"`
		CardName     string  `json:"cardName"`
		SetName      string  `json:"setName"`
		Grade        *string `json:"grade"`
		BuyCostCents *int    `json:"buyCostCents"`
		CLValueCents int     `json:"clValueCents"`
		CertNumber   string  `json:"certNumber"`
	}
	if !decodeBody(w, r, &dto) {
		return
	}
	if dto.CampaignID == "" || dto.CardName == "" {
		writeError(w, http.StatusBadRequest, "campaignId and cardName required")
		return
	}
	if dto.Grade == nil {
		writeError(w, http.StatusBadRequest, "grade is required")
		return
	}
	if dto.BuyCostCents == nil {
		writeError(w, http.StatusBadRequest, "buyCostCents is required")
		return
	}

	// Resolve campaign name for prompt context.
	var campaignName string
	if h.campaignsSvc != nil {
		if c, err := h.campaignsSvc.GetCampaign(r.Context(), dto.CampaignID); err == nil && c != nil {
			campaignName = c.Name
		} else if err != nil {
			h.logger.Warn(r.Context(), "could not resolve campaign name for purchase assessment",
				observability.String("campaignId", dto.CampaignID), observability.Err(err))
		}
	}
	if campaignName == "" {
		campaignName = dto.CampaignID // fallback: use ID if lookup fails
	}

	req := advisor.PurchaseAssessmentRequest{
		CampaignID:   dto.CampaignID,
		CampaignName: campaignName,
		CardName:     dto.CardName,
		SetName:      dto.SetName,
		Grade:        *dto.Grade,
		BuyCostCents: *dto.BuyCostCents,
		CLValueCents: dto.CLValueCents,
		CertNumber:   dto.CertNumber,
	}

	h.streamAnalysis(w, r, func(stream func(advisor.StreamEvent)) error {
		return h.service.AssessPurchase(r.Context(), req, stream)
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
