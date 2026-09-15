package handlers

import (
	"context"
	"encoding/json"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShowPrepHTTPPackingRetriesAndStaleAcknowledgment(t *testing.T) {
	p := sp.Purchase{ID: showTestID, CardName: "Card", CertNumber: "123", Grader: "PSA", Grade: 10, Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "pending", ListedPriceCents: 30000}
	item := sp.Item{ID: "22222222-2222-4222-8222-222222222222", PurchaseID: showTestID, Version: 1, AcknowledgedPriceCents: 30000, AcknowledgedStatus: sp.NeedsReview}
	writes := 0
	store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
		return map[string]sp.Purchase{showTestID: p}, nil
	}, GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return []sp.Item{item}, nil }, SaveItemFn: func(_ context.Context, _ string, saved sp.Item) error { item = saved; writes++; return nil }}
	svc := sp.NewService(store, nil, time.Now)
	h := NewShowPrepHandler(svc, mocks.NewMockLogger())
	es, err := svc.Evaluate(context.Background(), []string{showTestID})
	require.NoError(t, err)
	yes, no := true, false
	pack := sp.UpdateItem{Version: 1, EvaluationVersion: es[0].Version, Packed: &yes}
	for _, tt := range []struct {
		name           string
		cmd            sp.UpdateItem
		price          int
		status, writes int
	}{
		{"pack", pack, 30000, 200, 1}, {"duplicate transport request", pack, 30000, 200, 1},
		{"stale conflicting intent", sp.UpdateItem{Version: 1, Packed: &no}, 30000, 409, 1},
		{"stale acknowledgment after price sync", sp.UpdateItem{Version: 2, EvaluationVersion: es[0].Version, Acknowledge: true}, 40000, 409, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p.ListedPriceCents = tt.price
			body, err := json.Marshal(tt.cmd)
			require.NoError(t, err)
			r := httptest.NewRequest("PUT", "/api/show-prep/lists/list/items/item", strings.NewReader(string(body)))
			r.SetPathValue("listID", showTestID)
			r.SetPathValue("itemID", item.ID)
			w := httptest.NewRecorder()
			h.HandleUpdateItem(w, r)
			require.Equal(t, tt.status, w.Code)
			require.Equal(t, tt.writes, writes)
			if tt.status == 200 {
				var result sp.ListDetail
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.Len(t, result.Items, 1)
				require.Equal(t, 1, result.Summary.PackedCount)
				require.Equal(t, int64(2), result.Items[0].Version)
			}
		})
	}
}
func TestShowPrepHTTPBoundedInputs(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		refresh    bool
		status     int
	}{
		{"oversized body", `{"purchaseIds":["` + strings.Repeat("a", 128<<10) + `"]}`, false, 400},
		{"evaluate oversized batch", `{"purchaseIds":[` + strings.TrimSuffix(strings.Repeat(`"`+showTestID+`",`, 201), ",") + `]}`, false, 400},
		{"refresh retired regardless of batch", `{"purchaseIds":[` + strings.TrimSuffix(strings.Repeat(`"`+showTestID+`",`, 11), ",") + `]}`, true, 410},
		{"duplicate IDs deduplicate", `{"purchaseIds":["` + showTestID + `","` + showTestID + `"]}`, false, 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := NewShowPrepHandler(sp.NewService(&mocks.ShowPrepStoreMock{}, nil, time.Now), mocks.NewMockLogger())
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/show-prep/evaluate", strings.NewReader(tt.body))
			if tt.refresh {
				h.HandleRefresh(w, r)
			} else {
				h.HandleEvaluate(w, r)
			}
			require.Equal(t, tt.status, w.Code)
			if tt.status == 200 {
				var result struct {
					Evaluations []sp.Evaluation `json:"evaluations"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.Len(t, result.Evaluations, 1)
			}
		})
	}
	for _, tt := range []struct {
		name   string
		status int
	}{{" ", 400}, {strings.Repeat("界", 121), 400}, {strings.Repeat("界", 120), 200}} {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.ShowPrepStoreMock{CreateListFn: func(_ context.Context, id, name string) (sp.List, error) { return sp.List{ID: id, Name: name}, nil }}
			h := NewShowPrepHandler(sp.NewService(store, nil, time.Now), mocks.NewMockLogger())
			body, _ := json.Marshal(map[string]string{"id": showTestID, "name": tt.name})
			w := httptest.NewRecorder()
			h.HandleCreateList(w, httptest.NewRequest("POST", "/api/show-prep/lists", strings.NewReader(string(body))))
			require.Equal(t, tt.status, w.Code)
		})
	}
}
