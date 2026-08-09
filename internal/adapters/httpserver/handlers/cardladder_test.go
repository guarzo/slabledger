package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/cardladder"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// These tests exist because the handler now depends on the CardLadderStore
// seam rather than *postgres.CardLadderStore: every store-backed endpoint is
// reachable from a unit test without a database (SLA-92).

func newCLHandler(store CardLadderStore) *CardLadderHandler {
	return &CardLadderHandler{store: store, logger: mocks.NewMockLogger()}
}

func TestHandleStatus(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name       string
		store      *mocks.CardLadderStoreMock
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "not configured reports configured=false",
			store:      &mocks.CardLadderStoreMock{},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				if body["configured"] != false {
					t.Errorf("configured = %v, want false", body["configured"])
				}
				if _, ok := body["email"]; ok {
					t.Error("email should be absent when not configured")
				}
			},
		},
		{
			name: "configured reports email, collection, mapping count and price stats",
			store: &mocks.CardLadderStoreMock{
				GetConfigFn: func(context.Context) (*cardladder.Config, error) {
					return &cardladder.Config{Email: "cl@example.com", CollectionID: "col-1"}, nil
				},
				ListMappingsFn: func(context.Context) ([]cardladder.CardMapping, error) {
					return []cardladder.CardMapping{{SlabSerial: "111"}, {SlabSerial: "222"}}, nil
				},
				GetCLPriceStatsFn: func(context.Context) (*cardladder.PriceStats, error) {
					return &cardladder.PriceStats{UnsoldTotal: 7, WithCLValue: 4}, nil
				},
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				if body["configured"] != true {
					t.Errorf("configured = %v, want true", body["configured"])
				}
				if body["email"] != "cl@example.com" {
					t.Errorf("email = %v", body["email"])
				}
				if body["collectionId"] != "col-1" {
					t.Errorf("collectionId = %v", body["collectionId"])
				}
				if body["cardsMapped"] != float64(2) {
					t.Errorf("cardsMapped = %v, want 2", body["cardsMapped"])
				}
				stats, ok := body["priceStats"].(map[string]any)
				if !ok {
					t.Fatalf("priceStats = %v, want object", body["priceStats"])
				}
				if stats["unsoldTotal"] != float64(7) {
					t.Errorf("priceStats.unsoldTotal = %v, want 7", stats["unsoldTotal"])
				}
			},
		},
		{
			name: "a failing mapping list degrades the field, not the response",
			store: &mocks.CardLadderStoreMock{
				GetConfigFn: func(context.Context) (*cardladder.Config, error) {
					return &cardladder.Config{Email: "cl@example.com"}, nil
				},
				ListMappingsFn: func(context.Context) ([]cardladder.CardMapping, error) {
					return nil, errBoom
				},
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				if _, ok := body["cardsMapped"]; ok {
					t.Error("cardsMapped should be omitted when ListMappings fails")
				}
			},
		},
		{
			name: "a failing config read is a 500",
			store: &mocks.CardLadderStoreMock{
				GetConfigFn: func(context.Context) (*cardladder.Config, error) { return nil, errBoom },
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newCLHandler(tt.store).HandleStatus(rec, httptest.NewRequest(http.MethodGet, "/admin/cardladder/status", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.check == nil {
				return
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			tt.check(t, body)
		})
	}
}

func TestHandleFailures(t *testing.T) {
	tests := []struct {
		name       string
		store      *mocks.CardLadderStoreMock
		wantStatus int
		wantBody   string
	}{
		{
			name: "report is passed through unchanged",
			store: &mocks.CardLadderStoreMock{
				GetCLFailuresFn: func(context.Context, int) (*cardladder.IntegrationFailuresReport, error) {
					return &cardladder.IntegrationFailuresReport{
						ByReason: map[string]int{"no_match": 3},
						Samples: []cardladder.IntegrationFailureSample{
							{PurchaseID: "p1", CertNumber: "111", Reason: "no_match"},
						},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"byReason":{"no_match":3},"samples":[{"purchaseId":"p1","certNumber":"111","cardName":"","reason":"no_match","errorAt":""}]}`,
		},
		{
			name: "store error is a 500",
			store: &mocks.CardLadderStoreMock{
				GetCLFailuresFn: func(context.Context, int) (*cardladder.IntegrationFailuresReport, error) {
					return nil, errors.New("boom")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newCLHandler(tt.store).HandleFailures(rec, httptest.NewRequest(http.MethodGet, "/admin/cardladder/failures", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" {
				if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
					t.Errorf("body = %s\nwant  %s", got, tt.wantBody)
				}
			}
		})
	}
}

func TestHandleCoverage(t *testing.T) {
	tests := []struct {
		name       string
		store      *mocks.CardLadderStoreMock
		wantStatus int
		wantEra    string
	}{
		{
			name: "report is passed through unchanged",
			store: &mocks.CardLadderStoreMock{
				GetCLCoverageByMonthFn: func(context.Context) (*cardladder.CoverageReport, error) {
					return &cardladder.CoverageReport{
						EraStart: "2026-04-13T04:00:13Z",
						Months:   []cardladder.CoverageMonth{{Month: "2026-08"}},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantEra:    "2026-04-13T04:00:13Z",
		},
		{
			name: "store error is a 500",
			store: &mocks.CardLadderStoreMock{
				GetCLCoverageByMonthFn: func(context.Context) (*cardladder.CoverageReport, error) {
					return nil, errors.New("boom")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newCLHandler(tt.store).HandleCoverage(rec, httptest.NewRequest(http.MethodGet, "/admin/cardladder/coverage", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantEra == "" {
				return
			}
			var report cardladder.CoverageReport
			if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if report.EraStart != tt.wantEra {
				t.Errorf("eraStart = %q, want %q", report.EraStart, tt.wantEra)
			}
			if len(report.Months) != 1 || report.Months[0].Month != "2026-08" {
				t.Errorf("months = %+v", report.Months)
			}
		})
	}
}
