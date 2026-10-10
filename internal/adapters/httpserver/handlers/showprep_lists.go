package handlers

import (
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"net/http"
)

func (h *ShowPrepHandler) HandleLists(w http.ResponseWriter, r *http.Request) {
	lists, err := h.svc.Lists(r.Context())
	h.respond(w, r, struct {
		Lists []sp.List `json:"lists"`
	}{lists}, err)
}
func (h *ShowPrepHandler) HandleCreateList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if !showPrepDecode(w, r, &req) {
		return
	}
	if !showPrepID(req.ID) {
		writeError(w, 400, "invalid list UUID")
		return
	}
	list, err := h.svc.CreateList(r.Context(), req.ID, req.Name)
	h.respond(w, r, list, err)
}
func (h *ShowPrepHandler) HandleListDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := showPrepPath(w, r, "listID")
	if !ok {
		return
	}
	detail, err := h.svc.ListDetail(r.Context(), id)
	h.respond(w, r, detail, err)
}
func (h *ShowPrepHandler) HandleRenameList(w http.ResponseWriter, r *http.Request) {
	id, ok := showPrepPath(w, r, "listID")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !showPrepDecode(w, r, &req) {
		return
	}
	list, err := h.svc.RenameList(r.Context(), id, req.Name)
	h.respond(w, r, list, err)
}

// Stale clients receive guidance without parsing input or touching the service.
func (h *ShowPrepHandler) HandleAddItems(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "Add is retired; reload the application and scan each card using scan-confirm")
}
func (h *ShowPrepHandler) HandleUpdateItem(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "Pack/Acknowledge is retired; reload the application and scan each card using scan-confirm")
}
func (h *ShowPrepHandler) HandleRemoveItem(w http.ResponseWriter, r *http.Request) {
	list, ok := showPrepPath(w, r, "listID")
	if !ok {
		return
	}
	item, ok := showPrepPath(w, r, "itemID")
	if !ok {
		return
	}
	err := h.svc.RemoveItem(r.Context(), list, item)
	h.respond(w, r, struct {
		Removed bool `json:"removed"`
	}{true}, err)
}
