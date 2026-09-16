package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/stretchr/testify/require"
)

// Firebase documents credential failures in error.message under HTTP 400, not
// just 401/403. Other bad requests must retain HTTPX's original classification.
// https://firebase.google.com/docs/reference/rest/auth#section-refresh-token
func TestFirebaseRefreshTokenCredentialRejection(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		auth       bool
	}{
		{"expired", firebaseRefreshErrorBody("TOKEN_EXPIRED"), true},
		{"invalid token", firebaseRefreshErrorBody("INVALID_REFRESH_TOKEN"), true},
		{"disabled user", firebaseRefreshErrorBody("USER_DISABLED"), true},
		{"deleted user", firebaseRefreshErrorBody("USER_NOT_FOUND"), true},
		{"invalid grant", firebaseRefreshErrorBody("INVALID_GRANT_TYPE"), false},
		{"missing token", firebaseRefreshErrorBody("MISSING_REFRESH_TOKEN"), false},
		{"unknown code", firebaseRefreshErrorBody("UNKNOWN_ERROR"), false},
		{"code substring is not a credential code", firebaseRefreshErrorBody("INVALID_GRANT_TYPE: TOKEN_EXPIRED"), false},
		{"malformed envelope", `{"error":`, false},
		{"missing message", `{"error":{"code":400}}`, false},
		{"wrong message type", `{"error":{"code":400,"message":["TOKEN_EXPIRED"]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/token" || r.Method != http.MethodPost {
					t.Errorf("unexpected refresh request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			auth := NewFirebaseAuth("fixture-key", WithTokenBaseURL(server.URL))
			response, err := auth.RefreshToken(context.Background(), "fixture-refresh")
			require.Error(t, err)
			require.Nil(t, response)
			require.Equal(t, tc.auth, apperrors.HasErrorCode(err, apperrors.ErrCodeProviderAuth), "%v", err)
			if !tc.auth {
				require.True(t, apperrors.HasErrorCode(err, apperrors.ErrCodeProviderInvalidReq))
			}
			var upstream *httpx.UpstreamError
			require.ErrorAs(t, err, &upstream, "retain original HTTP error context")
			require.Equal(t, http.StatusBadRequest, upstream.StatusCode)
			require.Equal(t, int32(1), calls.Load(), "credential failures are not retried")
		})
	}
}

func firebaseRefreshErrorBody(message string) string {
	encoded, _ := json.Marshal(message)
	return fmt.Sprintf(`{"error":{"code":400,"message":%s,"errors":[{"domain":"global","reason":"invalid","message":%s}]}}`, encoded, encoded)
}
