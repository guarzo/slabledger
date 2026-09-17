package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepPreviewHTTPContract(t *testing.T) {
	valid := `{"purchaseId":"` + showTestID + `","priceCents":240000}`
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"valid", valid, 200},
		{"minimum", strings.Replace(valid, "240000", "1", 1), 200},
		{"maximum", strings.Replace(valid, "240000", "9007199254740991", 1), 200},
		{"missing id", `{"priceCents":240000}`, 400},
		{"noncanonical uuid", strings.Replace(valid, showTestID, "11111111111141118111111111111111", 1), 400},
		{"uppercase uuid", strings.Replace(valid, showTestID, "AAAAAAAA-1111-4111-8111-111111111111", 1), 400},
		{"missing cents", `{"purchaseId":"` + showTestID + `"}`, 400},
		{"zero", strings.Replace(valid, "240000", "0", 1), 400},
		{"negative", strings.Replace(valid, "240000", "-1", 1), 400},
		{"fractional", strings.Replace(valid, "240000", "240000.5", 1), 400},
		{"unsafe", strings.Replace(valid, "240000", "9007199254740992", 1), 400},
		{"overflow", strings.Replace(valid, "240000", "9223372036854775808", 1), 400},
		{"null cents", strings.Replace(valid, "240000", "null", 1), 400},
		{"string cents", strings.Replace(valid, "240000", `"240000"`, 1), 400},
		{"null body", "null", 400}, {"malformed", "{", 400},
		{"unknown field", strings.Replace(valid, "}", `,"version":"token"}`, 1), 400},
		{"trailing object", valid + ` {}`, 400}, {"trailing junk", valid + ` x`, 400},
		{"oversized", `{"purchaseId":"` + strings.Repeat("a", 128<<10) + `","priceCents":1}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
				reads++
				return map[string]sp.Purchase{showTestID: {ID: showTestID, LocalPriceCents: 280000}}, nil
			}}
			h := NewShowPrepHandler(sp.NewService(store, nil, time.Now), mocks.NewMockLogger())
			w := httptest.NewRecorder()
			h.HandlePreview(w, httptest.NewRequest(http.MethodPost, "/api/show-prep/preview", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status != 200 {
				require.Zero(t, reads)
				return
			}
			require.Equal(t, 1, reads)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body, 10)
			for _, key := range []string{"purchaseId", "currentPriceCents", "trialPriceCents", "status", "reason", "evidenceNeedsReview", "evidenceReason", "evidenceVersion", "policyVersion", "recent"} {
				require.Contains(t, body, key)
			}
			for _, key := range []string{"version", "canAdd", "canPack", "evaluation"} {
				require.NotContains(t, body, key)
			}
			require.Equal(t, float64(280000), body["currentPriceCents"])
			var input struct {
				PriceCents int `json:"priceCents"`
			}
			require.NoError(t, json.Unmarshal([]byte(tc.body), &input))
			require.Equal(t, float64(input.PriceCents), body["trialPriceCents"])
		})
	}
}

func TestShowPrepPreviewHTTPReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		status      int
	}{
		{"not found", "missing", 404}, {"purchase storage", "purchase", 500}, {"snapshot storage", "snapshot", 500},
		{"canceled at purchase", "cancel purchase", 500}, {"canceled at snapshot", "cancel snapshot", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(c context.Context, _ []string) (map[string]sp.Purchase, error) {
				require.Same(t, ctx, c)
				if tc.stage == "missing" {
					return nil, nil
				}
				if tc.stage == "purchase" {
					return nil, errors.New("secret postgres password")
				}
				if tc.stage == "cancel purchase" {
					cancel()
					return nil, c.Err()
				}
				return map[string]sp.Purchase{showTestID: {ID: showTestID}}, nil
			}, ReadSnapshotsFn: func(c context.Context, _ []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				require.Same(t, ctx, c)
				if tc.stage == "cancel snapshot" {
					cancel()
					return nil, c.Err()
				}
				return nil, errors.New("secret postgres password")
			}}
			h := NewShowPrepHandler(sp.NewService(store, nil, time.Now), mocks.NewMockLogger())
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/show-prep/preview", strings.NewReader(`{"purchaseId":"`+showTestID+`","priceCents":240000}`)).WithContext(ctx)
			h.HandlePreview(w, r)
			require.Equal(t, tc.status, w.Code)
			require.NotContains(t, w.Body.String(), "secret")
			if tc.status == 500 {
				require.JSONEq(t, `{"error":"Show preparation storage failed"}`, w.Body.String())
			}
		})
	}
}
