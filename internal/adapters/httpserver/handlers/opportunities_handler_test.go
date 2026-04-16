package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newOpportunitiesHandler(svc *mocks.MockArbitrageService) *OpportunitiesHandler {
	return NewOpportunitiesHandler(svc, mocks.NewMockLogger())
}

// --- HandleGetAcquisitionTargets ---

func TestHandleGetAcquisitionTargets(t *testing.T) {
	twoTargets := []arbitrage.AcquisitionOpportunity{
		{CardName: "Charizard", SetName: "Base", CardNumber: "4"},
		{CardName: "Blastoise", SetName: "Base", CardNumber: "2"},
	}

	tests := []struct {
		name        string
		serviceList []arbitrage.AcquisitionOpportunity
		serviceErr  error
		wantCode    int
		// wantBodyLen: -1 to skip; >=0 to assert decoded slice length.
		wantBodyLen int
		// wantBodyEqual: true to assert body string == "[]" (nil-list contract).
		wantEmptyArrayBody bool
	}{
		{
			name:        "success returns full list",
			serviceList: twoTargets,
			wantCode:    http.StatusOK,
			wantBodyLen: len(twoTargets),
		},
		{
			name:               "nil list serializes as empty array",
			serviceList:        nil,
			wantCode:           http.StatusOK,
			wantBodyLen:        -1,
			wantEmptyArrayBody: true,
		},
		{
			name:        "service error → 500",
			serviceErr:  errors.New("db error"),
			wantCode:    http.StatusInternalServerError,
			wantBodyLen: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockArbitrageService{
				GetAcquisitionTargetsFn: func(_ context.Context) ([]arbitrage.AcquisitionOpportunity, error) {
					return tt.serviceList, tt.serviceErr
				},
			}
			h := newOpportunitiesHandler(svc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/opportunities/acquisition", nil)
			h.HandleGetAcquisitionTargets(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantEmptyArrayBody {
				if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
					t.Errorf("body: got %q, want %q", body, "[]")
				}
			}
			if tt.wantBodyLen >= 0 {
				var got []arbitrage.AcquisitionOpportunity
				if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(got) != tt.wantBodyLen {
					t.Fatalf("length: got %d, want %d", len(got), tt.wantBodyLen)
				}
				// On the populated case, sanity-check first/last fidelity.
				if tt.wantBodyLen == len(twoTargets) {
					if got[0].CardName != twoTargets[0].CardName || got[1].CardName != twoTargets[1].CardName {
						t.Errorf("card names not preserved: %v", got)
					}
				}
			}
		})
	}
}

// --- HandleGetCrackOpportunities ---

func TestHandleGetCrackOpportunities(t *testing.T) {
	twoCracks := []arbitrage.CrackAnalysis{
		{PurchaseID: "p1"},
		{PurchaseID: "p2"},
	}

	tests := []struct {
		name        string
		serviceList []arbitrage.CrackAnalysis
		serviceErr  error
		wantCode    int
		wantBodyLen int
		// wantEmptyArrayBody asserts nil-list-becomes-empty-array contract.
		wantEmptyArrayBody bool
	}{
		{
			name:        "success returns full list",
			serviceList: twoCracks,
			wantCode:    http.StatusOK,
			wantBodyLen: len(twoCracks),
		},
		{
			name:               "nil list serializes as empty array",
			serviceList:        nil,
			wantCode:           http.StatusOK,
			wantBodyLen:        -1,
			wantEmptyArrayBody: true,
		},
		{
			name:        "service error → 500",
			serviceErr:  errors.New("upstream timeout"),
			wantCode:    http.StatusInternalServerError,
			wantBodyLen: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockArbitrageService{
				GetCrackOpportunitiesFn: func(_ context.Context) ([]arbitrage.CrackAnalysis, error) {
					return tt.serviceList, tt.serviceErr
				},
			}
			h := newOpportunitiesHandler(svc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/opportunities/crack", nil)
			h.HandleGetCrackOpportunities(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantEmptyArrayBody {
				if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
					t.Errorf("body: got %q, want %q", body, "[]")
				}
			}
			if tt.wantBodyLen >= 0 {
				var got []arbitrage.CrackAnalysis
				if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(got) != tt.wantBodyLen {
					t.Fatalf("length: got %d, want %d", len(got), tt.wantBodyLen)
				}
				if tt.wantBodyLen == len(twoCracks) {
					if got[0].PurchaseID != "p1" || got[1].PurchaseID != "p2" {
						t.Errorf("purchase IDs not preserved: %v", got)
					}
				}
			}
		})
	}
}
