package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func testLogger() observability.Logger {
	return mocks.NewMockLogger()
}

func TestServiceCall_Success(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()
	logger := testLogger()

	result, ok := serviceCall(w, ctx, logger, "test op", func() (string, error) {
		return "hello", nil
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %q", result)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 (no write), got %d", w.Code)
	}
}

func TestServiceCall_Error(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()
	logger := testLogger()

	_, ok := serviceCall(w, ctx, logger, "test op", func() (string, error) {
		return "", errors.New("boom")
	})
	if ok {
		t.Fatal("expected ok=false on error")
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "Internal server error" {
		t.Fatalf("expected 'Internal server error', got %q", resp["error"])
	}
}

func TestServiceCallVoid_Success(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()
	logger := testLogger()

	ok := serviceCallVoid(w, ctx, logger, "test op", func() error {
		return nil
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
}

func TestServiceCallVoid_Error(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()
	logger := testLogger()

	ok := serviceCallVoid(w, ctx, logger, "test op", func() error {
		return errors.New("boom")
	})
	if ok {
		t.Fatal("expected ok=false on error")
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
