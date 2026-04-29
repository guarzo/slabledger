package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/domain/psaexchange"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestPSAExchangeHandler_GetOpportunities_Success(t *testing.T) {
	svc := &mocks.MockPSAExchangeService{
		OpportunitiesFn: func(_ context.Context) (psaexchange.OpportunitiesResult, error) {
			return psaexchange.OpportunitiesResult{
				Opportunities: []psaexchange.Listing{{
					Cert:               "28660366",
					Description:        "2009 Pokemon Charizard G Lv.X",
					Grade:              "10",
					ListPriceCents:     1003219,
					TargetOfferCents:   1395000,
					MaxOfferPct:        0.75,
					CompCents:          1860000,
					LastSalePriceCents: 1860000,
					LastSaleDate:       time.Date(2026, 4, 19, 23, 56, 0, 0, time.UTC),
					VelocityMonth:      1,
					VelocityQuarter:    6,
					Confidence:         5,
					Population:         80,
					EdgeAtOffer:        0.333,
					Score:              0.231,
					ListRunwayPct:      -0.391,
					MayTakeAtList:      true,
					FrontImage:         "front.jpg",
					BackImage:          "back.jpg",
					Tier:               "high_liquidity",
				}},
				CategoryURL:      "https://psa-exchange-catalog.com/catalog/X?cat=POKEMON+CARDS",
				FetchedAt:        time.Date(2026, 4, 29, 17, 34, 15, 0, time.UTC),
				TotalCatalog:     279,
				AfterFilter:      142,
				EnrichmentErrors: 0,
			}, nil
		},
	}
	h := handlers.NewPSAExchangeHandler(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/psa-exchange/opportunities", nil)
	rec := httptest.NewRecorder()
	h.HandleGetOpportunities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Opportunities       []map[string]any `json:"opportunities"`
		CategoryURL         string           `json:"categoryUrl"`
		FetchedAt           string           `json:"fetchedAt"`
		TotalCatalogPokemon int              `json:"totalCatalogPokemon"`
		AfterFilter         int              `json:"afterFilter"`
		EnrichmentErrors    int              `json:"enrichmentErrors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	if got.TotalCatalogPokemon != 279 || got.AfterFilter != 142 {
		t.Fatalf("counts = %+v", got)
	}
	if len(got.Opportunities) != 1 {
		t.Fatalf("expected 1 opportunity, got %d", len(got.Opportunities))
	}
	row := got.Opportunities[0]
	if row["cert"] != "28660366" {
		t.Fatalf("cert = %v", row["cert"])
	}
	// Money serialized as dollars (float). 1003219 cents → 10032.19 USD.
	if row["listPrice"].(float64) != 10032.19 {
		t.Fatalf("listPrice = %v, want 10032.19", row["listPrice"])
	}
	if row["targetOffer"].(float64) != 13950.00 {
		t.Fatalf("targetOffer = %v, want 13950", row["targetOffer"])
	}
	if row["mayTakeAtList"] != true {
		t.Fatalf("mayTakeAtList = %v", row["mayTakeAtList"])
	}
}

func TestPSAExchangeHandler_GetOpportunities_ServiceError(t *testing.T) {
	svc := &mocks.MockPSAExchangeService{
		OpportunitiesFn: func(_ context.Context) (psaexchange.OpportunitiesResult, error) {
			return psaexchange.OpportunitiesResult{}, errors.New("upstream down")
		},
	}
	h := handlers.NewPSAExchangeHandler(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/psa-exchange/opportunities", nil)
	rec := httptest.NewRecorder()
	h.HandleGetOpportunities(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestPSAExchangeHandler_NilService(t *testing.T) {
	h := handlers.NewPSAExchangeHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/psa-exchange/opportunities", nil)
	rec := httptest.NewRecorder()
	h.HandleGetOpportunities(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
