package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	domainerrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- HandleListPurchases ---

func TestHandleListPurchases_GET_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListPurchasesByCampaignFn: func(_ context.Context, cid string, limit, offset int) ([]campaigns.Purchase, error) {
			if cid != "c1" {
				t.Errorf("expected campaignID=c1, got %q", cid)
			}
			return []campaigns.Purchase{{ID: "p1", CampaignID: cid}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/purchases", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleListPurchases(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleListPurchases_GET_Pagination(t *testing.T) {
	var capturedLimit, capturedOffset int
	svc := &mocks.MockCampaignService{
		ListPurchasesByCampaignFn: func(_ context.Context, _ string, limit, offset int) ([]campaigns.Purchase, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []campaigns.Purchase{}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/purchases?limit=10&offset=20", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleListPurchases(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedLimit != 10 {
		t.Errorf("expected limit=10, got %d", capturedLimit)
	}
	if capturedOffset != 20 {
		t.Errorf("expected offset=20, got %d", capturedOffset)
	}
}

func TestHandleListPurchases_MissingCampaignID(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns//purchases", nil)
	// No SetPathValue — simulates missing ID
	rec := httptest.NewRecorder()
	h.HandleListPurchases(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleCreatePurchase ---

func TestHandleCreatePurchase_POST_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		CreatePurchaseFn: func(_ context.Context, p *campaigns.Purchase) error {
			if p.CampaignID != "c1" {
				t.Errorf("expected campaignID=c1, got %q", p.CampaignID)
			}
			if p.GradeValue != 9.5 {
				t.Errorf("expected gradeValue=9.5, got %g", p.GradeValue)
			}
			p.ID = "new-purchase"
			return nil
		},
	}
	h := newTestHandler(svc)

	body := `{"cardName":"Charizard","certNumber":"1234","gradeValue":9.5,"buyCostCents":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/purchases", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCreatePurchase(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreatePurchase_POST_DuplicateCert(t *testing.T) {
	svc := &mocks.MockCampaignService{
		CreatePurchaseFn: func(_ context.Context, _ *campaigns.Purchase) error {
			return campaigns.ErrDuplicateCertNumber
		},
	}
	h := newTestHandler(svc)

	body := `{"cardName":"Charizard","certNumber":"1234","gradeValue":9.5,"buyCostCents":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/purchases", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCreatePurchase(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

func TestHandleCreatePurchase_POST_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/purchases", bytes.NewBufferString("{bad"))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCreatePurchase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleListSales ---

func TestHandleListSales_GET_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListSalesByCampaignFn: func(_ context.Context, _ string, _, _ int) ([]campaigns.Sale, error) {
			return []campaigns.Sale{{ID: "s1", PurchaseID: "p1"}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/sales", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleListSales(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// --- HandleCreateSale ---

func TestHandleCreateSale_POST_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPurchaseFn: func(_ context.Context, id string) (*campaigns.Purchase, error) {
			return &campaigns.Purchase{ID: id, CampaignID: "c1"}, nil
		},
		GetCampaignFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			return &campaigns.Campaign{ID: id}, nil
		},
		CreateSaleFn: func(_ context.Context, s *campaigns.Sale, _ *campaigns.Campaign, _ *campaigns.Purchase) error {
			s.ID = "new-sale"
			return nil
		},
	}
	h := newTestHandler(svc)

	body := `{"purchaseId":"p1","saleChannel":"ebay","salePriceCents":10000,"saleDate":"2025-01-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sales", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCreateSale(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateSale_POST_DuplicateSale(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPurchaseFn: func(_ context.Context, id string) (*campaigns.Purchase, error) {
			return &campaigns.Purchase{ID: id, CampaignID: "c1"}, nil
		},
		GetCampaignFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			return &campaigns.Campaign{ID: id}, nil
		},
		CreateSaleFn: func(_ context.Context, _ *campaigns.Sale, _ *campaigns.Campaign, _ *campaigns.Purchase) error {
			return campaigns.ErrDuplicateSale
		},
	}
	h := newTestHandler(svc)

	body := `{"purchaseId":"p1","saleChannel":"ebay","salePriceCents":10000,"saleDate":"2025-01-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sales", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCreateSale(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

func TestHandleCreateSale_POST_PurchaseNotInCampaign(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPurchaseFn: func(_ context.Context, id string) (*campaigns.Purchase, error) {
			return &campaigns.Purchase{ID: id, CampaignID: "other-campaign"}, nil
		},
	}
	h := newTestHandler(svc)

	body := `{"purchaseId":"p1","saleChannel":"ebay","salePriceCents":10000,"saleDate":"2025-01-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sales", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCreateSale(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

func TestHandleCreateSale_POST_PurchaseNotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPurchaseFn: func(_ context.Context, _ string) (*campaigns.Purchase, error) {
			return nil, campaigns.ErrPurchaseNotFound
		},
	}
	h := newTestHandler(svc)

	body := `{"purchaseId":"missing","saleChannel":"ebay","salePriceCents":10000,"saleDate":"2025-01-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sales", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCreateSale(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

// --- HandleCertLookup ---

func TestHandleCertLookup(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		certNumber     string
		lookupFn       func(context.Context, string) (*campaigns.CertInfo, *campaigns.MarketSnapshot, error)
		expectedStatus int
		checkBody      func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:       "success with market snapshot",
			method:     http.MethodGet,
			certNumber: "12345678",
			lookupFn: func(_ context.Context, cert string) (*campaigns.CertInfo, *campaigns.MarketSnapshot, error) {
				return &campaigns.CertInfo{
						CertNumber: cert,
						CardName:   "Charizard",
						Grade:      10,
					}, &campaigns.MarketSnapshot{
						LastSoldCents: 50000,
					}, nil
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var result map[string]json.RawMessage
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if _, ok := result["cert"]; !ok {
					t.Error("expected response to contain 'cert' key")
				}
				if _, ok := result["market"]; !ok {
					t.Error("expected response to contain 'market' key")
				}
			},
		},
		{
			name:       "success without market snapshot",
			method:     http.MethodGet,
			certNumber: "12345678",
			lookupFn: func(_ context.Context, cert string) (*campaigns.CertInfo, *campaigns.MarketSnapshot, error) {
				return &campaigns.CertInfo{CertNumber: cert}, nil, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "cert not found returns 404",
			method:     http.MethodGet,
			certNumber: "99999999",
			lookupFn: func(_ context.Context, _ string) (*campaigns.CertInfo, *campaigns.MarketSnapshot, error) {
				return nil, nil, fmt.Errorf("cert not found")
			},
			expectedStatus: http.StatusNotFound,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				decodeErrorResponse(t, rec)
			},
		},
		{
			name:           "missing cert number returns 400",
			method:         http.MethodGet,
			certNumber:     "",
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				decodeErrorResponse(t, rec)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{
				LookupCertFn: tt.lookupFn,
			}
			h := newTestHandler(svc)

			path := "/api/certs/"
			if tt.certNumber != "" {
				path += tt.certNumber
			}
			req := httptest.NewRequest(tt.method, path, nil)
			req.SetPathValue("certNumber", tt.certNumber)
			rec := httptest.NewRecorder()
			h.HandleCertLookup(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

// --- HandleUpdateBuyCost ---

func TestHandleUpdateBuyCost(t *testing.T) {
	tests := []struct {
		name           string
		purchaseID     string
		body           string
		updateFn       func(context.Context, string, int) error
		expectedStatus int
		checkCaptures  func(t *testing.T, id string, cents int)
	}{
		{
			name:       "success",
			purchaseID: "p1",
			body:       `{"buyCostCents":18699}`,
			updateFn: func(_ context.Context, _ string, _ int) error {
				return nil
			},
			expectedStatus: http.StatusNoContent,
			checkCaptures: func(t *testing.T, id string, cents int) {
				if id != "p1" {
					t.Errorf("purchaseID = %q, want p1", id)
				}
				if cents != 18699 {
					t.Errorf("buyCostCents = %d, want 18699", cents)
				}
			},
		},
		{
			name:       "not found",
			purchaseID: "missing",
			body:       `{"buyCostCents":18699}`,
			updateFn: func(_ context.Context, _ string, _ int) error {
				return fmt.Errorf("purchase lookup: %w", campaigns.ErrPurchaseNotFound)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "validation error",
			purchaseID: "p1",
			body:       `{"buyCostCents":-1}`,
			updateFn: func(_ context.Context, _ string, _ int) error {
				return domainerrors.NewAppError(campaigns.ErrCodeCampaignValidation, "buyCostCents must be >= 0")
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID string
			var capturedCents int
			svc := &mocks.MockCampaignService{
				UpdateBuyCostFn: func(_ context.Context, purchaseID string, buyCostCents int) error {
					capturedID = purchaseID
					capturedCents = buyCostCents
					return tt.updateFn(context.Background(), purchaseID, buyCostCents)
				},
			}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodPatch, "/api/purchases/"+tt.purchaseID+"/buy-cost", strings.NewReader(tt.body))
			req.SetPathValue("purchaseId", tt.purchaseID)
			rec := httptest.NewRecorder()
			h.HandleUpdateBuyCost(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
			if tt.expectedStatus >= 400 {
				decodeErrorResponse(t, rec)
			}
			if tt.checkCaptures != nil {
				tt.checkCaptures(t, capturedID, capturedCents)
			}
		})
	}
}

// --- HandleReassignPurchase ---

func TestHandleReassignPurchase_Success(t *testing.T) {
	var capturedPurchaseID, capturedCampaignID string
	svc := &mocks.MockCampaignService{
		ReassignPurchaseFn: func(_ context.Context, purchaseID, newCampaignID string) error {
			capturedPurchaseID = purchaseID
			capturedCampaignID = newCampaignID
			return nil
		},
	}
	h := newTestHandler(svc)

	body := `{"campaignId":"camp-2"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/purchases/p1/campaign", strings.NewReader(body))
	req.SetPathValue("purchaseId", "p1")
	rec := httptest.NewRecorder()
	h.HandleReassignPurchase(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if capturedPurchaseID != "p1" {
		t.Errorf("purchaseID = %q, want p1", capturedPurchaseID)
	}
	if capturedCampaignID != "camp-2" {
		t.Errorf("campaignID = %q, want camp-2", capturedCampaignID)
	}
}

func TestHandleReassignPurchase_MissingCampaignID(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body := `{"campaignId":""}`
	req := httptest.NewRequest(http.MethodPatch, "/api/purchases/p1/campaign", strings.NewReader(body))
	req.SetPathValue("purchaseId", "p1")
	rec := httptest.NewRecorder()
	h.HandleReassignPurchase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

func TestHandleReassignPurchase_NotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ReassignPurchaseFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("purchase lookup: %w", campaigns.ErrPurchaseNotFound)
		},
	}
	h := newTestHandler(svc)

	body := `{"campaignId":"camp-2"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/purchases/missing/campaign", strings.NewReader(body))
	req.SetPathValue("purchaseId", "missing")
	rec := httptest.NewRecorder()
	h.HandleReassignPurchase(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}
