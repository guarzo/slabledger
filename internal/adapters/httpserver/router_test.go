package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func setupTestRouter(t *testing.T) *Router {
	t.Helper()

	// Prevent LOCAL_API_TOKEN from enabling auth middleware in tests.
	t.Setenv("LOCAL_API_TOKEN", "")

	logger := mocks.NewMockLogger()
	cardProv := mocks.NewMockCardProvider()
	searchSvc := cards.NewSearchService(cardProv)

	handler := handlers.NewHandler(cardProv, searchSvc, logger)
	healthHandler := handlers.NewHealthHandler(nil, cardProv, nil, logger)
	spaHandler := handlers.NewSPAHandler(logger)

	campaignSvc := &mocks.MockInventoryService{}
	campaignsHandler := handlers.NewCampaignsHandler(campaignSvc, nil, nil, nil, logger, nil)

	return NewRouter(RouterConfig{
		Handler:          handler,
		HealthHandler:    healthHandler,
		SPAHandler:       spaHandler,
		CampaignsHandler: campaignsHandler,
		Logger:           logger,
	})
}

func TestRoutes_GetCampaigns_Returns200(t *testing.T) {
	rt := setupTestRouter(t)
	mux := rt.Setup()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/campaigns")
	if err != nil {
		t.Fatalf("GET /api/campaigns failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/campaigns status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestRoutes_GetHealth_Returns200(t *testing.T) {
	rt := setupTestRouter(t)
	mux := rt.Setup()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
