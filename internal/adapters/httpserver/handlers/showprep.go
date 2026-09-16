package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/domain/observability"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

// ShowPrepService exposes cached reads and explicit list writes, not acquisition.
type ShowPrepService interface {
	Evaluate(context.Context, []string) ([]sp.Evaluation, error)
	Evidence(context.Context, string) (sp.Evidence, error)
	Lists(context.Context) ([]sp.List, error)
	CreateList(context.Context, string, string) (sp.List, error)
	ListDetail(context.Context, string) (sp.ListDetail, error)
	RenameList(context.Context, string, string) (sp.List, error)
	AddItems(context.Context, string, []sp.AddItem) (sp.ListDetail, error)
	UpdateItem(context.Context, string, string, sp.UpdateItem) (sp.ListDetail, error)
	RemoveItem(context.Context, string, string) error
}

type ShowPrepHandler struct {
	svc    ShowPrepService
	logger observability.Logger
}

func NewShowPrepHandler(svc ShowPrepService, logger observability.Logger) *ShowPrepHandler {
	return &ShowPrepHandler{svc, logger}
}
func (h *ShowPrepHandler) respond(w http.ResponseWriter, r *http.Request, result any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	status := http.StatusInternalServerError
	message := "Show preparation storage failed"
	switch {
	case errors.Is(err, sp.ErrInvalid):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, sp.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, sp.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	default:
		if h.logger != nil {
			h.logger.Error(r.Context(), "show preparation request failed", observability.Err(err))
		}
	}
	writeError(w, status, message)
}
func showPrepDecode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		writeError(w, 400, "invalid request body")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, 400, "unexpected trailing request body")
		return false
	}
	return true
}
func showPrepID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}
func showPrepPath(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id := r.PathValue(name)
	if !showPrepID(id) {
		writeError(w, 400, "invalid "+name)
		return "", false
	}
	return id, true
}
func showPrepIDs(w http.ResponseWriter, r *http.Request, limit int) ([]string, bool) {
	var req struct {
		PurchaseIDs []string `json:"purchaseIds"`
	}
	if !showPrepDecode(w, r, &req) {
		return nil, false
	}
	if len(req.PurchaseIDs) < 1 || len(req.PurchaseIDs) > limit {
		writeError(w, 400, "invalid purchase batch size")
		return nil, false
	}
	ids := make([]string, 0, len(req.PurchaseIDs))
	seen := map[string]bool{}
	for _, id := range req.PurchaseIDs {
		if !showPrepID(id) {
			writeError(w, 400, "invalid purchase UUID")
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, true
}
func (h *ShowPrepHandler) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	ids, ok := showPrepIDs(w, r, 200)
	if !ok {
		return
	}
	es, err := h.svc.Evaluate(r.Context(), ids)
	h.respond(w, r, struct {
		Evaluations []sp.Evaluation `json:"evaluations"`
	}{es}, err)
}
func (h *ShowPrepHandler) HandleEvidence(w http.ResponseWriter, r *http.Request) {
	id, ok := showPrepPath(w, r, "purchaseID")
	if !ok {
		return
	}
	e, err := h.svc.Evidence(r.Context(), id)
	h.respond(w, r, e, err)
}
func (h *ShowPrepHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "Comp collection is server-managed; reload the application")
}
