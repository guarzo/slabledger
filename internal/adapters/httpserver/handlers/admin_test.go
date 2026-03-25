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

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newAdminHandlers(svc *mockAuthService) *AdminHandlers {
	return NewAdminHandlers(svc, mocks.NewMockLogger())
}

func withUser(r *http.Request) *http.Request {
	user := &auth.User{ID: 42, Username: "admin", Email: "admin@test.com"}
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, user)
	return r.WithContext(ctx)
}

// --- HandleListAllowedEmails ---

func TestHandleListAllowedEmails_Success(t *testing.T) {
	svc := &mockAuthService{
		listAllowedEmailsFunc: func(_ context.Context) ([]auth.AllowedEmail, error) {
			return []auth.AllowedEmail{
				{Email: "a@b.com", CreatedAt: time.Now()},
			}, nil
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/allowlist", nil)
	h.HandleListAllowedEmails(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []auth.AllowedEmail
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 email, got %d", len(result))
	}
}

func TestHandleListAllowedEmails_NilList(t *testing.T) {
	svc := &mockAuthService{
		listAllowedEmailsFunc: func(_ context.Context) ([]auth.AllowedEmail, error) {
			return nil, nil
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/allowlist", nil)
	h.HandleListAllowedEmails(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Should be an empty JSON array, not null
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("expected [], got %s", body)
	}
}

func TestHandleListAllowedEmails_ServiceError(t *testing.T) {
	svc := &mockAuthService{
		listAllowedEmailsFunc: func(_ context.Context) ([]auth.AllowedEmail, error) {
			return nil, errors.New("db error")
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/allowlist", nil)
	h.HandleListAllowedEmails(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- HandleAddAllowedEmail ---

func TestHandleAddAllowedEmail_Success(t *testing.T) {
	var capturedEmail string
	svc := &mockAuthService{
		addAllowedEmailFunc: func(_ context.Context, email string, _ int64, _ string) error {
			capturedEmail = email
			return nil
		},
	}
	h := newAdminHandlers(svc)

	body := `{"email":"test@example.com","notes":"test note"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/allowlist", strings.NewReader(body))
	req = withUser(req)
	h.HandleAddAllowedEmail(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if capturedEmail != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", capturedEmail)
	}
}

func TestHandleAddAllowedEmail_NoUser(t *testing.T) {
	h := newAdminHandlers(&mockAuthService{})

	body := `{"email":"test@example.com"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/allowlist", strings.NewReader(body))
	// No user in context
	h.HandleAddAllowedEmail(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleAddAllowedEmail_BadJSON(t *testing.T) {
	h := newAdminHandlers(&mockAuthService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/allowlist", strings.NewReader("{invalid"))
	req = withUser(req)
	h.HandleAddAllowedEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAddAllowedEmail_EmptyEmail(t *testing.T) {
	h := newAdminHandlers(&mockAuthService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/allowlist", strings.NewReader(`{"email":""}`))
	req = withUser(req)
	h.HandleAddAllowedEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAddAllowedEmail_NoAtSign(t *testing.T) {
	h := newAdminHandlers(&mockAuthService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/allowlist", strings.NewReader(`{"email":"invalid"}`))
	req = withUser(req)
	h.HandleAddAllowedEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAddAllowedEmail_NormalizesEmail(t *testing.T) {
	var capturedEmail string
	svc := &mockAuthService{
		addAllowedEmailFunc: func(_ context.Context, email string, _ int64, _ string) error {
			capturedEmail = email
			return nil
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/allowlist", strings.NewReader(`{"email":"FOO@BAR.COM"}`))
	req = withUser(req)
	h.HandleAddAllowedEmail(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if capturedEmail != "foo@bar.com" {
		t.Errorf("expected normalized email foo@bar.com, got %s", capturedEmail)
	}
}

func TestHandleAddAllowedEmail_ServiceError(t *testing.T) {
	svc := &mockAuthService{
		addAllowedEmailFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return errors.New("db error")
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/allowlist", strings.NewReader(`{"email":"ok@test.com"}`))
	req = withUser(req)
	h.HandleAddAllowedEmail(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- HandleRemoveAllowedEmail ---

func TestHandleRemoveAllowedEmail_Success(t *testing.T) {
	var capturedEmail string
	svc := &mockAuthService{
		removeAllowedEmailFunc: func(_ context.Context, email string) error {
			capturedEmail = email
			return nil
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/allowlist/test@test.com", nil)
	h.HandleRemoveAllowedEmail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedEmail != "test@test.com" {
		t.Errorf("expected test@test.com, got %s", capturedEmail)
	}
}

func TestHandleRemoveAllowedEmail_EmptyEmail(t *testing.T) {
	h := newAdminHandlers(&mockAuthService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/allowlist/", nil)
	h.HandleRemoveAllowedEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleRemoveAllowedEmail_ServiceError(t *testing.T) {
	svc := &mockAuthService{
		removeAllowedEmailFunc: func(_ context.Context, _ string) error {
			return errors.New("db error")
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/allowlist/test@test.com", nil)
	h.HandleRemoveAllowedEmail(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- HandleListUsers ---

func TestHandleListUsers_Success(t *testing.T) {
	now := time.Now()
	svc := &mockAuthService{
		listUsersFunc: func(_ context.Context) ([]auth.User, error) {
			return []auth.User{
				{ID: 1, Username: "alice", Email: "alice@test.com", IsAdmin: true, LastLoginAt: &now},
				{ID: 2, Username: "bob", Email: "bob@test.com"},
			}, nil
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	h.HandleListUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 users, got %d", len(result))
	}
	// Verify response shape
	if result[0]["username"] != "alice" {
		t.Errorf("expected username alice, got %v", result[0]["username"])
	}
	if result[0]["is_admin"] != true {
		t.Errorf("expected is_admin true, got %v", result[0]["is_admin"])
	}
}

func TestHandleListUsers_NilList(t *testing.T) {
	svc := &mockAuthService{
		listUsersFunc: func(_ context.Context) ([]auth.User, error) {
			return nil, nil
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	h.HandleListUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Should still be a valid JSON array (not null)
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("expected [], got %s", body)
	}
}

func TestHandleListUsers_ServiceError(t *testing.T) {
	svc := &mockAuthService{
		listUsersFunc: func(_ context.Context) ([]auth.User, error) {
			return nil, errors.New("db error")
		},
	}
	h := newAdminHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	h.HandleListUsers(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- HandleBackup ---

func TestHandleBackup_MethodNotAllowed(t *testing.T) {
	handler := HandleBackup("/nonexistent.db", mocks.NewMockLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/backup", nil)
	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
