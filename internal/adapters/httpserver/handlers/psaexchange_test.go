package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestPSAExchangeHandler_PolicyEndpoints(t *testing.T) {
	defaults := psaexchange.DefaultPolicy()
	activeCustom := psaexchange.DefaultPolicy()
	activeCustom.HighLiquidityOfferPct = 0.85

	// Wired by the SetPolicy cases below to capture what the handler forwarded
	// to the service. Each subtest gets its own pointer so the captures don't
	// alias across iterations.
	type captures struct {
		policy *psaexchange.Policy
	}

	cases := []struct {
		name       string
		method     string
		url        string
		body       string
		newSvc     func(*captures) *mocks.MockPSAExchangeService
		invoke     func(*handlers.PSAExchangeHandler, http.ResponseWriter, *http.Request)
		wantStatus int
		verify     func(t *testing.T, rec *httptest.ResponseRecorder, c *captures)
	}{
		{
			name:   "GET /policy returns active and defaults",
			method: http.MethodGet,
			url:    "/api/psa-exchange/policy",
			newSvc: func(_ *captures) *mocks.MockPSAExchangeService {
				return &mocks.MockPSAExchangeService{
					EffectivePolicyFn: func(_ context.Context) psaexchange.Policy { return activeCustom },
					PolicyFn:          func() psaexchange.Policy { return defaults },
				}
			},
			invoke:     (*handlers.PSAExchangeHandler).HandleGetPolicy,
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, rec *httptest.ResponseRecorder, _ *captures) {
				var got struct {
					Active   map[string]any `json:"active"`
					Defaults map[string]any `json:"defaults"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if got.Active["highLiquidityOfferPct"].(float64) != 0.85 {
					t.Fatalf("active hi pct = %v", got.Active["highLiquidityOfferPct"])
				}
				if got.Defaults["highLiquidityOfferPct"].(float64) != 0.75 {
					t.Fatalf("defaults hi pct = %v", got.Defaults["highLiquidityOfferPct"])
				}
			},
		},
		{
			name:   "PUT /policy happy path forwards parsed body to service",
			method: http.MethodPut,
			url:    "/api/psa-exchange/policy",
			body:   `{"highLiquidityVelocity":6,"highLiquidityConfidence":5,"highLiquidityOfferPct":0.78,"defaultOfferPct":0.62,"minConfidence":3,"minQuarterVelocity":1}`,
			newSvc: func(c *captures) *mocks.MockPSAExchangeService {
				return &mocks.MockPSAExchangeService{
					SetPolicyFn: func(_ context.Context, p psaexchange.Policy) error {
						c.policy = &p
						return nil
					},
					EffectivePolicyFn: func(_ context.Context) psaexchange.Policy { return defaults },
				}
			},
			invoke:     (*handlers.PSAExchangeHandler).HandlePutPolicy,
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, _ *httptest.ResponseRecorder, c *captures) {
				if c.policy == nil {
					t.Fatal("expected SetPolicy to be invoked")
				}
				if c.policy.HighLiquidityVelocity != 6 || c.policy.DefaultOfferPct != 0.62 {
					t.Fatalf("captured = %+v", c.policy)
				}
			},
		},
		{
			name:   "PUT /policy validation error → 400",
			method: http.MethodPut,
			url:    "/api/psa-exchange/policy",
			body:   `{"highLiquidityOfferPct":0,"defaultOfferPct":0.5}`,
			newSvc: func(_ *captures) *mocks.MockPSAExchangeService {
				return &mocks.MockPSAExchangeService{
					SetPolicyFn: func(_ context.Context, _ psaexchange.Policy) error {
						return psaexchange.ErrInvalidPolicy
					},
				}
			},
			invoke:     (*handlers.PSAExchangeHandler).HandlePutPolicy,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "PUT /policy malformed JSON → 400",
			method:     http.MethodPut,
			url:        "/api/psa-exchange/policy",
			body:       `{not json`,
			newSvc:     func(_ *captures) *mocks.MockPSAExchangeService { return &mocks.MockPSAExchangeService{} },
			invoke:     (*handlers.PSAExchangeHandler).HandlePutPolicy,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &captures{}
			h := handlers.NewPSAExchangeHandler(tc.newSvc(c), nil)
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.url, body)
			rec := httptest.NewRecorder()
			tc.invoke(h, rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.verify != nil {
				tc.verify(t, rec, c)
			}
		})
	}
}
