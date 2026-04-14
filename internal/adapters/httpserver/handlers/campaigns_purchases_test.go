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

	domainerrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- HandleListPurchases ---

func TestHandleListPurchases_GET_Success(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ListPurchasesByCampaignFn: func(_ context.Context, cid string, limit, offset int) ([]inventory.Purchase, error) {
			if cid != "c1" {
				t.Errorf("expected campaignID=c1, got %q", cid)
			}
			return []inventory.Purchase{{ID: "p1", CampaignID: cid}}, nil
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
	svc := &mocks.MockInventoryService{
		ListPurchasesByCampaignFn: func(_ context.Context, _ string, limit, offset int) ([]inventory.Purchase, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []inventory.Purchase{}, nil
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
	h := newTestHandler(&mocks.MockInventoryService{})

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
	svc := &mocks.MockInventoryService{
		CreatePurchaseFn: func(_ context.Context, p *inventory.Purchase) error {
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
	svc := &mocks.MockInventoryService{
		CreatePurchaseFn: func(_ context.Context, _ *inventory.Purchase) error {
			return inventory.ErrDuplicateCertNumber
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
	h := newTestHandler(&mocks.MockInventoryService{})

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
	svc := &mocks.MockInventoryService{
		ListSalesByCampaignFn: func(_ context.Context, _ string, _, _ int) ([]inventory.Sale, error) {
			return []inventory.Sale{{ID: "s1", PurchaseID: "p1"}}, nil
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
	svc := &mocks.MockInventoryService{
		GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
			return &inventory.Purchase{ID: id, CampaignID: "c1"}, nil
		},
		GetCampaignFn: func(_ context.Context, id string) (*inventory.Campaign, error) {
			return &inventory.Campaign{ID: id}, nil
		},
		CreateSaleFn: func(_ context.Context, s *inventory.Sale, _ *inventory.Campaign, _ *inventory.Purchase) error {
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
	svc := &mocks.MockInventoryService{
		GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
			return &inventory.Purchase{ID: id, CampaignID: "c1"}, nil
		},
		GetCampaignFn: func(_ context.Context, id string) (*inventory.Campaign, error) {
			return &inventory.Campaign{ID: id}, nil
		},
		CreateSaleFn: func(_ context.Context, _ *inventory.Sale, _ *inventory.Campaign, _ *inventory.Purchase) error {
			return inventory.ErrDuplicateSale
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
	svc := &mocks.MockInventoryService{
		GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
			return &inventory.Purchase{ID: id, CampaignID: "other-campaign"}, nil
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
	svc := &mocks.MockInventoryService{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return nil, inventory.ErrPurchaseNotFound
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
		lookupFn       func(context.Context, string) (*inventory.CertInfo, *inventory.MarketSnapshot, error)
		expectedStatus int
		checkBody      func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:       "success with market snapshot",
			method:     http.MethodGet,
			certNumber: "12345678",
			lookupFn: func(_ context.Context, cert string) (*inventory.CertInfo, *inventory.MarketSnapshot, error) {
				return &inventory.CertInfo{
						CertNumber: cert,
						CardName:   "Charizard",
						Grade:      10,
					}, &inventory.MarketSnapshot{
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
			lookupFn: func(_ context.Context, cert string) (*inventory.CertInfo, *inventory.MarketSnapshot, error) {
				return &inventory.CertInfo{CertNumber: cert}, nil, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "cert not found returns 404",
			method:     http.MethodGet,
			certNumber: "99999999",
			lookupFn: func(_ context.Context, _ string) (*inventory.CertInfo, *inventory.MarketSnapshot, error) {
				return nil, nil, fmt.Errorf("cert lookup: %w", inventory.ErrCertNotFound)
			},
			expectedStatus: http.StatusNotFound,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				decodeErrorResponse(t, rec)
			},
		},
		{
			name:       "internal error returns 500",
			method:     http.MethodGet,
			certNumber: "99999999",
			lookupFn: func(_ context.Context, _ string) (*inventory.CertInfo, *inventory.MarketSnapshot, error) {
				return nil, nil, fmt.Errorf("database connection failed")
			},
			expectedStatus: http.StatusInternalServerError,
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
			svc := &mocks.MockInventoryService{
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
				return fmt.Errorf("purchase lookup: %w", inventory.ErrPurchaseNotFound)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "validation error",
			purchaseID: "p1",
			body:       `{"buyCostCents":-1}`,
			updateFn: func(_ context.Context, _ string, _ int) error {
				return domainerrors.NewAppError(inventory.ErrCodeCampaignValidation, "buyCostCents must be >= 0")
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID string
			var capturedCents int
			svc := &mocks.MockInventoryService{
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
	svc := &mocks.MockInventoryService{
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
	h := newTestHandler(&mocks.MockInventoryService{})

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
	svc := &mocks.MockInventoryService{
		ReassignPurchaseFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("purchase lookup: %w", inventory.ErrPurchaseNotFound)
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

// --- HandleBulkSales ---

func TestHandleBulkSales(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		body       string
		wantStatus int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				CreateBulkSalesFn: func(_ context.Context, _ string, _ inventory.SaleChannel, _ string, items []inventory.BulkSaleInput) (*inventory.BulkSaleResult, error) {
					return &inventory.BulkSaleResult{Created: len(items)}, nil
				},
			},
			body:       `{"saleChannel":"ebay","saleDate":"2026-01-01","items":[{"purchaseId":"p-1","salePriceCents":5000},{"purchaseId":"p-2","salePriceCents":6000}]}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "no items returns 400",
			svc:        &mocks.MockInventoryService{},
			body:       `{"saleChannel":"ebay","saleDate":"2026-01-01","items":[]}`,
			wantStatus: http.StatusBadRequest,
			checkBody:  func(t *testing.T, rec *httptest.ResponseRecorder) { decodeErrorResponse(t, rec) },
		},
		{
			name: "service error returns 500",
			svc: &mocks.MockInventoryService{
				CreateBulkSalesFn: func(_ context.Context, _ string, _ inventory.SaleChannel, _ string, _ []inventory.BulkSaleInput) (*inventory.BulkSaleResult, error) {
					return nil, fmt.Errorf("db error")
				},
			},
			body:       `{"saleChannel":"ebay","saleDate":"2026-01-01","items":[{"purchaseId":"p-1","salePriceCents":5000}]}`,
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c-1/sales/bulk", strings.NewReader(tt.body))
			req.SetPathValue("id", "c-1")
			rec := httptest.NewRecorder()
			h.HandleBulkSales(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

// --- HandleDeletePurchase ---

func TestHandleDeletePurchase(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		campaignID string
		purchaseID string
		wantStatus int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: id, CampaignID: "c-1"}, nil
				},
				DeletePurchaseFn: func(_ context.Context, _ string) error { return nil },
			},
			campaignID: "c-1",
			purchaseID: "p-1",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "purchase not found returns 404",
			svc: &mocks.MockInventoryService{
				GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
					return nil, inventory.ErrPurchaseNotFound
				},
			},
			campaignID: "c-1",
			purchaseID: "missing",
			wantStatus: http.StatusNotFound,
			checkBody:  func(t *testing.T, rec *httptest.ResponseRecorder) { decodeErrorResponse(t, rec) },
		},
		{
			name: "wrong campaign returns 403",
			svc: &mocks.MockInventoryService{
				GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: id, CampaignID: "other-campaign"}, nil
				},
			},
			campaignID: "c-1",
			purchaseID: "p-1",
			wantStatus: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodDelete, "/api/campaigns/"+tt.campaignID+"/purchases/"+tt.purchaseID, nil)
			req.SetPathValue("id", tt.campaignID)
			req.SetPathValue("purchaseId", tt.purchaseID)
			rec := httptest.NewRecorder()
			h.HandleDeletePurchase(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

// --- HandleDeleteSale ---

func TestHandleDeleteSale(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		wantStatus int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: id, CampaignID: "c-1"}, nil
				},
				DeleteSaleByPurchaseIDFn: func(_ context.Context, _ string) error { return nil },
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "sale not found returns 404",
			svc: &mocks.MockInventoryService{
				GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: id, CampaignID: "c-1"}, nil
				},
				DeleteSaleByPurchaseIDFn: func(_ context.Context, _ string) error {
					return inventory.ErrSaleNotFound
				},
			},
			wantStatus: http.StatusNotFound,
			checkBody:  func(t *testing.T, rec *httptest.ResponseRecorder) { decodeErrorResponse(t, rec) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodDelete, "/api/campaigns/c-1/purchases/p-1/sale", nil)
			req.SetPathValue("id", "c-1")
			req.SetPathValue("purchaseId", "p-1")
			rec := httptest.NewRecorder()
			h.HandleDeleteSale(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

// --- HandleQuickAdd ---

func TestHandleQuickAdd(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		campaignID string
		body       string
		wantStatus int
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				QuickAddPurchaseFn: func(_ context.Context, campaignID string, _ inventory.QuickAddRequest) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: "p-new", CampaignID: campaignID}, nil
				},
			},
			campaignID: "c-1",
			body:       `{"certNumber":"12345","buyCostCents":5000}`,
			wantStatus: http.StatusCreated,
		},
		{
			name: "duplicate cert returns 409",
			svc: &mocks.MockInventoryService{
				QuickAddPurchaseFn: func(_ context.Context, _ string, _ inventory.QuickAddRequest) (*inventory.Purchase, error) {
					return nil, inventory.ErrDuplicateCertNumber
				},
			},
			campaignID: "c-1",
			body:       `{"certNumber":"12345","buyCostCents":5000}`,
			wantStatus: http.StatusConflict,
		},
		{
			name: "campaign not found returns 404",
			svc: &mocks.MockInventoryService{
				QuickAddPurchaseFn: func(_ context.Context, _ string, _ inventory.QuickAddRequest) (*inventory.Purchase, error) {
					return nil, inventory.ErrCampaignNotFound
				},
			},
			campaignID: "missing",
			body:       `{"certNumber":"99999"}`,
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/"+tt.campaignID+"/purchases/quick-add", strings.NewReader(tt.body))
			req.SetPathValue("id", tt.campaignID)
			rec := httptest.NewRecorder()
			h.HandleQuickAdd(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- HandlePriceOverrideStats ---

func TestHandlePriceOverrideStats(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		wantStatus int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success returns stats",
			svc: &mocks.MockInventoryService{
				GetPriceOverrideStatsFn: func(_ context.Context) (*inventory.PriceOverrideStats, error) {
					return &inventory.PriceOverrideStats{OverrideCount: 3}, nil
				},
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var result inventory.PriceOverrideStats
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.OverrideCount != 3 {
					t.Errorf("expected OverrideCount=3, got %d", result.OverrideCount)
				}
			},
		},
		{
			name: "service error returns 500",
			svc: &mocks.MockInventoryService{
				GetPriceOverrideStatsFn: func(_ context.Context) (*inventory.PriceOverrideStats, error) {
					return nil, fmt.Errorf("db error")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodGet, "/api/admin/price-override-stats", nil)
			rec := httptest.NewRecorder()
			h.HandlePriceOverrideStats(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

// --- HandleSetPriceOverride ---

func TestHandleSetPriceOverride(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		purchaseID string
		body       string
		wantStatus int
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				SetPriceOverrideFn: func(_ context.Context, _ string, _ int, _ string) error { return nil },
			},
			purchaseID: "p-1",
			body:       `{"priceCents":9900,"source":"manual"}`,
			wantStatus: http.StatusNoContent,
		},
		{
			name: "not found returns 404",
			svc: &mocks.MockInventoryService{
				SetPriceOverrideFn: func(_ context.Context, _ string, _ int, _ string) error {
					return inventory.ErrPurchaseNotFound
				},
			},
			purchaseID: "missing",
			body:       `{"priceCents":9900,"source":"manual"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "validation error returns 400",
			svc: &mocks.MockInventoryService{
				SetPriceOverrideFn: func(_ context.Context, _ string, _ int, _ string) error {
					return domainerrors.NewAppError(inventory.ErrCodeCampaignValidation, "priceCents must be >= 0")
				},
			},
			purchaseID: "p-1",
			body:       `{"priceCents":-1,"source":"manual"}`,
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodPatch, "/api/purchases/"+tt.purchaseID+"/price-override", strings.NewReader(tt.body))
			req.SetPathValue("purchaseId", tt.purchaseID)
			rec := httptest.NewRecorder()
			h.HandleSetPriceOverride(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- HandleClearPriceOverride ---

func TestHandleClearPriceOverride(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		purchaseID string
		wantStatus int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				SetPriceOverrideFn: func(_ context.Context, _ string, _ int, _ string) error { return nil },
			},
			purchaseID: "p-1",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "not found returns 404",
			svc: &mocks.MockInventoryService{
				SetPriceOverrideFn: func(_ context.Context, _ string, _ int, _ string) error {
					return inventory.ErrPurchaseNotFound
				},
			},
			purchaseID: "missing",
			wantStatus: http.StatusNotFound,
			checkBody:  func(t *testing.T, rec *httptest.ResponseRecorder) { decodeErrorResponse(t, rec) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodDelete, "/api/purchases/"+tt.purchaseID+"/price-override", nil)
			req.SetPathValue("purchaseId", tt.purchaseID)
			rec := httptest.NewRecorder()
			h.HandleClearPriceOverride(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

// --- HandleAcceptAISuggestion ---

func TestHandleAcceptAISuggestion(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		purchaseID string
		wantStatus int
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				AcceptAISuggestionFn: func(_ context.Context, _ string) error { return nil },
			},
			purchaseID: "p-1",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "no suggestion returns 409",
			svc: &mocks.MockInventoryService{
				AcceptAISuggestionFn: func(_ context.Context, _ string) error {
					return inventory.ErrNoAISuggestion
				},
			},
			purchaseID: "p-1",
			wantStatus: http.StatusConflict,
		},
		{
			name: "not found returns 404",
			svc: &mocks.MockInventoryService{
				AcceptAISuggestionFn: func(_ context.Context, _ string) error {
					return inventory.ErrPurchaseNotFound
				},
			},
			purchaseID: "missing",
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodPost, "/api/purchases/"+tt.purchaseID+"/accept-ai-suggestion", nil)
			req.SetPathValue("purchaseId", tt.purchaseID)
			rec := httptest.NewRecorder()
			h.HandleAcceptAISuggestion(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- HandleDismissAISuggestion ---

func TestHandleDismissAISuggestion(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mocks.MockInventoryService
		purchaseID string
		wantStatus int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			svc: &mocks.MockInventoryService{
				DismissAISuggestionFn: func(_ context.Context, _ string) error { return nil },
			},
			purchaseID: "p-1",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "not found returns 404",
			svc: &mocks.MockInventoryService{
				DismissAISuggestionFn: func(_ context.Context, _ string) error {
					return inventory.ErrPurchaseNotFound
				},
			},
			purchaseID: "missing",
			wantStatus: http.StatusNotFound,
			checkBody:  func(t *testing.T, rec *httptest.ResponseRecorder) { decodeErrorResponse(t, rec) },
		},
		{
			name: "service error returns 500",
			svc: &mocks.MockInventoryService{
				DismissAISuggestionFn: func(_ context.Context, _ string) error {
					return fmt.Errorf("db error")
				},
			},
			purchaseID: "p-1",
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.svc)
			req := httptest.NewRequest(http.MethodDelete, "/api/purchases/"+tt.purchaseID+"/ai-suggestion", nil)
			req.SetPathValue("purchaseId", tt.purchaseID)
			rec := httptest.NewRecorder()
			h.HandleDismissAISuggestion(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

// --- requirePurchaseInCampaign ---

func TestRequirePurchaseInCampaign(t *testing.T) {
	tests := []struct {
		name           string
		campaignID     string
		purchaseID     string
		setupMock      func(*mocks.MockInventoryService)
		wantStatus     int
		wantPurchaseID string
	}{
		{
			name:       "returns purchase when campaign matches",
			campaignID: "camp-1",
			purchaseID: "purch-1",
			setupMock: func(m *mocks.MockInventoryService) {
				m.GetPurchaseFn = func(_ context.Context, id string) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: id, CampaignID: "camp-1"}, nil
				}
			},
			wantStatus:     0, // no error response written
			wantPurchaseID: "purch-1",
		},
		{
			name:       "writes 404 when purchase not found",
			campaignID: "camp-1",
			purchaseID: "missing",
			setupMock: func(m *mocks.MockInventoryService) {
				m.GetPurchaseFn = func(_ context.Context, _ string) (*inventory.Purchase, error) {
					return nil, inventory.ErrPurchaseNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "writes 403 when purchase belongs to different campaign",
			campaignID: "camp-1",
			purchaseID: "purch-other",
			setupMock: func(m *mocks.MockInventoryService) {
				m.GetPurchaseFn = func(_ context.Context, id string) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: id, CampaignID: "camp-2"}, nil
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "writes 500 on internal service error",
			campaignID: "camp-1",
			purchaseID: "purch-1",
			setupMock: func(m *mocks.MockInventoryService) {
				m.GetPurchaseFn = func(_ context.Context, _ string) (*inventory.Purchase, error) {
					return nil, fmt.Errorf("database error")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockInventoryService{}
			tt.setupMock(svc)
			h := newTestHandler(svc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/", nil)

			purchase, ok := h.requirePurchaseInCampaign(rec, req, tt.campaignID, tt.purchaseID)
			if tt.wantStatus != 0 {
				if rec.Code != tt.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
				}
				if ok {
					t.Errorf("expected ok=false")
				}
				return
			}
			if !ok || purchase == nil {
				t.Fatalf("expected ok=true and non-nil purchase, got ok=%v purchase=%v", ok, purchase)
			}
			if purchase.ID != tt.wantPurchaseID {
				t.Errorf("purchase.ID = %q, want %q", purchase.ID, tt.wantPurchaseID)
			}
		})
	}
}
