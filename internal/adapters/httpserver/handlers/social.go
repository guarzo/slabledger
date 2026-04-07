package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// SocialHandler handles social media content endpoints.
type SocialHandler struct {
	service     social.Service
	repo        social.Repository
	logger      observability.Logger
	metricsRepo social.MetricsRepository // optional — nil if metrics not configured
	mediaDir    string
	baseURL     string
}

// NewSocialHandler creates a new social handler.
func NewSocialHandler(service social.Service, repo social.Repository, logger observability.Logger, mediaDir, baseURL string) *SocialHandler {
	return &SocialHandler{service: service, repo: repo, logger: logger, mediaDir: mediaDir, baseURL: baseURL}
}

// WithMetricsRepo sets the optional metrics repository.
func (h *SocialHandler) WithMetricsRepo(r social.MetricsRepository) { h.metricsRepo = r }

// resolveBaseURL returns the configured base URL or derives one from the request.
func (h *SocialHandler) resolveBaseURL(r *http.Request) string {
	if h.baseURL != "" {
		return h.baseURL
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

// HandleListPosts returns posts filtered by optional status query param.
func (h *SocialHandler) HandleListPosts(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	var statusFilter *social.PostStatus
	if s := r.URL.Query().Get("status"); s != "" {
		ps := social.PostStatus(s)
		switch ps {
		case social.PostStatusDraft, social.PostStatusPublishing, social.PostStatusPublished, social.PostStatusFailed:
			statusFilter = &ps
		default:
			writeError(w, http.StatusBadRequest, "invalid status filter")
			return
		}
	}

	posts, ok := serviceCall(w, r.Context(), h.logger, "list social posts failed", func() ([]social.SocialPost, error) {
		return h.service.ListPosts(r.Context(), statusFilter, 100, 0)
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, posts)
}

// HandleGetPost returns a single post with card details.
func (h *SocialHandler) HandleGetPost(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	id, ok := pathID(w, r, "id", "id")
	if !ok {
		return
	}

	detail, ok := serviceCall(w, r.Context(), h.logger, "get social post failed", func() (*social.PostDetail, error) {
		return h.service.GetPost(r.Context(), id)
	})
	if !ok {
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "Post not found")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// HandleGenerate triggers async post detection and generation.
// Returns 202 immediately; posts appear in the list as they are created.
func (h *SocialHandler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	// Run detection in a background goroutine with its own context so the
	// HTTP request timeout (30s) doesn't kill the LLM call.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		created, err := h.service.DetectAndGenerate(ctx)
		if err != nil {
			h.logger.Error(ctx, "social post generation failed", observability.Err(err))
			return
		}
		h.logger.Info(ctx, "social post generation completed", observability.Int("created", created))
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "generating"})
}

// HandleUpdateCaption updates a post's caption and hashtags.
func (h *SocialHandler) HandleUpdateCaption(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	id, ok := pathID(w, r, "id", "id")
	if !ok {
		return
	}

	var req struct {
		Caption  string `json:"caption"`
		Hashtags string `json:"hashtags"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	if !serviceCallVoid(w, r.Context(), h.logger, "update social caption failed", func() error {
		return h.service.UpdateCaption(r.Context(), id, req.Caption, req.Hashtags)
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleDelete deletes a post.
func (h *SocialHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	id, ok := pathID(w, r, "id", "id")
	if !ok {
		return
	}
	if !serviceCallVoid(w, r.Context(), h.logger, "delete social post failed", func() error {
		return h.service.Delete(r.Context(), id)
	}) {
		return
	}

	// Clean up media directory (best-effort)
	if h.mediaDir != "" && isValidUUID(id) {
		mediaPath := filepath.Join(h.mediaDir, "social", id)
		if err := os.RemoveAll(mediaPath); err != nil {
			h.logger.Warn(r.Context(), "failed to remove media directory", observability.String("path", mediaPath), observability.Err(err))
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleRegenerateCaption regenerates the AI caption via SSE streaming.
func (h *SocialHandler) HandleRegenerateCaption(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	id, ok := pathID(w, r, "id", "id")
	if !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		h.logger.Warn(r.Context(), "failed to disable SSE write deadline", observability.Err(err))
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	stream := func(evt ai.StreamEvent) {
		data, err := json.Marshal(evt)
		if err != nil {
			h.logger.Warn(r.Context(), "social: failed to marshal stream event",
				observability.String("eventType", string(evt.Type)),
				observability.Err(err))
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
		flusher.Flush()
	}

	if err := h.service.RegenerateCaption(r.Context(), id, stream); err != nil {
		h.logger.Error(r.Context(), "regenerate caption failed", observability.Err(err))
		stream(ai.StreamEvent{Type: ai.EventError, Content: "Caption generation failed. Please try again."})
	}

	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n") //nolint:errcheck
	flusher.Flush()
}

// HandleGetMetrics returns metrics snapshots for a single post.
func (h *SocialHandler) HandleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	id, ok := pathID(w, r, "id", "post")
	if !ok {
		return
	}
	if h.metricsRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not configured")
		return
	}
	metrics, err := h.metricsRepo.GetMetrics(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get metrics", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get metrics")
		return
	}
	if metrics == nil {
		metrics = []social.PostMetrics{}
	}
	writeJSONList(w, http.StatusOK, metrics)
}

// HandleGetMetricsSummary returns the latest metrics for all published posts.
func (h *SocialHandler) HandleGetMetricsSummary(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	if h.metricsRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not configured")
		return
	}
	summary, err := h.metricsRepo.GetMetricsSummary(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get metrics summary", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get metrics summary")
		return
	}
	if summary == nil {
		summary = []social.MetricsSummary{}
	}
	writeJSONList(w, http.StatusOK, summary)
}
