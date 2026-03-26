package instagram

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds an Instagram Client whose HTTPS calls are redirected to
// the provided TLS test server instead of graph.instagram.com / api.instagram.com.
// It uses a custom DialContext that routes all connections to the test server's
// host:port, plus InsecureSkipVerify to accept the self-signed test certificate.
func newTestClient(serverURL string) *Client {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		panic(fmt.Sprintf("parse test server URL: %v", err))
	}
	testHost := parsed.Host

	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, testHost)
		},
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test-only
		TLSHandshakeTimeout: 5 * time.Second,
	}

	cfg := httpx.DefaultConfig("Instagram")
	cfg.DefaultTimeout = 10 * time.Second
	cfg.Transport = transport

	c := NewClient("app-id", "app-secret", "https://example.com/callback", observability.NewNoopLogger())
	c.httpClient = httpx.NewClient(cfg)
	return c
}

// — Test helpers —

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	jsonResponse(w, code, map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "OAuthException",
			"code":    190,
		},
	})
}

// — Tests —

func TestPublishCarousel_Success(t *testing.T) {
	const igUserID = "user123"

	// Track how many item containers have been created so we can hand out unique IDs.
	var itemContainerCount int32

	// For the carousel container, we need to poll FINISHED once, then publish.
	var pollCount int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		// POST /{igUserID}/media — item containers or carousel container
		case r.Method == http.MethodPost && path == "/"+igUserID+"/media":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			mediaType := r.FormValue("media_type")
			isCarouselItem := r.FormValue("is_carousel_item")

			if mediaType == "CAROUSEL" {
				// Carousel container creation
				jsonResponse(w, http.StatusOK, map[string]string{"id": "carousel-container-001"})
			} else if isCarouselItem == "true" {
				// Item container creation
				n := atomic.AddInt32(&itemContainerCount, 1)
				jsonResponse(w, http.StatusOK, map[string]string{"id": fmt.Sprintf("item-%d", n)})
			} else {
				http.Error(w, "unexpected media POST", http.StatusBadRequest)
			}

		// GET /{containerID}?fields=status_code — waitForContainer
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "status_code"):
			n := atomic.AddInt32(&pollCount, 1)
			if n < 2 {
				jsonResponse(w, http.StatusOK, map[string]string{"status_code": "IN_PROGRESS"})
			} else {
				jsonResponse(w, http.StatusOK, map[string]string{"status_code": "FINISHED"})
			}

		// POST /{igUserID}/media_publish — publishContainer
		case r.Method == http.MethodPost && path == "/"+igUserID+"/media_publish":
			jsonResponse(w, http.StatusOK, map[string]string{"id": "post-id-abc"})

		default:
			http.Error(w, "unexpected request: "+r.Method+" "+path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	result, err := c.PublishCarousel(ctx, "tok", igUserID,
		[]string{"https://img.example.com/1.jpg", "https://img.example.com/2.jpg"},
		"Test carousel caption")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "post-id-abc", result.InstagramPostID)
	assert.Equal(t, int32(2), atomic.LoadInt32(&itemContainerCount), "should have created 2 item containers")
}

func TestPublishCarousel_SingleImage(t *testing.T) {
	const igUserID = "user456"

	var pollCount int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		// POST /{igUserID}/media — single image container (no is_carousel_item)
		case r.Method == http.MethodPost && path == "/"+igUserID+"/media":
			jsonResponse(w, http.StatusOK, map[string]string{"id": "single-container-001"})

		// GET — waitForContainer status polling
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "status_code"):
			atomic.AddInt32(&pollCount, 1)
			jsonResponse(w, http.StatusOK, map[string]string{"status_code": "FINISHED"})

		// POST /{igUserID}/media_publish
		case r.Method == http.MethodPost && path == "/"+igUserID+"/media_publish":
			jsonResponse(w, http.StatusOK, map[string]string{"id": "single-post-id"})

		default:
			http.Error(w, "unexpected request: "+r.Method+" "+path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	// PublishCarousel with a single image delegates to PublishSingleImage.
	result, err := c.PublishCarousel(ctx, "tok", igUserID,
		[]string{"https://img.example.com/only.jpg"},
		"Single image caption")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "single-post-id", result.InstagramPostID)
}

func TestPublishCarousel_ContainerCreateError(t *testing.T) {
	const igUserID = "user789"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// All POSTs to /media fail with an API error.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/media") {
			errorResponse(w, http.StatusBadRequest, "invalid image URL provided")
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	_, err := c.PublishCarousel(ctx, "tok", igUserID,
		[]string{"https://img.example.com/a.jpg", "https://img.example.com/b.jpg"},
		"caption")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create item container")
	assert.Contains(t, err.Error(), "invalid image URL provided")
}

func TestPublishCarousel_StatusError(t *testing.T) {
	const igUserID = "userErr"

	var itemCount int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodPost && path == "/"+igUserID+"/media":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			mediaType := r.FormValue("media_type")
			if mediaType == "CAROUSEL" {
				jsonResponse(w, http.StatusOK, map[string]string{"id": "carousel-err-001"})
			} else {
				n := atomic.AddInt32(&itemCount, 1)
				jsonResponse(w, http.StatusOK, map[string]string{"id": fmt.Sprintf("item-%d", n)})
			}

		// Polling always returns ERROR status.
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "status_code"):
			jsonResponse(w, http.StatusOK, map[string]string{"status_code": "ERROR"})

		default:
			http.Error(w, "unexpected request: "+r.Method+" "+path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	_, err := c.PublishCarousel(ctx, "tok", igUserID,
		[]string{"https://img.example.com/x.jpg", "https://img.example.com/y.jpg"},
		"caption")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "wait for carousel")
	assert.Contains(t, err.Error(), "failed processing")
}

func TestRefreshToken_Success(t *testing.T) {
	expiresIn := int64(5184000) // 60 days in seconds

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		// refreshTokenURL path is /refresh_access_token
		assert.Equal(t, "/refresh_access_token", r.URL.Path)
		assert.Equal(t, "ig_refresh_token", r.URL.Query().Get("grant_type"))
		assert.Equal(t, "old-access-token", r.URL.Query().Get("access_token"))

		jsonResponse(w, http.StatusOK, map[string]any{
			"access_token": "new-access-token",
			"token_type":   "bearer",
			"expires_in":   expiresIn,
		})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	before := time.Now()
	info, err := c.RefreshToken(ctx, "old-access-token")
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "new-access-token", info.AccessToken)

	expectedExpiry := before.Add(time.Duration(expiresIn) * time.Second)
	latestExpiry := after.Add(time.Duration(expiresIn) * time.Second)
	assert.True(t, !info.ExpiresAt.Before(expectedExpiry), "ExpiresAt should be at or after expected lower bound")
	assert.True(t, !info.ExpiresAt.After(latestExpiry), "ExpiresAt should be at or before expected upper bound")
}

func TestRefreshToken_Error(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/refresh_access_token", r.URL.Path)
		errorResponse(w, http.StatusUnauthorized, "The access token has expired")
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	_, err := c.RefreshToken(ctx, "expired-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refresh token")
	assert.Contains(t, err.Error(), "The access token has expired")
}
