package handlers

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

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
