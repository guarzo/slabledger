package handlers

import (
	"context"
	"net/http"
	"time"
)

type showPrepStartKey struct{}

// CaptureShowPrepStart includes authentication and body decoding in the same
// socket-anchored budget; the router places it outside RequireAuth.
func CaptureShowPrepStart(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), showPrepStartKey{}, time.Now())))
	})
}

// Deadlines are anchored to route entry, not to the end of body decoding.
// A zero server write timeout is unlimited transport, not unlimited source work.
func refreshDeadlines(start time.Time, writeTimeout time.Duration, parent context.Context) (source, persistence, response time.Time) {
	window := 80 * time.Second
	if writeTimeout > 0 {
		window = min(window, writeTimeout)
	}
	if deadline, ok := parent.Deadline(); ok {
		window = min(window, max(time.Duration(0), deadline.Sub(start)))
	}
	reserve := min(5*time.Second, window/4)
	return start.Add(min(60*time.Second, window-2*reserve)), start.Add(window - reserve), start.Add(window)
}
