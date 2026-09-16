package httpserver

import "net/http"

func (rt *Router) registerShowPrepWorkerRoutes(mux *http.ServeMux) {
	for _, route := range []struct {
		pattern string
		admin   bool
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"GET /api/show-prep/coverage", false, func(w http.ResponseWriter, r *http.Request) { rt.showPrepWorkerHandler.HandleCoverage(w, r) }},
		{"GET /api/admin/show-prep/worker", true, func(w http.ResponseWriter, r *http.Request) { rt.showPrepWorkerHandler.HandleStatus(w, r) }},
		{"POST /api/admin/show-prep/worker/run", true, func(w http.ResponseWriter, r *http.Request) { rt.showPrepWorkerHandler.HandleRun(w, r) }},
		{"POST /api/admin/show-prep/worker/retry", true, func(w http.ResponseWriter, r *http.Request) { rt.showPrepWorkerHandler.HandleRetry(w, r) }},
	} {
		var handler http.Handler = http.HandlerFunc(route.handler)
		if rt.showPrepWorkerHandler == nil {
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { workerRouteError(w, 503, "Evidence worker unavailable") })
		}
		if rt.authMW == nil {
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { workerRouteError(w, 401, "Authentication required") })
		} else if route.admin {
			handler = rt.authMW.RequireAdmin(handler)
		} else {
			handler = rt.authMW.RequireAuth(handler)
		}
		mux.Handle(route.pattern, handler)
	}
}

func workerRouteError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
