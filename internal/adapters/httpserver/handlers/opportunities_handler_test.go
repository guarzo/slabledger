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

func TestHandleGetAcquisitionTargets_Success(t *testing.T) {
	want := []arbitrage.AcquisitionOpportunity{
		{CardName: "Charizard", SetName: "Base", CardNumber: "4"},
		{CardName: "Blastoise", SetName: "Base", CardNumber: "2"},
	}
	svc := &mocks.MockArbitrageService{
		GetAcquisitionTargetsFn: func(_ context.Context) ([]arbitrage.AcquisitionOpportunity, error) {
			return want, nil
		},
	}
	h := newOpportunitiesHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/opportunities/acquisition", nil)
	h.HandleGetAcquisitionTargets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got []arbitrage.AcquisitionOpportunity
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d", len(got), len(want))
	}
	if got[0].CardName != want[0].CardName || got[1].CardName != want[1].CardName {
		t.Errorf("card names not preserved: %v", got)
	}
}

func TestHandleGetAcquisitionTargets_NilBecomesEmptyArray(t *testing.T) {
	svc := &mocks.MockArbitrageService{
		GetAcquisitionTargetsFn: func(_ context.Context) ([]arbitrage.AcquisitionOpportunity, error) {
			return nil, nil
		},
	}
	h := newOpportunitiesHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/opportunities/acquisition", nil)
	h.HandleGetAcquisitionTargets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("body: got %q, want %q (nil should serialize as empty array)", body, "[]")
	}
}

func TestHandleGetAcquisitionTargets_ServiceError(t *testing.T) {
	svc := &mocks.MockArbitrageService{
		GetAcquisitionTargetsFn: func(_ context.Context) ([]arbitrage.AcquisitionOpportunity, error) {
			return nil, errors.New("db error")
		},
	}
	h := newOpportunitiesHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/opportunities/acquisition", nil)
	h.HandleGetAcquisitionTargets(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
}

// --- HandleGetCrackOpportunities ---

func TestHandleGetCrackOpportunities_Success(t *testing.T) {
	want := []arbitrage.CrackAnalysis{
		{PurchaseID: "p1"},
		{PurchaseID: "p2"},
	}
	svc := &mocks.MockArbitrageService{
		GetCrackOpportunitiesFn: func(_ context.Context) ([]arbitrage.CrackAnalysis, error) {
			return want, nil
		},
	}
	h := newOpportunitiesHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/opportunities/crack", nil)
	h.HandleGetCrackOpportunities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got []arbitrage.CrackAnalysis
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d", len(got), len(want))
	}
	if got[0].PurchaseID != "p1" || got[1].PurchaseID != "p2" {
		t.Errorf("purchase IDs not preserved: %v", got)
	}
}

func TestHandleGetCrackOpportunities_NilBecomesEmptyArray(t *testing.T) {
	svc := &mocks.MockArbitrageService{
		GetCrackOpportunitiesFn: func(_ context.Context) ([]arbitrage.CrackAnalysis, error) {
			return nil, nil
		},
	}
	h := newOpportunitiesHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/opportunities/crack", nil)
	h.HandleGetCrackOpportunities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("body: got %q, want %q", body, "[]")
	}
}

func TestHandleGetCrackOpportunities_ServiceError(t *testing.T) {
	svc := &mocks.MockArbitrageService{
		GetCrackOpportunitiesFn: func(_ context.Context) ([]arbitrage.CrackAnalysis, error) {
			return nil, errors.New("upstream timeout")
		},
	}
	h := newOpportunitiesHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/opportunities/crack", nil)
	h.HandleGetCrackOpportunities(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
}
