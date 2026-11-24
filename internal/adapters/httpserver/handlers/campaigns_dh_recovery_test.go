package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDHInventoryRecoveryHandler(t *testing.T) {
	for _, tt := range []struct {
		name, body                         string
		user, admin, unavailable, conflict bool
		want                               int
	}{
		{"unauthenticated", `{"confirm":true}`, false, false, false, false, 401},
		{"nonadmin", `{"confirm":true}`, true, false, false, false, 403},
		{"absent confirmation", `{}`, true, true, false, false, 400},
		{"false confirmation", `{"confirm":false}`, true, true, false, false, 400},
		{"wrong type", `{"confirm":"true"}`, true, true, false, false, 400},
		{"caller target", `{"confirm":true,"dhInventoryId":99}`, true, true, false, false, 400},
		{"caller actor", `{"confirm":true,"actor":"other"}`, true, true, false, false, 400},
		{"observed state", `{"confirm":true,"status":"listed"}`, true, true, false, false, 400},
		{"trailing body", `{"confirm":true} {}`, true, true, false, false, 400},
		{"not configured", `{"confirm":true}`, true, true, true, false, 503},
		{"blocked", `{"confirm":true}`, true, true, false, true, 409},
		{"confirmed", `{"confirm":true}`, true, true, false, false, 204},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := []CampaignsHandlerOption{}
			if !tt.unavailable {
				opts = append(opts, WithDHInventoryRecovery(&mocks.DHInventoryRecoveryMock{RecoverFn: func(_ context.Context, id, action, actor string) error {
					require.Equal(t, "purchase", id)
					require.Equal(t, "relink-returned", action)
					require.Equal(t, "17", actor)
					if tt.want != 204 && !tt.conflict {
						t.Fatal("invalid request reached recovery")
					}
					if tt.conflict {
						return inventory.NewReturnConflict("dh_recovery_blocked", "Recovery blocked")
					}
					return nil
				}}))
			}
			h := NewCampaignsHandler(nil, nil, nil, nil, mocks.NewMockLogger(), nil, opts...)
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			r.SetPathValue("purchaseId", "purchase")
			r.SetPathValue("action", "relink-returned")
			if tt.user {
				r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.User{ID: 17, IsAdmin: tt.admin}))
			}
			w := httptest.NewRecorder()
			h.HandleDHInventoryRecovery(w, r)
			require.Equal(t, tt.want, w.Code, w.Body.String())
		})
	}
}
