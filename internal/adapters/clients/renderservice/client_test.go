package renderservice_test

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/renderservice"
	"github.com/guarzo/slabledger/internal/domain/social"
)

func TestHealth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := renderservice.NewClient(srv.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health() error: %v", err)
	}
}

func TestHealth_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := renderservice.NewClient(srv.URL)
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected error for non-200 health response")
	}
}

func TestRender_Success(t *testing.T) {
	fakeJPEG1 := []byte{0xFF, 0xD8, 0xFF, 0x01}
	fakeJPEG2 := []byte{0xFF, 0xD8, 0xFF, 0x02}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/render/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		mw := multipart.NewWriter(w)
		w.Header().Set("Content-Type", mw.FormDataContentType())
		fw0, _ := mw.CreateFormField("slide-0")
		fw0.Write(fakeJPEG1) //nolint:errcheck
		fw1, _ := mw.CreateFormField("slide-1")
		fw1.Write(fakeJPEG2) //nolint:errcheck
		mw.Close()           //nolint:errcheck
	}))
	defer srv.Close()

	c := renderservice.NewClient(srv.URL)
	detail := social.PostDetail{
		SocialPost: social.SocialPost{ID: "test-id", CardCount: 1},
	}
	blobs, err := c.Render(context.Background(), "test-id", detail)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if len(blobs) != 2 {
		t.Fatalf("expected 2 blobs, got %d", len(blobs))
	}
	if string(blobs[0]) != string(fakeJPEG1) {
		t.Errorf("blob[0] mismatch")
	}
}

func TestRender_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"render failed"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := renderservice.NewClient(srv.URL)
	detail := social.PostDetail{SocialPost: social.SocialPost{ID: "test-id"}}
	if _, err := c.Render(context.Background(), "test-id", detail); err == nil {
		t.Fatal("expected error for 500 response")
	}
}
