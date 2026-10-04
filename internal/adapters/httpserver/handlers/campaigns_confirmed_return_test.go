package handlers

import (
	"context"
	"errors"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfirmedReturnHandlerErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{{"missing coordination", inventory.NewReturnConflict("coordination_unavailable", "coordination required"), 503, "coordination_unavailable"}, {"unknown purchase", inventory.ErrPurchaseNotFound, 404, "purchase_not_found"}, {"unsupported source", inventory.NewReturnConflict("unsupported_return_source", "Native order cannot be returned externally"), 409, "unsupported_return_source"}, {"stale CAS", inventory.NewReturnConflict("sale_precondition_failed", "Current sale changed"), 409, "sale_precondition_failed"}, {"preceding attempt", inventory.NewReturnConflict("preceding_dh_mutation_uncertain", "Earlier PATCH is unresolved"), 409, "preceding_dh_mutation_uncertain"}, {"provider attribution", &inventory.DHReturnError{Status: 409, Code: "sale_attribution_missing", Message: "Original attribution missing"}, 409, "sale_attribution_missing"}, {"timeout", context.DeadlineExceeded, 502, "return_uncertain"}, {"unknown error", errors.New("raw private diagnostic"), 502, "return_uncertain"}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.ConfirmedReturnServiceMock{ConfirmReturnFn: func(context.Context, string, inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error) {
				return nil, tc.err
			}}
			h := NewCampaignsHandler(nil, nil, nil, nil, mocks.NewMockLogger(), nil, WithConfirmedReturnService(svc))
			r := withUser(httptest.NewRequest("POST", "/", strings.NewReader(`{"returnConfirmed":true,"expectedSaleId":null}`)))
			r.SetPathValue("purchaseId", "p1")
			w := httptest.NewRecorder()
			h.HandleConfirmReturn(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "raw private") {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
func TestExplicitReturnedListingRequiresUserAndCoordinator(t *testing.T) {
	received := "2026-01-01"
	calls := 0
	inv := &mocks.MockInventoryService{GetPurchaseFn: func(context.Context, string) (*inventory.Purchase, error) {
		return &inventory.Purchase{ID: "p1", CertNumber: "cert", ReceivedAt: &received, DHInventoryID: 42, DHStatus: "in_stock", ReviewedPriceCents: 25000}, nil
	}}
	state := &mocks.ConfirmedReturnServiceMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) {
		return &inventory.ConfirmedReturnState{AwaitingListing: true, Operation: &inventory.ConfirmedReturnEpisode{ID: "op1"}}, nil
	}}
	list := &mocks.MockDHListingService{ListPurchasesFn: func(context.Context, []string) dhlisting.DHListingResult {
		calls++
		return dhlisting.DHListingResult{Listed: 1}
	}}
	for _, user := range []bool{false, true} {
		h := NewCampaignsHandler(inv, nil, nil, nil, mocks.NewMockLogger(), nil, WithConfirmedReturnService(state), WithDHListingService(list))
		r := httptest.NewRequest("POST", "/", nil)
		r.SetPathValue("purchaseId", "p1")
		if user {
			r = withUser(r)
		}
		w := httptest.NewRecorder()
		h.HandleListPurchaseOnDH(w, r)
		want := 401
		if user {
			want = 503
		}
		if w.Code != want || calls != 0 {
			t.Fatalf("status=%d calls=%d", w.Code, calls)
		}
	}
}
func TestConfirmedReturnHandlerContract(t *testing.T) {
	tests := []struct {
		name, body string
		user       bool
		want       int
		calls      int
	}{
		{"no user", `{"returnConfirmed":true,"expectedSaleId":null}`, false, 401, 0},
		{"absent confirmation", `{"expectedSaleId":null}`, true, 400, 0},
		{"false confirmation", `{"returnConfirmed":false,"expectedSaleId":null}`, true, 400, 0},
		{"string confirmation", `{"returnConfirmed":"true","expectedSaleId":null}`, true, 400, 0},
		{"absent cas", `{"returnConfirmed":true}`, true, 400, 0},
		{"invalid cas", `{"returnConfirmed":true,"expectedSaleId":1}`, true, 400, 0},
		{"client target", `{"returnConfirmed":true,"expectedSaleId":null,"dhInventoryId":848}`, true, 400, 0},
		{"literal null", `{"returnConfirmed":true,"expectedSaleId":null}`, true, 200, 1},
		{"sale cas", `{"returnConfirmed":true,"expectedSaleId":"s1","operationId":"op1"}`, true, 200, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			svc := &mocks.ConfirmedReturnServiceMock{ConfirmReturnFn: func(_ context.Context, id string, req inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error) {
				calls++
				if id != "p1" || !req.ReturnConfirmed {
					t.Fatalf("bad request %s %+v", id, req)
				}
				if tt.name == "sale cas" && (req.ExpectedSaleID == nil || *req.ExpectedSaleID != "s1" || req.OperationID != "op1") {
					t.Fatalf("lost CAS: %+v", req)
				}
				return &inventory.ConfirmedReturnState{Outcome: "completed"}, nil
			}}
			h := NewCampaignsHandler(nil, nil, nil, nil, mocks.NewMockLogger(), nil, WithConfirmedReturnService(svc))
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			r.SetPathValue("purchaseId", "p1")
			if tt.user {
				r = withUser(r)
			}
			w := httptest.NewRecorder()
			h.HandleConfirmReturn(w, r)
			if w.Code != tt.want || calls != tt.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
		})
	}
}
func TestConfirmedReturnStateRequiresUser(t *testing.T) {
	calls := 0
	svc := &mocks.ConfirmedReturnServiceMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) { calls++; return nil, nil }}
	h := NewCampaignsHandler(nil, nil, nil, nil, mocks.NewMockLogger(), nil, WithConfirmedReturnService(svc))
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("purchaseId", "p1")
	w := httptest.NewRecorder()
	h.HandleConfirmedReturnState(w, r)
	if w.Code != 401 || calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, calls)
	}
}
