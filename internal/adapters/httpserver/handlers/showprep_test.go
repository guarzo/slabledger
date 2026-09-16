package handlers

import (
	"context"
	"encoding/json"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const showTestID = "11111111-1111-4111-8111-111111111111"

func TestShowPrepHTTPContract(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"valid", `{"purchaseIds":["` + showTestID + `"]}`, 200},
		{"missing", `{}`, 400}, {"bad uuid", `{"purchaseIds":["bad"]}`, 400},
		{"unknown field", `{"purchaseIds":["` + showTestID + `"],"price":5}`, 400},
		{"trailing body", `{"purchaseIds":["` + showTestID + `"]} {}`, 400},
		{"empty", `{"purchaseIds":[]}`, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := NewShowPrepHandler(sp.NewService(&mocks.ShowPrepStoreMock{}, nil, time.Now), mocks.NewMockLogger())
			r := httptest.NewRequest(http.MethodPost, "/api/show-prep/evaluate", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.HandleEvaluate(w, r)
			require.Equal(t, tt.status, w.Code)
			if tt.status == 200 {
				var body struct {
					Evaluations []map[string]any `json:"evaluations"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Len(t, body.Evaluations, 1)
				for _, key := range []string{"purchaseId", "cardName", "certNumber", "grader", "grade", "status", "reason", "evidenceNeedsReview", "evidenceReason", "availability", "canAdd", "canPack", "listedPriceCents", "localPriceCents", "priceMismatch", "priceAssociationUnclear", "listingSyncedAt", "medianCents", "compCount", "latestSaleDate", "windowStart", "windowEnd", "refreshedAt", "evidenceVersion", "version", "readiness"} {
					require.Contains(t, body.Evaluations[0], key)
				}
				require.Equal(t, map[string]any{"state": "unavailable", "refreshEligibility": "unavailable", "identityKey": "", "expiresAt": "", "retryAt": ""}, body.Evaluations[0]["readiness"])
			}
		})
	}
}
func TestShowPrepHTTPErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{{"missing", sp.ErrNotFound, 404}, {"stale", sp.ErrConflict, 409}, {"invalid", sp.ErrInvalid, 400}, {"storage", context.DeadlineExceeded, 500}} {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.ShowPrepStoreMock{CreateListFn: func(context.Context, string, string) (sp.List, error) { return sp.List{}, tt.err }}
			h := NewShowPrepHandler(sp.NewService(store, nil, time.Now), mocks.NewMockLogger())
			r := httptest.NewRequest("POST", "/api/show-prep/lists", strings.NewReader(`{"id":"`+showTestID+`","name":"Show"}`))
			w := httptest.NewRecorder()
			h.HandleCreateList(w, r)
			require.Equal(t, tt.status, w.Code)
		})
	}
}
