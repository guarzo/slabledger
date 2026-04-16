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

func TestHandleReconcile_NotConfigured(t *testing.T) {
	h := newReconcileTestHandler(nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dh/reconcile", nil)
	h.HandleReconcile(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want 503", rec.Code)
	}
}

func TestHandleReconcile_Success(t *testing.T) {
	want := dhlisting.ReconcileResult{
		Scanned:     50,
		MissingOnDH: 3,
		Reset:       2,
		Errors:      []string{"p9: write conflict"},
		ResetIDs:    []string{"p1", "p2"},
	}
	svc := &mockReconciler{
		ReconcileFn: func(_ context.Context) (dhlisting.ReconcileResult, error) {
			return want, nil
		},
	}
	h := newReconcileTestHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dh/reconcile", nil)
	h.HandleReconcile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got reconcileResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Scanned != want.Scanned ||
		got.MissingOnDH != want.MissingOnDH ||
		got.Reset != want.Reset ||
		len(got.Errors) != len(want.Errors) ||
		len(got.ResetIDs) != len(want.ResetIDs) {
		t.Errorf("response shape mismatch: got %+v, want %+v", got, want)
	}
}

func TestHandleReconcile_ServiceError(t *testing.T) {
	svc := &mockReconciler{
		ReconcileFn: func(_ context.Context) (dhlisting.ReconcileResult, error) {
			return dhlisting.ReconcileResult{}, errors.New("DH 500")
		},
	}
	h := newReconcileTestHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dh/reconcile", nil)
	h.HandleReconcile(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want 502 (body=%s)", rec.Code, rec.Body.String())
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
