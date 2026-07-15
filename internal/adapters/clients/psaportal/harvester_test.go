package psaportal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// analyticsFetcher returns the given SvelteKit __data.json body for the
// analytics path and 404 otherwise.
type analyticsFetcher struct {
	analyticsBody string
	status        int
	err           error
}

func (a analyticsFetcher) Do(_ context.Context, req FetchRequest) (FetchResponse, error) {
	if a.err != nil {
		return FetchResponse{}, a.err
	}
	if strings.Contains(req.URL, "/buyercampaignmanager/analytics/__data.json") {
		st := a.status
		if st == 0 {
			st = 200
		}
		return FetchResponse{Status: st, Body: a.analyticsBody}, nil
	}
	return FetchResponse{Status: 404}, nil
}

func TestHarvester_Run(t *testing.T) {
	srv := fakeLightdash(t)

	t.Run("full pipeline saves token then snapshot", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		snaps := &mocks.PSASnapshotWriterMock{}
		h := NewHarvester(repo, snaps, observability.NewNoopLogger(), WithLightdashBaseURL(srv.URL))

		svelteKit := fmt.Sprintf(
			`{"type":"data","nodes":[{"type":"data","data":[{"embedUrl":1},"%s/embed/proj-1#jwt-1"]}]}`, srv.URL)
		af := analyticsFetcher{analyticsBody: svelteKit}
		exp := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

		if err := h.Run(context.Background(), af, "tok-1", exp); err != nil {
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

	t.Run("token still saved when lightdash fails", func(t *testing.T) {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer bad.Close()

		repo := &mocks.PSATokenRepositoryMock{}
		snaps := &mocks.PSASnapshotWriterMock{}
		h := NewHarvester(repo, snaps, observability.NewNoopLogger(), WithLightdashBaseURL(bad.URL))

		svelteKit := fmt.Sprintf(
			`{"type":"data","nodes":[{"type":"data","data":[{"embedUrl":1},"%s/embed/proj-1#jwt-1"]}]}`, bad.URL)
		af := analyticsFetcher{analyticsBody: svelteKit}
		exp := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

		err := h.Run(context.Background(), af, "tok-1", exp)
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
		h := NewHarvester(repo, snaps, observability.NewNoopLogger(), WithLightdashBaseURL(empty.URL))

		svelteKit := fmt.Sprintf(
			`{"type":"data","nodes":[{"type":"data","data":[{"embedUrl":1},"%s/embed/proj-1#jwt-1"]}]}`, empty.URL)
		af := analyticsFetcher{analyticsBody: svelteKit}
		exp := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

		err := h.Run(context.Background(), af, "tok-1", exp)
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

	t.Run("empty token is rejected", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, observability.NewNoopLogger())
		af := analyticsFetcher{}
		err := h.Run(context.Background(), af, "", time.Now())
		if err == nil || !strings.Contains(err.Error(), "empty token") {
			t.Fatalf("expected empty-token error, got %v", err)
		}
	})

	t.Run("analytics fetch error", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, observability.NewNoopLogger())
		af := analyticsFetcher{err: fmt.Errorf("boom")}
		if err := h.Run(context.Background(), af, "tok-1", time.Now().Add(time.Hour)); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("analytics fetch non-200", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, observability.NewNoopLogger())
		af := analyticsFetcher{status: 500, analyticsBody: "oops"}
		if err := h.Run(context.Background(), af, "tok-1", time.Now().Add(time.Hour)); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("analytics 200 missing embedUrl", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, observability.NewNoopLogger())
		af := analyticsFetcher{status: 200, analyticsBody: "{}"}
		err := h.Run(context.Background(), af, "tok-1", time.Now().Add(time.Hour))
		if err == nil || !strings.Contains(err.Error(), "missing embedUrl") {
			t.Fatalf("expected missing-embedUrl error, got %v", err)
		}
	})

	t.Run("SaveToken failure is wrapped as ErrPersistence", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{
			SaveTokenFn: func(_ context.Context, _ string, _ time.Time) error {
				return errors.New("db write failed")
			},
		}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, observability.NewNoopLogger())
		af := analyticsFetcher{}
		err := h.Run(context.Background(), af, "tok-1", time.Now().Add(time.Hour))
		if !errors.Is(err, ErrPersistence) {
			t.Fatalf("expected ErrPersistence, got %v", err)
		}
	})

	t.Run("analytics failure is not ErrPersistence", func(t *testing.T) {
		repo := &mocks.PSATokenRepositoryMock{}
		h := NewHarvester(repo, &mocks.PSASnapshotWriterMock{}, observability.NewNoopLogger())
		af := analyticsFetcher{err: fmt.Errorf("boom")}
		err := h.Run(context.Background(), af, "tok-1", time.Now().Add(time.Hour))
		if err == nil {
			t.Fatal("expected an error from analytics failure")
		}
		if errors.Is(err, ErrPersistence) {
			t.Fatalf("analytics failure must not be ErrPersistence, got %v", err)
		}
	})
}
