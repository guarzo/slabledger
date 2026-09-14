package main

import (
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildShowPrepIndependentOfCredentials(t *testing.T) {
	for _, tt := range []struct {
		name string
		db   *postgres.DB
		want bool
	}{{"no DB", nil, false}, {"DB without CL", &postgres.DB{}, true}} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			h := buildShowPrepHandler(handlerInputs{DB: tt.db, Cfg: &cfg, Logger: mocks.NewMockLogger()})
			require.Equal(t, tt.want, h != nil)
			if h != nil {
				w := httptest.NewRecorder()
				h.HandleEvaluate(w, httptest.NewRequest("POST", "/api/show-prep/evaluate", strings.NewReader(`{"purchaseIds":[]}`)))
				require.Equal(t, 400, w.Code)
			}
		})
	}
}
