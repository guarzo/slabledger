package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestHandleGetSaleByCert(t *testing.T) {
	purchase := &inventory.Purchase{ID: "p1", CampaignID: "c1", CardName: "Charizard", CertNumber: "12345678", SetName: "Base Set", GradeValue: 9}
	tests := []struct {
		name     string
		cert     string
		fn       func(context.Context, string) (*inventory.SaleStatusByCert, error)
		wantCode int
		wantSale bool
	}{
		{
			name: "sold", cert: "12345678", wantCode: http.StatusOK, wantSale: true,
			fn: func(context.Context, string) (*inventory.SaleStatusByCert, error) {
				return &inventory.SaleStatusByCert{Purchase: purchase, Sale: &inventory.Sale{ID: "s1", SaleChannel: inventory.SaleChannelLocal, SaleDate: "2026-08-09", SalePriceCents: 8500}}, nil
			},
		},
		{
			name: "unsold", cert: "12345678", wantCode: http.StatusOK,
			fn: func(context.Context, string) (*inventory.SaleStatusByCert, error) {
				return &inventory.SaleStatusByCert{Purchase: purchase}, nil
			},
		},
		{
			name: "not found", cert: "99999999", wantCode: http.StatusNotFound,
			fn: func(context.Context, string) (*inventory.SaleStatusByCert, error) {
				return nil, fmt.Errorf("lookup: %w", inventory.ErrPurchaseNotFound)
			},
		},
		{name: "empty", cert: "", wantCode: http.StatusBadRequest},
		{
			name: "internal error", cert: "12345678", wantCode: http.StatusInternalServerError,
			fn: func(context.Context, string) (*inventory.SaleStatusByCert, error) {
				return nil, fmt.Errorf("db down")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(&mocks.MockInventoryService{GetSaleStatusByCertFn: tt.fn})
			req := httptest.NewRequest("GET", "/api/sales/by-cert/"+tt.cert, nil)
			req.SetPathValue("certNumber", tt.cert)
			rec := httptest.NewRecorder()
			h.HandleGetSaleByCert(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var got struct {
				Purchase map[string]any `json:"purchase"`
				Sale     map[string]any `json:"sale"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Purchase["id"] != "p1" || got.Purchase["certNumber"] != "12345678" {
				t.Errorf("purchase = %v", got.Purchase)
			}
			if tt.wantSale {
				if got.Sale["id"] != "s1" || got.Sale["salePriceCents"] != float64(8500) || got.Sale["saleChannel"] != "local" || got.Sale["saleDate"] != "2026-08-09" {
					t.Errorf("sale = %v", got.Sale)
				}
			} else if got.Sale != nil {
				t.Errorf("sale = %v, want null", got.Sale)
			}
		})
	}
}
