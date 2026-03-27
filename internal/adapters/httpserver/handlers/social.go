package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// ImageBackfiller can fetch PSA slab images and update purchase records.
type ImageBackfiller interface {
	BackfillImages(ctx context.Context) (updated int, errors int, err error)
}

// SocialHandler handles social media content endpoints.
type SocialHandler struct {
	service    social.Service
	repo       social.Repository
	logger     observability.Logger
	backfiller ImageBackfiller // optional — nil if PSA image API not configured
	mediaDir   string
	baseURL    string
}

// NewSocialHandler creates a new social handler.
func NewSocialHandler(service social.Service, repo social.Repository, logger observability.Logger, mediaDir, baseURL string) *SocialHandler {
	return &SocialHandler{service: service, repo: repo, logger: logger, mediaDir: mediaDir, baseURL: baseURL}
}

// WithBackfiller sets the optional image backfiller.
func (h *SocialHandler) WithBackfiller(b ImageBackfiller) { h.backfiller = b }

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

	posts, err := h.service.ListPosts(r.Context(), statusFilter, 100, 0)
	if err != nil {
		h.logger.Error(r.Context(), "list social posts failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if posts == nil {
		posts = []social.SocialPost{}
	}
	writeJSON(w, http.StatusOK, posts)
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

	detail, err := h.service.GetPost(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "get social post failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
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

	if err := h.service.UpdateCaption(r.Context(), id, req.Caption, req.Hashtags); err != nil {
		h.logger.Error(r.Context(), "update social caption failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
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
	if err := h.service.Delete(r.Context(), id); err != nil {
		h.logger.Error(r.Context(), "delete social post failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
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

// HandleBackfillImages triggers PSA image backfill for purchases missing images.
func (h *SocialHandler) HandleBackfillImages(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	if h.backfiller == nil {
		writeError(w, http.StatusServiceUnavailable, "Image backfill not configured (missing PSA image API token)")
		return
	}

	updated, backfillErrors, err := h.backfiller.BackfillImages(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "image backfill failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Image backfill failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"updated": updated, "errors": backfillErrors})
}

// HandleUploadSlides accepts rendered slide images and saves them to disk.
func (h *SocialHandler) HandleUploadSlides(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	id, ok := pathID(w, r, "id", "id")
	if !ok {
		return
	}

	if !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid post ID format")
		return
	}

	// Verify post exists and is in uploadable status
	post, err := h.repo.GetPost(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "get post for upload failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if post == nil {
		writeError(w, http.StatusNotFound, "Post not found")
		return
	}
	if post.Status != social.PostStatusDraft && post.Status != social.PostStatusFailed {
		writeError(w, http.StatusBadRequest, "Post must be in draft or failed status to upload slides")
		return
	}

	const maxUploadSize = 10 * (8 << 20) // 10 files x 8MB
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxUploadSize))

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse upload")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll() //nolint:errcheck // best-effort cleanup
	}

	dir := filepath.Join(h.mediaDir, "social", id)
	// Remove old slides before writing new ones to avoid orphaned files on re-upload
	_ = os.RemoveAll(dir) //nolint:errcheck // best-effort cleanup
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.logger.Error(r.Context(), "create media dir failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var urls []string
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("slide-%d", i)
		file, _, err := r.FormFile(key)
		if err != nil {
			if errors.Is(err, http.ErrMissingFile) {
				break // no more slides
			}
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to read %s: %v", key, err))
			return
		}

		const maxSlideSize = 8<<20 + 1 // 8MB + 1 byte for limit detection
		data, err := io.ReadAll(io.LimitReader(file, maxSlideSize))
		file.Close() //nolint:errcheck // read-only multipart form file
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to read %s", key))
			return
		}

		if len(data) > 8<<20 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s exceeds 8MB limit", key))
			return
		}

		if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is not a JPEG file", key))
			return
		}

		filename := fmt.Sprintf("slide-%d.jpg", i)
		outPath := filepath.Join(dir, filename)
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			h.logger.Error(r.Context(), "write slide file failed", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		base := h.resolveBaseURL(r)
		slideURL := fmt.Sprintf("%s/api/media/social/%s/%s", base, id, filename)
		urls = append(urls, slideURL)
		h.logger.Info(r.Context(), "slide uploaded",
			observability.String("postId", id),
			observability.String("file", filename),
			observability.Int("bytes", len(data)),
			observability.String("url", slideURL))
	}

	if len(urls) == 0 {
		writeError(w, http.StatusBadRequest, "no slide files found in upload")
		return
	}

	if err := h.repo.UpdateSlideURLs(r.Context(), id, urls); err != nil {
		h.logger.Error(r.Context(), "save slide URLs failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	h.logger.Info(r.Context(), "all slides saved",
		observability.String("postId", id),
		observability.Int("count", len(urls)))
	writeJSON(w, http.StatusOK, map[string]int{"slides": len(urls)})
}

// HandleServeMedia serves rendered slide images. Unauthenticated — Instagram API needs access.
func (h *SocialHandler) HandleServeMedia(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("postId")
	filename := r.PathValue("filename")

	if !isValidUUID(postID) {
		http.NotFound(w, r)
		return
	}

	if !isValidSlideFilename(filename) {
		http.NotFound(w, r)
		return
	}

	filePath := filepath.Join(h.mediaDir, "social", postID, filename)

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	absMedia, err2 := filepath.Abs(h.mediaDir)
	if err2 != nil {
		http.NotFound(w, r)
		return
	}
	if !strings.HasPrefix(absPath, absMedia+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}

	info, statErr := os.Stat(filePath)
	if statErr != nil {
		h.logger.Warn(r.Context(), "media serve: file not found",
			observability.String("path", filePath),
			observability.Err(statErr))
		http.NotFound(w, r)
		return
	}

	h.logger.Info(r.Context(), "media serve",
		observability.String("postId", postID),
		observability.String("file", filename),
		observability.Int("bytes", int(info.Size())),
		observability.String("userAgent", r.UserAgent()),
		observability.String("remoteAddr", r.RemoteAddr))

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filePath)
}

func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func isValidSlideFilename(s string) bool {
	if len(s) < 11 || len(s) > 12 {
		return false
	}
	if !strings.HasPrefix(s, "slide-") || !strings.HasSuffix(s, ".jpg") {
		return false
	}
	num := s[6 : len(s)-4]
	for _, c := range num {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// allowedImageHosts limits the image proxy to PSA card image CDN only.
// d1htnxwo4o0jhw.cloudfront.net serves card front/back images from the PSA cert API.
var allowedImageHosts = map[string]bool{
	"d1htnxwo4o0jhw.cloudfront.net": true,
}

var imageProxyClient = &http.Client{Timeout: 15 * time.Second}

// HandleImageProxy fetches an external card image and serves it same-origin,
// avoiding CORS issues during client-side slide rendering.
func (h *SocialHandler) HandleImageProxy(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		http.Error(w, "missing url param", http.StatusBadRequest)
		return
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || !allowedImageHosts[parsed.Host] {
		http.Error(w, "url not allowed", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}

	resp, err := imageProxyClient.Do(req)
	if err != nil {
		h.logger.Error(r.Context(), "image proxy fetch failed", observability.Err(err))
		http.Error(w, "fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != http.StatusOK {
		h.logger.Warn(r.Context(), "image proxy: upstream non-OK",
			observability.String("url", raw),
			observability.Int("status", resp.StatusCode))
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		http.Error(w, "not an image", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	const maxImageSize = 10 << 20
	n, copyErr := io.Copy(w, io.LimitReader(resp.Body, maxImageSize))
	if copyErr != nil {
		h.logger.Warn(r.Context(), "image proxy: copy interrupted",
			observability.String("url", raw),
			observability.Err(copyErr))
	} else if n == maxImageSize {
		h.logger.Warn(r.Context(), "image proxy: response truncated at 10MB limit",
			observability.String("url", raw))
	}
}
