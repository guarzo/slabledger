package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// mockReconciler is a test double for dhlisting.Reconciler. The function field
// pattern matches the rest of the mocks/ package.
type mockReconciler struct {
	ReconcileFn func(ctx context.Context) (dhlisting.ReconcileResult, error)
}

func (m *mockReconciler) Reconcile(ctx context.Context) (dhlisting.ReconcileResult, error) {
	if m.ReconcileFn != nil {
		return m.ReconcileFn(ctx)
	}
	return dhlisting.ReconcileResult{}, nil
}

func newReconcileTestHandler(rec dhlisting.Reconciler) *DHHandler {
	return NewDHHandler(DHHandlerDeps{
		Reconciler: rec,
		Logger:     mocks.NewMockLogger(),
		BaseCtx:    context.Background(),
	})
}

// TestHandleReconcile groups the synchronous (non-concurrent) entry-path cases.
// The concurrency case lives in TestHandleReconcile_ConcurrentRunsRejected
// because forcing channel/wg coordination into a table struct hurts readability.
func TestHandleReconcile(t *testing.T) {
	successResult := dhlisting.ReconcileResult{
		Scanned:     50,
		MissingOnDH: 3,
		Reset:       2,
		Errors:      []string{"p9: write conflict"},
		ResetIDs:    []string{"p1", "p2"},
	}

	tests := []struct {
		name string
		// reconciler is the dhlisting.Reconciler injected. nil means "not
		// configured" → 503.
		reconciler dhlisting.Reconciler
		wantCode   int
		// wantBody is non-nil for the success case to verify response shape.
		wantBody *dhlisting.ReconcileResult
	}{
		{
			name:       "reconciler not configured → 503",
			reconciler: nil,
			wantCode:   http.StatusServiceUnavailable,
		},
		{
			name: "success → 200 with full ReconcileResponse shape",
			reconciler: &mockReconciler{
				ReconcileFn: func(_ context.Context) (dhlisting.ReconcileResult, error) {
					return successResult, nil
				},
			},
			wantCode: http.StatusOK,
			wantBody: &successResult,
		},
		{
			name: "service error → 502",
			reconciler: &mockReconciler{
				ReconcileFn: func(_ context.Context) (dhlisting.ReconcileResult, error) {
					return dhlisting.ReconcileResult{}, errors.New("DH 500")
				},
			},
			wantCode: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newReconcileTestHandler(tt.reconciler)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/reconcile", nil)
			h.HandleReconcile(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantBody == nil {
				return
			}
			var got reconcileResponse
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Scanned != tt.wantBody.Scanned ||
				got.MissingOnDH != tt.wantBody.MissingOnDH ||
				got.Reset != tt.wantBody.Reset ||
				len(got.Errors) != len(tt.wantBody.Errors) ||
				len(got.ResetIDs) != len(tt.wantBody.ResetIDs) {
				t.Errorf("response shape mismatch: got %+v, want %+v", got, *tt.wantBody)
			}
		})
	}
}

// TestHandleReconcile_ConcurrentRunsRejected verifies the mutex-protected
// guard: a second concurrent caller gets 409 instead of starting a duplicate
// reconcile run.
func TestHandleReconcile_ConcurrentRunsRejected(t *testing.T) {
	// Block the first reconcile in a goroutine so we can fire the second
	// while the first still holds the mutex.
	started := make(chan struct{})
	release := make(chan struct{})
	svc := &mockReconciler{
		ReconcileFn: func(_ context.Context) (dhlisting.ReconcileResult, error) {
			close(started)
			<-release
			return dhlisting.ReconcileResult{Scanned: 1}, nil
		},
	}
	h := newReconcileTestHandler(svc)

	var wg sync.WaitGroup
	first := httptest.NewRecorder()

	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodPost, "/api/dh/reconcile", nil)
		h.HandleReconcile(first, req)
	}()

	<-started // first caller has the mutex

	second := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dh/reconcile", nil)
	h.HandleReconcile(second, req)

	if second.Code != http.StatusConflict {
		t.Fatalf("second call: got %d, want 409", second.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(second.Body).Decode(&body); err != nil {
		t.Fatalf("decode second body: %v", err)
	}
	if body["status"] != "already_running" {
		t.Errorf("second body: got %v, want already_running", body)
	}

	close(release)
	wg.Wait()

	if first.Code != http.StatusOK {
		t.Errorf("first call: got %d, want 200", first.Code)
	}
}

// mockDHReconcileRunner is a test double for handlers.DHReconcileRunner.
// Uses the Fn-field pattern to match the project's mock conventions.
type mockDHReconcileRunner struct {
	RunOnceFn          func(ctx context.Context) error
	GetLastRunResultFn func() *dhlisting.ReconcileResult
}

func (m *mockDHReconcileRunner) RunOnce(ctx context.Context) error {
	if m.RunOnceFn != nil {
		return m.RunOnceFn(ctx)
	}
	return nil
}

func (m *mockDHReconcileRunner) GetLastRunResult() *dhlisting.ReconcileResult {
	if m.GetLastRunResultFn != nil {
		return m.GetLastRunResultFn()
	}
	return nil
}

// TestDHReconcileHandler_Trigger exercises the admin trigger endpoint.
// The runner contract is RunOnce + GetLastRunResult, mirroring
// DHReconcileScheduler so it plugs in directly at wiring time.
func TestDHReconcileHandler_Trigger(t *testing.T) {
	tests := []struct {
		name       string
		runner     *mockDHReconcileRunner
		wantStatus int
	}{
		{
			name: "success returns result",
			runner: &mockDHReconcileRunner{
				RunOnceFn: func(context.Context) error { return nil },
				GetLastRunResultFn: func() *dhlisting.ReconcileResult {
					return &dhlisting.ReconcileResult{Scanned: 5, MissingOnDH: 2, Reset: 2}
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "runner error returns 502",
			runner: &mockDHReconcileRunner{
				RunOnceFn: func(context.Context) error { return errors.New("boom") },
			},
			wantStatus: http.StatusBadGateway,
		},
		{
			name: "success with nil last result returns 200 zero body",
			runner: &mockDHReconcileRunner{
				RunOnceFn:          func(context.Context) error { return nil },
				GetLastRunResultFn: func() *dhlisting.ReconcileResult { return nil },
			},
			wantStatus: http.StatusOK,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewDHReconcileHandler(tc.runner, observability.NewNoopLogger())
			req := httptest.NewRequest(http.MethodPost, "/api/admin/dh-reconcile/trigger", nil)
			rec := httptest.NewRecorder()
			h.HandleTrigger(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK && tc.runner.GetLastRunResultFn != nil && tc.runner.GetLastRunResultFn() != nil {
				var got dhlisting.ReconcileResult
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("unmarshal body: %v", err)
				}
				if got.Scanned != 5 || got.Reset != 2 {
					t.Errorf("result: got %+v, want Scanned=5 Reset=2", got)
				}
			}
			if tc.name == "success with nil last result returns 200 zero body" {
				var body map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("unmarshal zero body: %v", err)
				}
				for _, key := range []string{"scanned", "missingOnDH", "reset", "errors", "resetIds"} {
					if _, ok := body[key]; !ok {
						t.Errorf("zero body missing key %q; got %v", key, body)
					}
				}
			}
		})
	}
}

// TestDHReconcileHandler_Trigger_NilRunner verifies that constructing the
// handler without a runner (e.g. DH scheduler disabled) makes the endpoint
// return 503 instead of panicking.
func TestDHReconcileHandler_Trigger_NilRunner(t *testing.T) {
	h := NewDHReconcileHandler(nil, observability.NewNoopLogger())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/dh-reconcile/trigger", nil)
	rec := httptest.NewRecorder()
	h.HandleTrigger(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
