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
func (h *ShowPrepHandler) HandleAddItems(w http.ResponseWriter, r *http.Request) {
	id, ok := showPrepPath(w, r, "listID")
	if !ok {
		return
	}
	var req struct {
		Items []sp.AddItem `json:"items"`
	}
	if !showPrepDecode(w, r, &req) {
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 200 {
		writeError(w, 400, "invalid item batch size")
		return
	}
	seen := map[string]string{}
	for _, item := range req.Items {
		if !showPrepID(item.PurchaseID) || !showPrepVersion(item.EvaluationVersion) {
			writeError(w, 400, "invalid item purchase ID or evaluation version")
			return
		}
		if prior, ok := seen[item.PurchaseID]; ok && prior != item.EvaluationVersion {
			writeError(w, 400, "conflicting duplicate item versions")
			return
		}
		seen[item.PurchaseID] = item.EvaluationVersion
	}
	detail, err := h.svc.AddItems(r.Context(), id, req.Items)
	h.respond(w, r, detail, err)
}
func showPrepVersion(version string) bool { return len(version) > 0 && len(version) <= 128 }
func (h *ShowPrepHandler) HandleUpdateItem(w http.ResponseWriter, r *http.Request) {
	list, ok := showPrepPath(w, r, "listID")
	if !ok {
		return
	}
	item, ok := showPrepPath(w, r, "itemID")
	if !ok {
		return
	}
	var req sp.UpdateItem
	if !showPrepDecode(w, r, &req) {
		return
	}
	if req.Version <= 0 || (req.Packed == nil && !req.Acknowledge) || ((req.Acknowledge || (req.Packed != nil && *req.Packed)) && !showPrepVersion(req.EvaluationVersion)) {
		writeError(w, 400, "invalid item update or evaluation version")
		return
	}
	detail, err := h.svc.UpdateItem(r.Context(), list, item, req)
	h.respond(w, r, detail, err)
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
