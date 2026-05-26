package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

func TestDHErrorStatus(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMsgPart string
	}{
		{
			name:        "422 client passthrough",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /sync", StatusCode: 422, Message: "No active channel"},
			wantStatus:  http.StatusUnprocessableEntity,
			wantMsgPart: "No active channel",
		},
		{
			name:        "409 client passthrough",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /sync", StatusCode: 409, Message: "conflict"},
			wantStatus:  http.StatusConflict,
			wantMsgPart: "conflict",
		},
		{
			name:        "404 client passthrough",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "GET /x", StatusCode: 404, Message: "not found"},
			wantStatus:  http.StatusNotFound,
			wantMsgPart: "not found",
		},
		{
			name:        "401 remapped to 502",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /x", StatusCode: 401, Message: "bad creds"},
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "DH auth failed",
		},
		{
			name:        "403 remapped to 502",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /x", StatusCode: 403, Message: "forbidden"},
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "DH auth failed",
		},
		{
			name:        "500 mapped to 502",
			err:         &httpx.UpstreamError{Provider: "dh", Op: "POST /x", StatusCode: 500, Message: "boom"},
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "boom",
		},
		{
			name:        "non-upstream error becomes 502",
			err:         errors.New("dial tcp: timeout"),
			wantStatus:  http.StatusBadGateway,
			wantMsgPart: "dial tcp: timeout",
		},
		{
			name:        "wrapped upstream error is unwrapped",
			err:         fmt.Errorf("op failed: %w", &httpx.UpstreamError{Provider: "dh", Op: "POST /sync", StatusCode: 422, Message: "No active channel"}),
			wantStatus:  http.StatusUnprocessableEntity,
			wantMsgPart: "No active channel",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMsg := dhErrorStatus(tt.err)
			if gotStatus != tt.wantStatus {
				t.Errorf("status = %d, want %d", gotStatus, tt.wantStatus)
			}
			if !strings.Contains(gotMsg, tt.wantMsgPart) {
				t.Errorf("msg = %q, want it to contain %q", gotMsg, tt.wantMsgPart)
			}
		})
	}
}
