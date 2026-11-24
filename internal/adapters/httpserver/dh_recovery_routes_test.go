package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDHInventoryRecoveryRoutesWithoutAuth(t *testing.T) {
	for _, action := range []string{"relink-returned", "release-void"} {
		t.Run(action, func(t *testing.T) {
			mux := setupTestRouter(t).Setup()
			r := httptest.NewRequest(http.MethodPost, "/api/admin/purchases/purchase/dh-recovery/"+action, strings.NewReader(`{"confirm":true}`))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.Contains(t, w.Body.String(), "Authentication not configured")
		})
	}
}
