package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

const scanListID = "11111111-1111-4111-8111-111111111111"
const scanCommandID = "22222222-2222-4222-8222-222222222222"
const scanPurchaseID = "33333333-3333-4333-8333-333333333333"

func scanAPI(t *testing.T, store *mocks.ShowPrepStoreMock, method, suffix, body string) *httptest.ResponseRecorder {
	t.Helper()
	logger := mocks.NewMockLogger()
	source := &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
		t.Fatal("API must never acquire comps")
		return sp.Snapshot{}, nil
	}}
	h := handlers.NewShowPrepHandler(sp.NewService(store, source, func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }), logger)
	router := httpserver.NewRouter(httpserver.RouterConfig{ShowPrepHandler: h, LocalAPIToken: "fixture", Logger: logger,
		SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
	r := httptest.NewRequest(method, "/api/show-prep/lists/"+suffix, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer fixture")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	return w
}

// Catches HTTP accepting malformed immutable intents or normalizing away cert identity.
func TestShowPrepScanBoundary(t *testing.T) {
	for _, tt := range []struct {
		name, list, body string
		status           int
		cert             string
	}{
		{"valid padded alphanumeric", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"  00123AB  ","withdrawalGeneration":0}`, 200, "00123AB"},
		{"128 bytes", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"` + strings.Repeat("a", 128) + `","withdrawalGeneration":0}`, 200, strings.Repeat("a", 128)},
		{"UTF8 128 bytes", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"` + strings.Repeat("界", 42) + `ab","withdrawalGeneration":0}`, 200, strings.Repeat("界", 42) + "ab"},
		{"129 bytes", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"` + strings.Repeat("a", 129) + `","withdrawalGeneration":0}`, 400, ""},
		{"UTF8 over limit", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"` + strings.Repeat("界", 43) + `","withdrawalGeneration":0}`, 400, ""},
		{"empty cert", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"  ","withdrawalGeneration":0}`, 400, ""},
		{"bad command UUID", scanListID, `{"commandId":"bad","certNumber":"123","withdrawalGeneration":0}`, 400, ""},
		{"noncanonical command UUID", scanListID, `{"commandId":"22222222222242228222222222222222","certNumber":"123","withdrawalGeneration":0}`, 400, ""},
		{"bad list UUID", "bad", `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":0}`, 400, ""},
		{"negative generation", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":-1}`, 400, ""},
		{"unsafe generation", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":9007199254740992}`, 400, ""},
		{"safe maximum", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":9007199254740991}`, 200, "123"},
		{"fractional generation", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":0.5}`, 400, ""},
		{"null generation", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":null}`, 400, ""},
		{"missing generation", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123"}`, 400, ""},
		{"unknown field", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":0,"price":1}`, 400, ""},
		{"trailing body", scanListID, `{"commandId":"` + scanCommandID + `","certNumber":"123","withdrawalGeneration":0} {}`, 400, ""},
		{"oversized body", scanListID, `{"certNumber":"` + strings.Repeat("a", 128<<10) + `"}`, 400, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			receipts := 0
			m := &mocks.ShowPrepStoreMock{WithinFn: func(_ context.Context, fn func(sp.Session) error) error {
				if tt.status == 400 {
					t.Fatal("invalid body must not transact")
				}
				return fn(mustSession(t, tt.cert, &receipts))
			}}
			w := scanAPI(t, m, http.MethodPost, tt.list+"/scan-confirm", tt.body)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			if tt.status == 200 {
				var result sp.ScanResult
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.Equal(t, tt.cert, result.Original.CertNumber)
				require.Equal(t, "rejected", result.Original.Outcome)
				require.Equal(t, 1, receipts)
			}
		})
	}
}

func mustSession(t *testing.T, cert string, receipts *int) *mocks.ShowPrepStoreMock {
	t.Helper()
	return &mocks.ShowPrepStoreMock{ReadCertPurchaseIDsFn: func(_ context.Context, got string) ([]string, error) {
		require.Equal(t, cert, got)
		return nil, nil
	}, InsertCommandReceiptFn: func(_ context.Context, r sp.CommandReceipt) error {
		*receipts++
		require.Equal(t, cert, r.Scan.CertNumber)
		return nil
	}}
}

// Catches converting terminal results to transport errors or amending approval on replay.
func TestShowPrepScanOriginalAndCurrent(t *testing.T) {
	for _, tt := range []struct {
		name, outcome   string
		packed          bool
		baseline, price int
		removed         bool
	}{
		{"new confirmation", "confirmed", false, 0, 30000, false},
		{"no asking or comps", "confirmed", false, 0, 0, false},
		{"already confirmed", "already_confirmed", true, 30000, 30000, false},
		{"rechecked", "rechecked", true, 30000, 25000, false},
		{"removed replay", "confirmed", false, 0, 30000, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := sp.Purchase{ID: scanPurchaseID, CardName: "Card", CertNumber: "00123AB", Grader: "PSA", Grade: 10, Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "pending", LocalPriceCents: tt.price}
			items := []sp.Item{}
			if tt.packed {
				items = append(items, sp.Item{ID: scanPurchaseID, PurchaseID: scanPurchaseID, CardName: "Card", CertNumber: p.CertNumber, Grader: "PSA", Grade: 10, PackedAt: "2026-10-01T12:00:00Z", Version: 2, AcknowledgedPriceCents: tt.baseline, AcknowledgedStatus: sp.NeedsReview})
			}
			var receipt *sp.CommandReceipt
			writes := 0
			m := &mocks.ShowPrepStoreMock{
				ReadCertPurchaseIDsFn: func(context.Context, string) ([]string, error) { return []string{scanPurchaseID}, nil },
				ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
					return map[string]sp.Purchase{scanPurchaseID: p}, nil
				},
				GetItemsFn:   func(context.Context, string) ([]sp.Item, error) { return items, nil },
				InsertItemFn: func(_ context.Context, _ string, i sp.Item) error { items = append(items, i); writes++; return nil },
				SaveItemFn:   func(_ context.Context, _ string, i sp.Item) error { items[0] = i; writes++; return nil },
				GetCommandReceiptFn: func(context.Context, string) (sp.CommandReceipt, bool, error) {
					if receipt != nil {
						return *receipt, true, nil
					}
					return sp.CommandReceipt{}, false, nil
				},
				InsertCommandReceiptFn: func(_ context.Context, r sp.CommandReceipt) error { receipt = &r; return nil },
			}
			body := `{"commandId":"` + scanCommandID + `","certNumber":" 00123AB ","withdrawalGeneration":0}`
			w := scanAPI(t, m, "POST", scanListID+"/scan-confirm", body)
			require.Equal(t, 200, w.Code, w.Body.String())
			var original sp.ScanResult
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &original))
			require.Equal(t, tt.outcome, original.Original.Outcome)
			require.Equal(t, "", original.Original.Code)
			require.Equal(t, tt.price, original.Original.AcknowledgedPriceCents)
			require.NotEmpty(t, original.Original.ItemID)
			require.Equal(t, scanCommandID, original.Original.CommandID)
			require.Equal(t, scanListID, original.Original.ListID)
			require.Equal(t, scanPurchaseID, original.Original.PurchaseID)
			require.Equal(t, "Card", original.Original.CardName)
			require.Equal(t, "present", original.Current.State)
			require.NotNil(t, original.Current.Evaluation)
			require.False(t, original.Replay)
			priorWrites := writes
			p.LocalPriceCents = 99000
			if tt.removed {
				items = nil
			}
			// Whitespace normalization must produce the same immutable payload hash.
			body = strings.Replace(body, " 00123AB ", "00123AB", 1)
			w = scanAPI(t, m, "POST", scanListID+"/scan-confirm", body)
			require.Equal(t, 200, w.Code, w.Body.String())
			var replay sp.ScanResult
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &replay))
			require.Equal(t, original.Original, replay.Original)
			require.True(t, replay.Replay)
			want := "present"
			if tt.removed {
				want = "removed"
			}
			require.Equal(t, want, replay.Current.State)
			require.Equal(t, priorWrites, writes, "replay cannot amend confirmation")
			w = scanAPI(t, m, "POST", scanPurchaseID+"/scan-confirm", body)
			require.Equal(t, 409, w.Code, "command payload is bound to the destination")
			changed := strings.Replace(body, "00123AB", "other-cert", 1)
			w = scanAPI(t, m, "POST", scanListID+"/scan-confirm", changed)
			require.Equal(t, 409, w.Code, w.Body.String())
			require.Equal(t, priorWrites, writes)
		})
	}
}

// Catches infrastructure logs leaking a body, credential or downstream error payload.
func TestShowPrepScanSafeFailureLogging(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		outcome string
		status  int
	}{
		{"storage", errors.New("secret-downstream-payload"), "storage_failed", 500},
		{"contention", sp.ErrContended, "contended", 503},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := mocks.NewCapturingLogger()
			m := &mocks.ShowPrepStoreMock{WithinFn: func(context.Context, func(sp.Session) error) error { return tt.err }}
			h := handlers.NewShowPrepHandler(sp.NewService(m, nil, time.Now), logger)
			r := httptest.NewRequest("POST", "/api/show-prep/lists/"+scanListID+"/scan-confirm", strings.NewReader(`{"commandId":"`+scanCommandID+`","certNumber":"secret-cert","withdrawalGeneration":0}`))
			r.SetPathValue("listID", scanListID)
			r.Header.Set("Authorization", "Bearer secret-token")
			w := httptest.NewRecorder()
			h.HandleScanConfirm(w, r)
			require.Equal(t, tt.status, w.Code)
			entries := logger.Entries()
			require.Len(t, entries, 1)
			for key, want := range map[string]string{"list_id": scanListID, "command_id": scanCommandID, "operation": "scan", "outcome": tt.outcome} {
				got, ok := entries[0].FindField(key)
				require.True(t, ok)
				require.Equal(t, want, got)
			}
			encoded := fmt.Sprint(entries)
			require.NotContains(t, encoded, "secret-")
			require.NotContains(t, w.Body.String(), "secret-")
		})
	}
}

func TestShowPrepScanTerminalAndFailures(t *testing.T) {
	for _, tt := range []struct {
		name       string
		err        error
		status     int
		retired    bool
		generation int64
		code       string
		purchase   *sp.Purchase
		candidates []string
	}{
		{name: "no match", status: 200, code: "no_match"},
		{name: "old generation", status: 200, generation: 1, code: "contents_changed"},
		{name: "missing", err: sp.ErrNotFound, status: 404},
		{name: "retired", err: sp.ErrNotFound, status: 410, retired: true},
		{name: "contention", err: fmt.Errorf("lock: %w", sp.ErrContended), status: 503, code: "scan_contended"},
		{name: "storage", err: context.DeadlineExceeded, status: 500},
		{name: "ambiguous", status: 200, code: "ambiguous_cert", candidates: []string{scanPurchaseID, scanCommandID}},
		{name: "not received", status: 200, code: "not_received", purchase: &sp.Purchase{Known: true, Exists: true, CampaignExists: true, Phase: "active"}},
		{name: "sold", status: 200, code: "sold", purchase: &sp.Purchase{Known: true, Exists: true, CampaignExists: true, Sold: true}},
		{name: "refunded", status: 200, code: "refunded", purchase: &sp.Purchase{Known: true, Exists: true, CampaignExists: true, Refunded: true}},
		{name: "closed", status: 200, code: "campaign_closed", purchase: &sp.Purchase{Known: true, Exists: true, CampaignExists: true, Phase: "closed"}},
		{name: "removed", status: 200, code: "removed", purchase: &sp.Purchase{Known: true}},
		{name: "unknown", status: 200, code: "unknown_availability", purchase: &sp.Purchase{}},
		{name: "identity changed", status: 200, code: "identity_changed", purchase: &sp.Purchase{Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "active", CertNumber: "other"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			receipts := 0
			var saved *sp.CommandReceipt
			m := &mocks.ShowPrepStoreMock{
				GetCommandReceiptFn: func(context.Context, string) (sp.CommandReceipt, bool, error) {
					if saved != nil {
						return *saved, true, nil
					}
					return sp.CommandReceipt{}, false, nil
				},
				GetListFn: func(context.Context, string) (sp.List, error) {
					return sp.List{ID: scanListID, WithdrawalGeneration: tt.generation}, tt.err
				},
				GetRetiredListFn: func(context.Context, string) (sp.RetiredList, bool, error) {
					return sp.RetiredList{ListID: scanListID}, tt.retired, nil
				},
				InsertCommandReceiptFn: func(_ context.Context, receipt sp.CommandReceipt) error { receipts++; saved = &receipt; return nil },
			}
			if tt.purchase != nil {
				p := *tt.purchase
				p.ID = scanPurchaseID
				if p.CertNumber == "" {
					p.CertNumber = "123"
				}
				m.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
					return map[string]sp.Purchase{scanPurchaseID: p}, nil
				}
				m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) { return []string{scanPurchaseID}, nil }
			} else if tt.candidates != nil {
				m.ReadCertPurchaseIDsFn = func(context.Context, string) ([]string, error) { return tt.candidates, nil }
			}
			w := scanAPI(t, m, "POST", scanListID+"/scan-confirm", `{"commandId":"`+scanCommandID+`","certNumber":"123","withdrawalGeneration":0}`)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			if tt.status == 200 {
				var result sp.ScanResult
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.Equal(t, "rejected", result.Original.Outcome)
				require.Equal(t, tt.code, result.Original.Code)
				require.Equal(t, 1, receipts)
				m.GetListFn = func(context.Context, string) (sp.List, error) {
					t.Fatal("terminal receipt must replay before live state")
					return sp.List{}, sp.ErrNotFound
				}
				w = scanAPI(t, m, "POST", scanListID+"/scan-confirm", `{"commandId":"`+scanCommandID+`","certNumber":"123","withdrawalGeneration":0}`)
				require.Equal(t, 200, w.Code, w.Body.String())
				var replay sp.ScanResult
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &replay))
				require.True(t, replay.Replay)
				require.Equal(t, result.Original, replay.Original)
				require.Equal(t, 1, receipts)
			} else {
				require.Zero(t, receipts)
				var result map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.NotContains(t, result, "original")
				if tt.status == 503 {
					require.Equal(t, "scan_contended", result["code"])
					require.Equal(t, true, result["retryable"])
				}
			}
		})
	}
}
