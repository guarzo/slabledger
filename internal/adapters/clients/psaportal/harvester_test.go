package psaportal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// fakeLightdash serves the two embed endpoints the harvester calls.
func fakeLightdash(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/embed/proj-1/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Lightdash-Embed-Token") != "jwt-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"results":{"tiles":[{"uuid":"tile-1","properties":{"chartSlug":"embed-itemized-purchases"}}]}}`)
	})
	mux.HandleFunc("/api/v1/embed/proj-1/chart-and-results", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":{"rows":[{"fct_instantoffers_offers_cert_number":{"value":{"raw":"12345678"}}}]}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// scriptOutputFile writes a fake harvest-script JSON output to a temp file and
// returns its path; tests run it via `cat` instead of node.
func scriptOutputFile(t *testing.T, ldURL string) string {
	t.Helper()
	// Minimal SvelteKit __data.json: root object {"embedUrl": <ptr 1>}, value at
	// index 1 is the embed URL string.
	svelteKit := fmt.Sprintf(
		`{"type":"data","nodes":[{"type":"data","data":[{"embedUrl":1},"%s/embed/proj-1#jwt-1"]}]}`, ldURL)
	out, err := json.Marshal(map[string]string{
		"accessToken":   "tok-1",
		"expiresAt":     "2099-01-01T00:00:00Z",
		"analyticsData": svelteKit,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "script-output.json")
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHarvester_Run(t *testing.T) {
	srv := fakeLightdash(t)

	t.Run("full pipeline saves token then snapshot", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		snaps := &mocks.PSASnapshotWriterMock{}
		h := NewHarvester(repo, snaps, ".", "email@test.com", "pw",
			observability.NewNoopLogger(), WithLightdashBaseURL(srv.URL))
		h.name = "cat"
		h.args = []string{scriptOutputFile(t, srv.URL)}

		if err := h.Run(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.SavedToken != "tok-1" {
			t.Errorf("savedToken: expected tok-1, got %q", repo.SavedToken)
		}
		if len(snaps.SavedRows) != 1 || snaps.SavedRows[0]["fct_instantoffers_offers_cert_number"] != "12345678" {
			t.Errorf("unexpected saved rows: %+v", snaps.SavedRows)
		}
		if snaps.SavedFetchedAt.IsZero() {
			t.Error("expected non-zero SavedFetchedAt")
		}
	})

	t.Run("passes stored valid token to script env", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{
			CurrentTokenFn: func(_ context.Context) (string, time.Time, error) {
				return "stored-tok", time.Now().Add(12 * time.Hour), nil
			},
		}
		snaps := &mocks.PSASnapshotWriterMock{}
		h := NewHarvester(repo, snaps, ".", "email@test.com", "pw",
			observability.NewNoopLogger(), WithLightdashBaseURL(srv.URL))
		// Echo the env var back as the failure message so we can assert it was set.
		h.name = "sh"
		h.args = []string{"-c", `test "$PSA_PORTAL_ACCESS_TOKEN" = "stored-tok" && cat ` + scriptOutputFile(t, srv.URL) + ` || exit 42`}

		if err := h.Run(context.Background()); err != nil {
			t.Fatalf("expected env var to be set, got error: %v", err)
		}
	})

	t.Run("expired stored token is not passed to script", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{
			CurrentTokenFn: func(_ context.Context) (string, time.Time, error) {
				return "expired-tok", time.Now().Add(-1 * time.Hour), nil
			},
		}
		snaps := &mocks.PSASnapshotWriterMock{}
		h := NewHarvester(repo, snaps, ".", "email@test.com", "pw",
			observability.NewNoopLogger(), WithLightdashBaseURL(srv.URL))
		h.name = "sh"
		h.args = []string{"-c", `test -z "$PSA_PORTAL_ACCESS_TOKEN" && cat ` + scriptOutputFile(t, srv.URL) + ` || exit 42`}

		if err := h.Run(context.Background()); err != nil {
			t.Fatalf("expected env var to be empty, got error: %v", err)
		}
	})

	t.Run("token still saved when lightdash fails", func(t *testing.T) {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer bad.Close()

		repo := &mocks.PSATokenRepositoryMock{}
		snaps := &mocks.PSASnapshotWriterMock{}
		h := NewHarvester(repo, snaps, ".", "email@test.com", "pw",
			observability.NewNoopLogger(), WithLightdashBaseURL(bad.URL))
		h.name = "cat"
		h.args = []string{scriptOutputFile(t, bad.URL)}

		err := h.Run(context.Background())
		if err == nil {
			t.Fatal("expected error from lightdash failure")
		}
		if repo.SavedToken != "tok-1" {
			t.Errorf("token must be saved before the lightdash exchange; got %q", repo.SavedToken)
		}
		if snaps.SavedRows != nil {
			t.Errorf("snapshot must not be saved on lightdash failure; got %+v", snaps.SavedRows)
		}
	})

	t.Run("empty rows do not overwrite the previous snapshot", func(t *testing.T) {
		empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/dashboard"):
				fmt.Fprint(w, `{"results":{"tiles":[{"uuid":"tile-1","properties":{"chartSlug":"embed-itemized-purchases"}}]}}`)
			default:
				fmt.Fprint(w, `{"results":{"rows":[]}}`)
			}
		}))
		defer empty.Close()

		repo := &mocks.PSATokenRepositoryMock{}
		snaps := &mocks.PSASnapshotWriterMock{}
		h := NewHarvester(repo, snaps, ".", "email@test.com", "pw",
			observability.NewNoopLogger(), WithLightdashBaseURL(empty.URL))
		h.name = "cat"
		h.args = []string{scriptOutputFile(t, empty.URL)}

		err := h.Run(context.Background())
		if err == nil {
			t.Fatal("expected error on zero-row harvest")
		}
		if repo.SavedToken != "tok-1" {
			t.Errorf("token must still be saved on an empty harvest; got %q", repo.SavedToken)
		}
		if snaps.SavedRows != nil {
			t.Errorf("empty harvest must not overwrite the snapshot; got %+v", snaps.SavedRows)
		}
	})

	t.Run("script exec error", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, ".", "e", "p",
			observability.NewNoopLogger())
		h.name = "false"
		if err := h.Run(context.Background()); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty token in script output", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, ".", "e", "p",
			observability.NewNoopLogger())
		h.name = "sh"
		h.args = []string{"-c", `printf '{"accessToken":"","expiresAt":"2099-01-01T00:00:00Z","analyticsData":"{}"}'`}
		if err := h.Run(context.Background()); err == nil ||
			!strings.Contains(err.Error(), "empty token") {
			t.Fatalf("expected empty-token error, got %v", err)
		}
	})
}
