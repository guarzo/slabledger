package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// noDiffCampaignAndPortal returns a linked internal campaign and its matching
// portal campaign whose scalar fields agree exactly, so TranslateToDiff
// produces zero changes — isolating the test from anything but the catalog
// wiring under test.
func noDiffCampaignAndPortal() (*inventory.Campaign, psacampaign.PortalCampaign) {
	c := &inventory.Campaign{
		ID:                   "c1",
		PSACampaignRequestID: "portal-1",
		BuyTermsCLPct:        0.75,
		DailySpendCapCents:   400000,
		GradeRange:           "9-10",
		YearRange:            "2020-2024",
		PriceRange:           "100-3000",
		CLConfidence:         "3-4",
	}
	portal := psacampaign.PortalCampaign{
		CampaignRequestID: "portal-1",
		BuyPercentClv:     75,
		DailyBudgetCents:  400000,
		BuyBox: psacampaign.CampaignBuyBox{
			GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
			PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
		},
	}
	return c, portal
}

// TestCampaignsHandlerOptions_WiresPSACatalogStore exercises
// campaignsHandlerOptions end-to-end through an actual handler call, rather
// than inspecting the returned option slice, so the assertion survives
// refactors of CampaignsHandlerOption's internal shape. Deleting the
// PSACatalogStore branch in campaignsHandlerOptions (or the whole append in
// startWebServer, pre-extraction) leaves h.psaCatalog nil in the running
// binary — invisible to every test that builds the handler directly with
// WithPSACatalogStore in hand, which is exactly what Task 9's tests do. This
// test goes through campaignsHandlerOptions itself to close that gap.
func TestCampaignsHandlerOptions_WiresPSACatalogStore(t *testing.T) {
	c, portal := noDiffCampaignAndPortal()
	now := time.Now()

	deps := ServerDependencies{
		CampaignsService: &mocks.MockInventoryService{
			GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) {
				return c, nil
			},
		},
		PSASnapshotStore: &mocks.SnapshotStoreMock{
			GetSnapshotFn: func(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
				return []psacampaign.PortalCampaign{portal}, now, nil
			},
		},
		PSAPushQueue: &mocks.PushQueueStoreMock{},
		PSACatalogStore: &mocks.CatalogStoreMock{
			SpecListsFn: func(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error) {
				return nil, now, nil
			},
			SubjectsFn: func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
				return nil, now, nil
			},
		},
	}

	h := handlers.NewCampaignsHandler(
		deps.CampaignsService, nil, nil, nil,
		observability.NewNoopLogger(), context.Background(),
		campaignsHandlerOptions(deps)...,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-propose", strings.NewReader(`{}`))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePSAPropose(rec, req)

	// A nil h.psaCatalog (the PSACatalogStore branch missing from
	// campaignsHandlerOptions) short-circuits to this exact 503 before ever
	// reaching TranslateToDiff — so its absence here is the discriminator.
	if rec.Code == http.StatusServiceUnavailable && strings.Contains(rec.Body.String(), "PSA catalog not enabled") {
		t.Fatalf("psaCatalog was not wired: got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (catalog wired, zero diff), got %d: %s", rec.Code, rec.Body.String())
	}
}
