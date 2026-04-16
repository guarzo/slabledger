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
