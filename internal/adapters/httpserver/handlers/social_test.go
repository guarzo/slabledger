package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestSocialHandler(svc *mocks.MockSocialService) *SocialHandler {
	return NewSocialHandler(svc, nil, mocks.NewMockLogger(), "", "")
}

func authenticatedContext() context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, &auth.User{
		ID:       1,
		Username: "testuser",
		Email:    "test@example.com",
	})
}

func samplePost() social.SocialPost {
	return social.SocialPost{
		ID:        "post-abc",
		PostType:  social.PostTypeNewArrivals,
		Status:    social.PostStatusDraft,
		Caption:   "Check out these new arrivals!",
		Hashtags:  "#pokemon #cards",
		CardCount: 3,
		CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestHandleListPosts_Success(t *testing.T) {
	svc := &mocks.MockSocialService{
		ListPostsFn: func(_ context.Context, status *social.PostStatus, limit, offset int) ([]social.SocialPost, error) {
			if status != nil {
				t.Errorf("expected nil status, got %v", *status)
			}
			return []social.SocialPost{samplePost()}, nil
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts", nil)
	req = req.WithContext(authenticatedContext())
	rec := httptest.NewRecorder()
	h.HandleListPosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []social.SocialPost
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 post, got %d", len(result))
	}
}

func TestHandleListPosts_Filter(t *testing.T) {
	svc := &mocks.MockSocialService{
		ListPostsFn: func(_ context.Context, status *social.PostStatus, _, _ int) ([]social.SocialPost, error) {
			if status == nil || *status != social.PostStatusDraft {
				t.Errorf("expected status=draft, got %v", status)
			}
			return []social.SocialPost{samplePost()}, nil
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts?status=draft", nil)
	req = req.WithContext(authenticatedContext())
	rec := httptest.NewRecorder()
	h.HandleListPosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleListPosts_InvalidFilter(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts?status=bogus", nil)
	req = req.WithContext(authenticatedContext())
	rec := httptest.NewRecorder()
	h.HandleListPosts(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleListPosts_Unauthenticated(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts", nil)
	rec := httptest.NewRecorder()
	h.HandleListPosts(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleListPosts_Error(t *testing.T) {
	svc := &mocks.MockSocialService{
		ListPostsFn: func(_ context.Context, _ *social.PostStatus, _, _ int) ([]social.SocialPost, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts", nil)
	req = req.WithContext(authenticatedContext())
	rec := httptest.NewRecorder()
	h.HandleListPosts(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleListPosts_ValidStatuses(t *testing.T) {
	validStatuses := []social.PostStatus{
		social.PostStatusDraft,
		social.PostStatusPublishing,
		social.PostStatusPublished,
		social.PostStatusFailed,
	}

	for _, s := range validStatuses {
		t.Run(string(s), func(t *testing.T) {
			var captured social.PostStatus
			svc := &mocks.MockSocialService{
				ListPostsFn: func(_ context.Context, status *social.PostStatus, _, _ int) ([]social.SocialPost, error) {
					if status != nil {
						captured = *status
					}
					return []social.SocialPost{}, nil
				},
			}
			h := newTestSocialHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/social/posts?status="+string(s), nil)
			req = req.WithContext(authenticatedContext())
			rec := httptest.NewRecorder()
			h.HandleListPosts(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if captured != s {
				t.Errorf("expected status=%s, got %s", s, captured)
			}
		})
	}
}

func TestHandleGetPost_Success(t *testing.T) {
	svc := &mocks.MockSocialService{
		GetPostFn: func(_ context.Context, id string) (*social.PostDetail, error) {
			return &social.PostDetail{
				SocialPost: samplePost(),
				Cards:      []social.PostCardDetail{},
			}, nil
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts/post-abc", nil)
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleGetPost_NotFound(t *testing.T) {
	svc := &mocks.MockSocialService{
		GetPostFn: func(_ context.Context, _ string) (*social.PostDetail, error) {
			return nil, nil
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts/missing", nil)
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleGetPost_Error(t *testing.T) {
	svc := &mocks.MockSocialService{
		GetPostFn: func(_ context.Context, _ string) (*social.PostDetail, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts/post-abc", nil)
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleGetPost_Unauthenticated(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts/post-abc", nil)
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleGetPost_MissingID(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts/", nil)
	req = req.WithContext(authenticatedContext())
	rec := httptest.NewRecorder()
	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateCaption_Success(t *testing.T) {
	var capturedID, capturedCaption, capturedHashtags string
	svc := &mocks.MockSocialService{
		UpdateCaptionFn: func(_ context.Context, id string, caption, hashtags string) error {
			capturedID = id
			capturedCaption = caption
			capturedHashtags = hashtags
			return nil
		},
	}
	h := newTestSocialHandler(svc)

	body := `{"caption":"Updated caption","hashtags":"#updated"}`
	req := httptest.NewRequest(http.MethodPut, "/api/social/posts/post-abc/caption", bytes.NewBufferString(body))
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleUpdateCaption(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if capturedID != "post-abc" {
		t.Errorf("expected id=post-abc, got %q", capturedID)
	}
	if capturedCaption != "Updated caption" {
		t.Errorf("expected caption=Updated caption, got %q", capturedCaption)
	}
	if capturedHashtags != "#updated" {
		t.Errorf("expected hashtags=#updated, got %q", capturedHashtags)
	}
}

func TestHandleUpdateCaption_Error(t *testing.T) {
	svc := &mocks.MockSocialService{
		UpdateCaptionFn: func(_ context.Context, _ string, _, _ string) error {
			return fmt.Errorf("update failed")
		},
	}
	h := newTestSocialHandler(svc)

	body := `{"caption":"new","hashtags":"#test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/social/posts/post-abc/caption", bytes.NewBufferString(body))
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleUpdateCaption(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleUpdateCaption_InvalidBody(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodPut, "/api/social/posts/post-abc/caption", bytes.NewBufferString("{bad"))
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleUpdateCaption(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateCaption_Unauthenticated(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	body := `{"caption":"new","hashtags":"#test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/social/posts/post-abc/caption", bytes.NewBufferString(body))
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleUpdateCaption(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleDelete_Social_Success(t *testing.T) {
	var capturedID string
	svc := &mocks.MockSocialService{
		DeleteFn: func(_ context.Context, id string) error {
			capturedID = id
			return nil
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/social/posts/post-abc", nil)
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if capturedID != "post-abc" {
		t.Errorf("expected id=post-abc, got %q", capturedID)
	}
}

func TestHandleDelete_Social_Error(t *testing.T) {
	svc := &mocks.MockSocialService{
		DeleteFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("delete failed")
		},
	}
	h := newTestSocialHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/social/posts/post-abc", nil)
	req = req.WithContext(authenticatedContext())
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleDelete_Social_Unauthenticated(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/social/posts/post-abc", nil)
	req.SetPathValue("id", "post-abc")
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleDelete_Social_MissingID(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/social/posts/", nil)
	req = req.WithContext(authenticatedContext())
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleBackfillImages_NotConfigured(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodPost, "/api/social/backfill-images", nil)
	req = req.WithContext(authenticatedContext())
	rec := httptest.NewRecorder()
	h.HandleBackfillImages(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleBackfillImages_Unauthenticated(t *testing.T) {
	h := newTestSocialHandler(&mocks.MockSocialService{})

	req := httptest.NewRequest(http.MethodPost, "/api/social/backfill-images", nil)
	rec := httptest.NewRecorder()
	h.HandleBackfillImages(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
