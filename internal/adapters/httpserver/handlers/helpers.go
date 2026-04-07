package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// decodeBody decodes the JSON request body into dst.
// It rejects unknown fields and trailing data so only a single
// well-formed JSON object with known fields is accepted.
// On failure it writes a 400 response and returns false.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return false
	}
	// Reject trailing data after the first JSON value.
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return false
	}
	return true
}

// pathID extracts the named path value from the request.
// If missing, it writes a 400 response with "<label> required" and returns ("", false).
func pathID(w http.ResponseWriter, r *http.Request, name, label string) (string, bool) {
	v := r.PathValue(name)
	if v == "" {
		writeError(w, http.StatusBadRequest, label+" required")
		return "", false
	}
	return v, true
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data) //nolint:errcheck // response already committed; write error unactionable
}

// writeJSONList writes a JSON array response, converting nil slices to empty
// arrays so the client always receives [] instead of null.
func writeJSONList[T any](w http.ResponseWriter, status int, list []T) {
	if list == nil {
		list = []T{}
	}
	writeJSON(w, status, list)
}

// writeError writes a structured JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// truncateID returns the first 8 characters of an ID followed by "..."
// if the ID is longer than 8 characters.
func truncateID(id string) string {
	if id == "" {
		return ""
	}
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}

// extractDomain extracts the domain part from an email address.
func extractDomain(email string) string {
	if email == "" {
		return ""
	}
	atIndex := strings.LastIndex(email, "@")
	if atIndex == -1 || atIndex == len(email)-1 {
		return ""
	}
	return email[atIndex+1:]
}

// requireUser extracts the authenticated user from context.
// Returns nil and writes a 401 JSON response if not authenticated.
func requireUser(w http.ResponseWriter, r *http.Request) *auth.User {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok || user == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return nil
	}
	return user
}

// parsePagination extracts limit and offset query parameters from the request.
func parsePagination(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return
}

// serviceCall invokes fn and returns the result. On error, it logs the failure
// and writes a 500 JSON response, returning the zero value and false.
// The msg parameter is the log message (e.g. "failed to list campaigns").
func serviceCall[T any](w http.ResponseWriter, ctx context.Context, logger observability.Logger, msg string, fn func() (T, error)) (T, bool) {
	result, err := fn()
	if err != nil {
		logger.Error(ctx, msg, observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return result, false
	}
	return result, true
}

// serviceCallVoid invokes fn. On error, it logs the failure and writes a 500
// JSON response, returning false.
func serviceCallVoid(w http.ResponseWriter, ctx context.Context, logger observability.Logger, msg string, fn func() error) bool {
	if err := fn(); err != nil {
		logger.Error(ctx, msg, observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return false
	}
	return true
}
