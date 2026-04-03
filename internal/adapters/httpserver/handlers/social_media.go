package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

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
