package httpserver

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
)

func (rt *Router) registerShowPrepRoutes(mux *http.ServeMux) {
	// Always claim the API prefix. Missing OAuth/local-token configuration must
	// not accidentally expose shared ledger data or fall through to the SPA.
	if rt.authMW == nil {
		mux.HandleFunc("/api/show-prep/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
		})
		return
	}
	if rt.showPrepHandler == nil {
		mux.Handle("/api/show-prep/", rt.authMW.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"show preparation unavailable"}`))
		})))
		return
	}
	mux.Handle("/api/show-prep/", rt.authMW.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"show preparation endpoint not found"}`))
	})))
	h := rt.showPrepHandler
	for _, route := range []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"POST /api/show-prep/evaluate", h.HandleEvaluate},
		{"GET /api/show-prep/evidence/{purchaseID}", h.HandleEvidence},
		{"POST /api/show-prep/refresh", h.HandleRefresh},
		{"GET /api/show-prep/lists", h.HandleLists},
		{"POST /api/show-prep/lists", h.HandleCreateList},
		{"GET /api/show-prep/lists/{listID}", h.HandleListDetail},
		{"PUT /api/show-prep/lists/{listID}", h.HandleRenameList},
		{"POST /api/show-prep/lists/{listID}/items", h.HandleAddItems},
		{"PUT /api/show-prep/lists/{listID}/items/{itemID}", h.HandleUpdateItem},
		{"DELETE /api/show-prep/lists/{listID}/items/{itemID}", h.HandleRemoveItem},
	} {
		handler := rt.authMW.RequireAuth(route.handler)
		if route.pattern == "POST /api/show-prep/refresh" {
			handler = handlers.CaptureShowPrepStart(handler)
		}
		mux.Handle(route.pattern, handler)
	}
}
