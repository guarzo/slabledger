package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestHandleCardPricing(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		query      string
		priceProv  pricing.PriceProvider
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "GET success with all params",
			method:     http.MethodGet,
			query:      "name=Charizard&set=Base+Set&number=4",
			priceProv:  mocks.NewMockPriceProvider(),
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp CardPricingResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.Card != "Charizard" {
					t.Errorf("expected card=Charizard, got %s", resp.Card)
				}
				if resp.Set != "Base Set" {
					t.Errorf("expected set=Base Set, got %s", resp.Set)
				}
				if resp.Number != "4" {
					t.Errorf("expected number=4, got %s", resp.Number)
				}
				// Prices should be in USD (dollars), not cents
				if resp.RawUSD <= 0 {
					t.Errorf("expected rawUSD > 0, got %f", resp.RawUSD)
				}
				if resp.PSA10 <= 0 {
					t.Errorf("expected psa10 > 0, got %f", resp.PSA10)
				}
			},
		},
		{
			name:       "GET success with name only",
			method:     http.MethodGet,
			query:      "name=Pikachu",
			priceProv:  mocks.NewMockPriceProvider(),
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp CardPricingResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.Card != "Pikachu" {
					t.Errorf("expected card=Pikachu, got %s", resp.Card)
				}
			},
		},
		{
			name:       "GET missing name parameter",
			method:     http.MethodGet,
			query:      "set=Base+Set&number=4",
			priceProv:  mocks.NewMockPriceProvider(),
			wantStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				errMsg, ok := resp["error"]
				if !ok {
					t.Fatal("expected 'error' key in response")
				}
				if errMsg != "name parameter required" {
					t.Errorf("expected error 'name parameter required', got %q", errMsg)
				}
			},
		},
		{
			name:       "GET name too long",
			method:     http.MethodGet,
			query:      "name=" + strings.Repeat("a", 201) + "&set=Base+Set",
			priceProv:  mocks.NewMockPriceProvider(),
			wantStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if _, ok := resp["error"]; !ok {
					t.Fatal("expected 'error' key in response")
				}
			},
		},
		{
			name:       "GET price provider returns error",
			method:     http.MethodGet,
			query:      "name=UnknownCard&set=UnknownSet",
			priceProv:  mocks.NewMockPriceProvider(mocks.WithError(fmt.Errorf("lookup failed"))),
			wantStatus: http.StatusNotFound,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				errMsg, ok := resp["error"]
				if !ok {
					t.Fatal("expected 'error' key in response")
				}
				if errMsg != "no pricing data found" {
					t.Errorf("expected error 'no pricing data found', got %q", errMsg)
				}
			},
		},
		{
			name:       "POST wrong method",
			method:     http.MethodPost,
			query:      "name=Charizard",
			priceProv:  mocks.NewMockPriceProvider(),
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "GET no price provider configured",
			method:     http.MethodGet,
			query:      "name=Charizard",
			priceProv:  nil,
			wantStatus: http.StatusServiceUnavailable,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				errMsg, ok := resp["error"]
				if !ok {
					t.Fatal("expected 'error' key in response")
				}
				if errMsg != "pricing not available" {
					t.Errorf("expected error 'pricing not available', got %q", errMsg)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cardProv := mocks.NewMockCardProvider()
			searchSvc := cards.NewSearchService(cardProv)
			logger := mocks.NewMockLogger()

			var opts []HandlerOption
			if tc.priceProv != nil {
				opts = append(opts, WithPriceProvider(tc.priceProv))
			}

			h := NewHandler(cardProv, searchSvc, logger, opts...)

			url := "/api/cards/pricing"
			if tc.query != "" {
				url += "?" + tc.query
			}

			req := httptest.NewRequest(tc.method, url, nil)
			w := httptest.NewRecorder()
			h.HandleCardPricing(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, w.Body.Bytes())
			}
		})
	}
}
