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

func TestHandleListPosts(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		authenticated  bool
		setupMock      func(*mocks.MockSocialService)
		wantStatus     int
		wantCount      int
		wantDecodeErr  bool
	}{
		{
			name:          "success",
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.ListPostsFn = func(_ context.Context, status *social.PostStatus, _, _ int) ([]social.SocialPost, error) {
					if status != nil {
						t.Errorf("expected nil status, got %v", *status)
					}
					return []social.SocialPost{samplePost()}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:          "filter by status",
			query:         "?status=draft",
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.ListPostsFn = func(_ context.Context, status *social.PostStatus, _, _ int) ([]social.SocialPost, error) {
					if status == nil || *status != social.PostStatusDraft {
						t.Errorf("expected status=draft, got %v", status)
					}
					return []social.SocialPost{samplePost()}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:          "invalid filter",
			query:         "?status=bogus",
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
			wantDecodeErr: true,
		},
		{
			name:       "unauthenticated",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "service error",
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.ListPostsFn = func(_ context.Context, _ *social.PostStatus, _, _ int) ([]social.SocialPost, error) {
					return nil, fmt.Errorf("db error")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockSocialService{}
			if tt.setupMock != nil {
				tt.setupMock(svc)
			}
			h := newTestSocialHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/social/posts"+tt.query, nil)
			if tt.authenticated {
				req = req.WithContext(authenticatedContext())
			}
			rec := httptest.NewRecorder()
			h.HandleListPosts(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantDecodeErr {
				decodeErrorResponse(t, rec)
			}
			if tt.wantCount > 0 {
				var result []social.SocialPost
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(result) != tt.wantCount {
					t.Errorf("expected %d post(s), got %d", tt.wantCount, len(result))
				}
			}
		})
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

func TestHandleGetPost(t *testing.T) {
	tests := []struct {
		name          string
		pathID        string
		authenticated bool
		setupMock     func(*mocks.MockSocialService)
		wantStatus    int
	}{
		{
			name:          "success",
			pathID:        "post-abc",
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.GetPostFn = func(_ context.Context, id string) (*social.PostDetail, error) {
					return &social.PostDetail{
						SocialPost: samplePost(),
						Cards:      []social.PostCardDetail{},
					}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:          "not found",
			pathID:        "missing",
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.GetPostFn = func(_ context.Context, _ string) (*social.PostDetail, error) {
					return nil, nil
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:          "service error",
			pathID:        "post-abc",
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.GetPostFn = func(_ context.Context, _ string) (*social.PostDetail, error) {
					return nil, fmt.Errorf("db error")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unauthenticated",
			pathID:     "post-abc",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "missing ID",
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockSocialService{}
			if tt.setupMock != nil {
				tt.setupMock(svc)
			}
			h := newTestSocialHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/social/posts/"+tt.pathID, nil)
			if tt.authenticated {
				req = req.WithContext(authenticatedContext())
			}
			if tt.pathID != "" {
				req.SetPathValue("id", tt.pathID)
			}
			rec := httptest.NewRecorder()
			h.HandleGetPost(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandleUpdateCaption(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		authenticated bool
		setupMock     func(*mocks.MockSocialService)
		wantStatus    int
		validate      func(t *testing.T)
	}{
		{
			name:          "success",
			body:          `{"caption":"Updated caption","hashtags":"#updated"}`,
			authenticated: true,
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "service error",
			body:          `{"caption":"new","hashtags":"#test"}`,
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.UpdateCaptionFn = func(_ context.Context, _ string, _, _ string) error {
					return fmt.Errorf("update failed")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:          "invalid body",
			body:          "{bad",
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:       "unauthenticated",
			body:       `{"caption":"new","hashtags":"#test"}`,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID, capturedCaption, capturedHashtags string
			svc := &mocks.MockSocialService{}
			if tt.setupMock != nil {
				tt.setupMock(svc)
			} else {
				svc.UpdateCaptionFn = func(_ context.Context, id string, caption, hashtags string) error {
					capturedID = id
					capturedCaption = caption
					capturedHashtags = hashtags
					return nil
				}
			}
			h := newTestSocialHandler(svc)

			req := httptest.NewRequest(http.MethodPut, "/api/social/posts/post-abc/caption", bytes.NewBufferString(tt.body))
			if tt.authenticated {
				req = req.WithContext(authenticatedContext())
			}
			req.SetPathValue("id", "post-abc")
			rec := httptest.NewRecorder()
			h.HandleUpdateCaption(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantStatus == http.StatusNoContent {
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
		})
	}
}

func TestHandleDelete_Social(t *testing.T) {
	tests := []struct {
		name          string
		pathID        string
		authenticated bool
		setupMock     func(*mocks.MockSocialService)
		wantStatus    int
	}{
		{
			name:          "success",
			pathID:        "post-abc",
			authenticated: true,
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "service error",
			pathID:        "post-abc",
			authenticated: true,
			setupMock: func(m *mocks.MockSocialService) {
				m.DeleteFn = func(_ context.Context, _ string) error {
					return fmt.Errorf("delete failed")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unauthenticated",
			pathID:     "post-abc",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "missing ID",
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID string
			svc := &mocks.MockSocialService{}
			if tt.setupMock != nil {
				tt.setupMock(svc)
			} else {
				svc.DeleteFn = func(_ context.Context, id string) error {
					capturedID = id
					return nil
				}
			}
			h := newTestSocialHandler(svc)

			req := httptest.NewRequest(http.MethodDelete, "/api/social/posts/"+tt.pathID, nil)
			if tt.authenticated {
				req = req.WithContext(authenticatedContext())
			}
			if tt.pathID != "" {
				req.SetPathValue("id", tt.pathID)
			}
			rec := httptest.NewRecorder()
			h.HandleDelete(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantStatus == http.StatusNoContent && capturedID != "post-abc" {
				t.Errorf("expected id=post-abc, got %q", capturedID)
			}
		})
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
