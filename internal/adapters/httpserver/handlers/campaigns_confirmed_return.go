package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"io"
	"net/http"
	"unicode/utf8"
)

// ConfirmedReturnService is deliberately separate from the generic inventory service.
type ConfirmedReturnService interface {
	ConfirmReturn(context.Context, string, inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error)
	GetReturnState(context.Context, string) (*inventory.ConfirmedReturnState, error)
}

func WithConfirmedReturnService(s ConfirmedReturnService) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.confirmedReturns = s }
}
func (h *CampaignsHandler) HandleConfirmReturn(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	id, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}
	if !inventory.DHDependenciesPresent(h.confirmedReturns) {
		writeError(w, 503, "Confirmed returns not configured")
		return
	}
	// RawMessage distinguishes explicit null CAS from an absent field.
	var body struct {
		ReturnConfirmed *bool                           `json:"returnConfirmed"`
		ExpectedSaleID  json.RawMessage                 `json:"expectedSaleId"`
		ExpectedTarget  *inventory.ReturnTargetIdentity `json:"expectedTarget"`
		OperationID     string                          `json:"operationId"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, 400, "Invalid confirmation body")
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF || body.ReturnConfirmed == nil || !*body.ReturnConfirmed || len(body.ExpectedSaleID) == 0 {
		writeError(w, 400, "returnConfirmed must be true and expectedSaleId must be present")
		return
	}
	var saleID *string
	if err := json.Unmarshal(body.ExpectedSaleID, &saleID); err != nil || (saleID != nil && *saleID == "") {
		writeError(w, 400, "expectedSaleId must be null or a nonempty sale ID")
		return
	}
	state, err := h.confirmedReturns.ConfirmReturn(r.Context(), id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: saleID, ExpectedTarget: body.ExpectedTarget, OperationID: body.OperationID})
	if err != nil {
		writeConfirmedReturnError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func (h *CampaignsHandler) HandleConfirmedReturnState(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	id, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}
	if !inventory.DHDependenciesPresent(h.confirmedReturns) {
		writeError(w, 503, "Confirmed returns not configured")
		return
	}
	state, err := h.confirmedReturns.GetReturnState(r.Context(), id)
	if err != nil {
		writeConfirmedReturnError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func writeConfirmedReturnError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusBadGateway, "return_uncertain", "DH return could not be verified; reload confirmed-return state before retrying"
	var conflict *inventory.ReturnConflict
	var provider *inventory.DHReturnError
	switch {
	case inventory.IsPurchaseNotFound(err):
		status, code, message = 404, "purchase_not_found", "Purchase not found"
	case errors.As(err, &conflict):
		status, code, message = 409, conflict.Code, conflict.Message
		if code == "coordination_unavailable" {
			status = 503
		}
	case errors.As(err, &provider):
		code, message = provider.Code, provider.Message
		if inventory.IsDefinitiveDHReturnRejection(err) {
			status = provider.Status
		} else if provider.Status == 503 {
			status = 503
		}
	}
	writeJSON(w, status, map[string]string{"error": boundedReturnText(message, 1024), "code": boundedReturnText(code, 128)})
}
func boundedReturnText(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
