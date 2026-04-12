package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/social"
)

// --- test doubles ---

type mockPublishRepo struct {
	countTodayFn      func(ctx context.Context) (int, error)
	fetchEligibleFn   func(ctx context.Context) (*social.PostDetail, error)
	updateSlideURLsFn func(ctx context.Context, id string, urls []string) error
}

func (m *mockPublishRepo) CountPublishedToday(ctx context.Context) (int, error) {
	return m.countTodayFn(ctx)
}
func (m *mockPublishRepo) FetchEligibleDraft(ctx context.Context) (*social.PostDetail, error) {
	return m.fetchEligibleFn(ctx)
}
func (m *mockPublishRepo) UpdateSlideURLs(ctx context.Context, id string, urls []string) error {
	return m.updateSlideURLsFn(ctx, id, urls)
}

type mockRenderClient struct {
	healthFn func(ctx context.Context) error
	renderFn func(ctx context.Context, postID string, detail social.PostDetail) ([][]byte, error)
}

func (m *mockRenderClient) Health(ctx context.Context) error {
	return m.healthFn(ctx)
}
func (m *mockRenderClient) Render(ctx context.Context, postID string, detail social.PostDetail) ([][]byte, error) {
	return m.renderFn(ctx, postID, detail)
}

type mockSocialPublisher struct {
	publishFn func(ctx context.Context, id string) error
}

func (m *mockSocialPublisher) Publish(ctx context.Context, id string) error {
	return m.publishFn(ctx, id)
}

func newTestPublishScheduler(t testing.TB, repo *mockPublishRepo, rc *mockRenderClient, pub *mockSocialPublisher, cfg SocialPublishSchedulerConfig) *SocialPublishScheduler {
	return &SocialPublishScheduler{
		StopHandle:   NewStopHandle(),
		repo:         repo,
		renderClient: rc,
		publisher:    pub,
		mediaDir:     t.TempDir(),
		cfg:          cfg,
		logger:       nopLogger{},
	}
}

func defaultPublishConfig() SocialPublishSchedulerConfig {
	return SocialPublishSchedulerConfig{
		StartHour:       0,
		EndHour:         24,
		IntervalMinutes: 60,
		MaxDaily:        10,
		fixedHour:       -1, // use real clock
	}
}

func TestSocialPublishScheduler(t *testing.T) {
	fakeJPEG := []byte{0xFF, 0xD8, 0xFF}
	post := &social.PostDetail{
		SocialPost: social.SocialPost{
			ID:        "post-abc",
			CardCount: 1,
			Caption:   "some caption",
		},
		Cards: []social.PostCardDetail{{PurchaseID: "p1"}},
	}

	tests := []struct {
		name               string
		cfg                SocialPublishSchedulerConfig
		countTodayFn       func(ctx context.Context) (int, error)
		fetchEligibleFn    func(ctx context.Context) (*social.PostDetail, error)
		updateSlideURLsFn  func(ctx context.Context, id string, urls []string) error
		healthFn           func(ctx context.Context) error
		renderFn           func(ctx context.Context, postID string, detail social.PostDetail) ([][]byte, error)
		publishFn          func(ctx context.Context, id string) error
		shouldFetch        bool
		shouldPublish      bool
		expectSlideURLsSet bool
	}{
		{
			name:         "DailyCap",
			cfg:          defaultPublishConfig(),
			countTodayFn: func(_ context.Context) (int, error) { return 10, nil },
			fetchEligibleFn: func(_ context.Context) (*social.PostDetail, error) {
				t.Fatal("FetchEligibleDraft should not be called when cap is reached")
				return nil, nil
			},
			updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
			healthFn:          func(_ context.Context) error { return nil },
			publishFn: func(_ context.Context, _ string) error {
				t.Fatal("Publish should not be called when cap is reached")
				return nil
			},
			shouldFetch:   false,
			shouldPublish: false,
		},
		{
			name: "WindowCheck",
			cfg: SocialPublishSchedulerConfig{
				StartHour:       14,
				EndHour:         16,
				IntervalMinutes: 60,
				MaxDaily:        10,
				fixedHour:       8, // 8 AM — outside window
			},
			countTodayFn:      func(_ context.Context) (int, error) { return 0, nil },
			fetchEligibleFn:   func(_ context.Context) (*social.PostDetail, error) { return nil, nil },
			updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
			healthFn:          func(_ context.Context) error { return nil },
			publishFn:         func(_ context.Context, _ string) error { return nil },
			shouldFetch:       false,
			shouldPublish:     false,
		},
		{
			name:              "RenderClientUnhealthy",
			cfg:               defaultPublishConfig(),
			countTodayFn:      func(_ context.Context) (int, error) { return 0, nil },
			fetchEligibleFn:   func(_ context.Context) (*social.PostDetail, error) { return nil, nil },
			updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
			healthFn:          func(_ context.Context) error { return errors.New("connection refused") },
			publishFn:         func(_ context.Context, _ string) error { return nil },
			shouldFetch:       false,
			shouldPublish:     false,
		},
		{
			name:              "NoDraftsAvailable",
			cfg:               defaultPublishConfig(),
			countTodayFn:      func(_ context.Context) (int, error) { return 0, nil },
			fetchEligibleFn:   func(_ context.Context) (*social.PostDetail, error) { return nil, nil },
			updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
			healthFn:          func(_ context.Context) error { return nil },
			renderFn: func(_ context.Context, _ string, _ social.PostDetail) ([][]byte, error) {
				return nil, nil
			},
			publishFn:     func(_ context.Context, _ string) error { return nil },
			shouldFetch:   true,
			shouldPublish: false,
		},
		{
			name:            "HappyPath",
			cfg:             defaultPublishConfig(),
			countTodayFn:    func(_ context.Context) (int, error) { return 0, nil },
			fetchEligibleFn: func(_ context.Context) (*social.PostDetail, error) { return post, nil },
			updateSlideURLsFn: func(_ context.Context, _ string, urls []string) error {
				if len(urls) != 2 {
					t.Errorf("expected 2 slide URLs, got %d", len(urls))
				}
				return nil
			},
			healthFn: func(_ context.Context) error { return nil },
			renderFn: func(_ context.Context, _ string, _ social.PostDetail) ([][]byte, error) {
				return [][]byte{fakeJPEG, fakeJPEG}, nil
			},
			publishFn: func(_ context.Context, id string) error {
				if id != post.ID {
					t.Errorf("publish called with wrong ID: %s", id)
				}
				return nil
			},
			shouldFetch:        true,
			shouldPublish:      true,
			expectSlideURLsSet: true,
		},
		{
			name:         "PreRenderedPost",
			cfg:          defaultPublishConfig(),
			countTodayFn: func(_ context.Context) (int, error) { return 0, nil },
			fetchEligibleFn: func(_ context.Context) (*social.PostDetail, error) {
				preRenderedPost := &social.PostDetail{
					SocialPost: social.SocialPost{
						ID:        "post-prerendered",
						CardCount: 1,
						Caption:   "pre-rendered caption",
						SlideURLs: []string{"/api/media/social/post-prerendered/slide-0.jpg"},
					},
					Cards: []social.PostCardDetail{{PurchaseID: "p1"}},
				}
				return preRenderedPost, nil
			},
			updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error {
				t.Error("UpdateSlideURLs should not be called for pre-rendered posts")
				return nil
			},
			healthFn: func(_ context.Context) error {
				t.Error("Health should not be called for pre-rendered posts")
				return nil
			},
			renderFn: func(_ context.Context, _ string, _ social.PostDetail) ([][]byte, error) {
				t.Error("Render should not be called for pre-rendered posts")
				return nil, nil
			},
			publishFn: func(_ context.Context, id string) error {
				if id != "post-prerendered" {
					t.Errorf("publish called with wrong ID: %s", id)
				}
				return nil
			},
			shouldFetch:        true,
			shouldPublish:      true,
			expectSlideURLsSet: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetchCalled := false
			publishCalled := false
			slideURLsSet := false

			fetchWrapper := func(ctx context.Context) (*social.PostDetail, error) {
				fetchCalled = true
				return tc.fetchEligibleFn(ctx)
			}
			publishWrapper := func(ctx context.Context, id string) error {
				publishCalled = true
				return tc.publishFn(ctx, id)
			}
			updateWrapper := func(ctx context.Context, id string, urls []string) error {
				slideURLsSet = true
				return tc.updateSlideURLsFn(ctx, id, urls)
			}

			repo := &mockPublishRepo{
				countTodayFn:      tc.countTodayFn,
				fetchEligibleFn:   fetchWrapper,
				updateSlideURLsFn: updateWrapper,
			}
			rc := &mockRenderClient{
				healthFn: tc.healthFn,
				renderFn: tc.renderFn,
			}
			pub := &mockSocialPublisher{publishFn: publishWrapper}

			s := newTestPublishScheduler(t, repo, rc, pub, tc.cfg)
			s.tick(context.Background())

			if tc.shouldFetch && !fetchCalled {
				t.Error("expected FetchEligibleDraft to be called")
			}
			if !tc.shouldFetch && fetchCalled {
				t.Error("expected FetchEligibleDraft NOT to be called")
			}
			if tc.shouldPublish && !publishCalled {
				t.Error("expected Publish to be called")
			}
			if !tc.shouldPublish && publishCalled {
				t.Error("expected Publish NOT to be called")
			}
			if tc.expectSlideURLsSet && !slideURLsSet {
				t.Error("expected slide URLs to be set")
			}
		})
	}
}
