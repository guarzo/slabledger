package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Catches stale whole-show deletion and replay bypassing the durable tombstone.
func TestShowPrepDeleteList(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stale  bool
		member bool
	}{
		{"empty", false, false}, {"members", false, true}, {"stale contents", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			list := sp.List{ID: scanListID, Name: "Show", WithdrawalGeneration: 2}
			items := []sp.Item{}
			if tt.member {
				items = append(items, sp.Item{ID: scanPurchaseID, PurchaseID: scanPurchaseID, Version: 1})
			}
			state := sp.ListStateFingerprint(list, items)
			if tt.stale {
				items[0].Version++
			}
			var tombstone *sp.RetiredList
			m := &mocks.ShowPrepStoreMock{
				GetListFn:  func(context.Context, string) (sp.List, error) { return list, nil },
				GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return items, nil },
				GetRetiredListFn: func(context.Context, string) (sp.RetiredList, bool, error) {
					if tombstone != nil {
						return *tombstone, true, nil
					}
					return sp.RetiredList{}, false, nil
				},
				RetireListFn: func(_ context.Context, r sp.RetiredList) error { tombstone = &r; items = nil; return nil },
			}
			cmd := sp.DeleteListCommand{CommandID: scanCommandID, ExpectedState: state}
			body, err := json.Marshal(cmd)
			require.NoError(t, err)
			w := scanAPI(t, m, http.MethodDelete, scanListID, string(body))
			if tt.stale {
				require.Equal(t, 409, w.Code, w.Body.String())
				require.Nil(t, tombstone)
				require.Len(t, items, 1)
				return
			}
			require.Equal(t, 200, w.Code, w.Body.String())
			require.JSONEq(t, `{"listId":"`+scanListID+`","deleted":true,"replay":false}`, w.Body.String())
			require.NotNil(t, tombstone)
			require.Empty(t, items)
			m.GetListFn = func(context.Context, string) (sp.List, error) {
				t.Fatal("replay cannot inspect live state")
				return sp.List{}, sp.ErrNotFound
			}
			m.RetireListFn = func(context.Context, sp.RetiredList) error { t.Fatal("replay cannot retire twice"); return nil }
			w = scanAPI(t, m, http.MethodDelete, scanListID, string(body))
			require.Equal(t, 200, w.Code, w.Body.String())
			require.JSONEq(t, `{"listId":"`+scanListID+`","deleted":true,"replay":true}`, w.Body.String())
			cmd.ExpectedState = "changed"
			body, err = json.Marshal(cmd)
			require.NoError(t, err)
			w = scanAPI(t, m, http.MethodDelete, scanListID, string(body))
			require.Equal(t, 409, w.Code, w.Body.String())
			cmd.CommandID = scanPurchaseID
			body, err = json.Marshal(cmd)
			require.NoError(t, err)
			w = scanAPI(t, m, http.MethodDelete, scanListID, string(body))
			require.Equal(t, 410, w.Code, w.Body.String())
		})
	}
}

func TestShowPrepDeleteBoundary(t *testing.T) {
	for _, tt := range []struct{ name, list, body string }{
		{"bad list", "bad", `{"commandId":"` + scanCommandID + `","expectedState":"state"}`},
		{"bad command", scanListID, `{"commandId":"bad","expectedState":"state"}`},
		{"empty state", scanListID, `{"commandId":"` + scanCommandID + `","expectedState":""}`},
		{"padded state", scanListID, `{"commandId":"` + scanCommandID + `","expectedState":" state "}`},
		{"unknown field", scanListID, `{"commandId":"` + scanCommandID + `","expectedState":"state","itemIds":[]}`},
		{"trailing body", scanListID, `{"commandId":"` + scanCommandID + `","expectedState":"state"} {}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &mocks.ShowPrepStoreMock{WithinFn: func(context.Context, func(sp.Session) error) error {
				t.Fatal("invalid deletion must not transact")
				return nil
			}}
			w := scanAPI(t, m, http.MethodDelete, tt.list, tt.body)
			require.Equal(t, 400, w.Code, w.Body.String())
		})
	}
}

func TestShowPrepDeleteFailures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{
		{"missing", sp.ErrNotFound, 404}, {"contention", sp.ErrContended, 503}, {"storage", context.DeadlineExceeded, 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &mocks.ShowPrepStoreMock{WithinFn: func(context.Context, func(sp.Session) error) error { return tt.err }}
			w := scanAPI(t, m, http.MethodDelete, scanListID, `{"commandId":"`+scanCommandID+`","expectedState":"state"}`)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			if tt.status == 503 {
				var body struct {
					Code      string `json:"code"`
					Retryable bool   `json:"retryable"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Equal(t, "show_contended", body.Code)
				require.True(t, body.Retryable)
			}
		})
	}
}
