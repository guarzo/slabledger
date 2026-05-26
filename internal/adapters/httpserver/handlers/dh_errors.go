package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

// dhErrorStatus inspects err for an *httpx.UpstreamError and maps it to an
// HTTP status + user-facing message suitable for writeError.
//
// Routing rules:
//   - DH 4xx (except 401/403) → pass the status through (422, 409, 404, 400,
//     429, …). These are logical rejections the user can act on.
//   - DH 401/403 → 502. These are OUR credentials being bad, not the user's
//     session; surfacing 401/403 would let auth middleware/UI mistake it for
//     a session problem.
//   - DH 5xx → 502. DH is broken or unreachable.
//   - Anything else (no UpstreamError in the chain — network, timeout, parse
//     failure) → 502 with the raw error message.
func dhErrorStatus(err error) (int, string) {
	var ue *httpx.UpstreamError
	if errors.As(err, &ue) {
		msg := ue.Message
		if msg == "" {
			msg = ue.Body
		}
		switch {
		case ue.StatusCode == http.StatusUnauthorized || ue.StatusCode == http.StatusForbidden:
			return http.StatusBadGateway, fmt.Sprintf("DH auth failed (status %d): %s", ue.StatusCode, msg)
		case ue.IsClientError():
			return ue.StatusCode, fmt.Sprintf("DH %s: %s", ue.Op, msg)
		default:
			return http.StatusBadGateway, fmt.Sprintf("DH %s failed (status %d): %s", ue.Op, ue.StatusCode, msg)
		}
	}
	return http.StatusBadGateway, err.Error()
}
