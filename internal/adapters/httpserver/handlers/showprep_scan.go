package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/observability"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

func (h *ShowPrepHandler) HandleScanConfirm(w http.ResponseWriter, r *http.Request) {
	listID, ok := showPrepPath(w, r, "listID")
	if !ok {
		return
	}
	// A pointer distinguishes a captured zero from an absent/null generation.
	var req struct {
		CommandID            string `json:"commandId"`
		CertNumber           string `json:"certNumber"`
		WithdrawalGeneration *int64 `json:"withdrawalGeneration"`
	}
	if !showPrepDecode(w, r, &req) {
		return
	}
	cert := strings.TrimSpace(req.CertNumber)
	if !showPrepID(req.CommandID) || cert == "" || len(cert) > 128 || req.WithdrawalGeneration == nil || *req.WithdrawalGeneration < 0 || *req.WithdrawalGeneration > sp.MaxWithdrawalGeneration {
		writeError(w, http.StatusBadRequest, "invalid scan command UUID, cert number or withdrawal generation")
		return
	}
	result, err := h.svc.ScanConfirm(r.Context(), listID, sp.ScanCommand{CommandID: req.CommandID, CertNumber: cert, WithdrawalGeneration: *req.WithdrawalGeneration})
	if err != nil {
		h.logShowCommandFailure(r, listID, req.CommandID, "scan", err)
		if errors.Is(err, sp.ErrContended) {
			writeJSON(w, http.StatusServiceUnavailable, struct {
				Error     string `json:"error"`
				Code      string `json:"code"`
				Retryable bool   `json:"retryable"`
			}{"Scan temporarily contended; retry the same command", "scan_contended", true})
			return
		}
		if showPrepInfrastructureError(err) {
			writeError(w, http.StatusInternalServerError, "Show preparation storage failed")
			return
		}
	}
	h.respond(w, r, result, err)
}

func (h *ShowPrepHandler) HandleDeleteList(w http.ResponseWriter, r *http.Request) {
	listID, ok := showPrepPath(w, r, "listID")
	if !ok {
		return
	}
	var cmd sp.DeleteListCommand
	if !showPrepDecode(w, r, &cmd) {
		return
	}
	if !showPrepID(cmd.CommandID) || cmd.ExpectedState == "" || cmd.ExpectedState != strings.TrimSpace(cmd.ExpectedState) {
		writeError(w, http.StatusBadRequest, "invalid deletion command UUID or expected state")
		return
	}
	result, err := h.svc.DeleteList(r.Context(), listID, cmd)
	if err != nil {
		h.logShowCommandFailure(r, listID, cmd.CommandID, "delete", err)
		if showPrepInfrastructureError(err) {
			writeError(w, http.StatusInternalServerError, "Show preparation storage failed")
			return
		}
	}
	h.respond(w, r, result, err)
}

func showPrepInfrastructureError(err error) bool {
	return err != nil && !errors.Is(err, sp.ErrInvalid) && !errors.Is(err, sp.ErrNotFound) && !errors.Is(err, sp.ErrConflict)
}

func (h *ShowPrepHandler) logShowCommandFailure(r *http.Request, listID, commandID, operation string, err error) {
	if h.logger == nil || (!showPrepInfrastructureError(err) && !errors.Is(err, sp.ErrContended)) {
		return
	}
	outcome := "storage_failed"
	if errors.Is(err, sp.ErrContended) {
		outcome = "contended"
	}
	// Log identifiers and failure class, never a cert, fingerprint or request body.
	h.logger.Error(r.Context(), "show preparation command failed", observability.String("list_id", listID), observability.String("command_id", commandID), observability.String("operation", operation), observability.String("outcome", outcome))
}
