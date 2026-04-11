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
	}
}

func TestSocialPublishScheduler_DailyCap(t *testing.T) {
	repo := &mockPublishRepo{
		countTodayFn: func(_ context.Context) (int, error) { return 10, nil },
		fetchEligibleFn: func(_ context.Context) (*social.PostDetail, error) {
			t.Fatal("FetchEligibleDraft should not be called when cap is reached")
			return nil, nil
		},
		updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
	}
	rc := &mockRenderClient{
		healthFn: func(_ context.Context) error { return nil },
	}
	pub := &mockSocialPublisher{
		publishFn: func(_ context.Context, _ string) error {
			t.Fatal("Publish should not be called when cap is reached")
			return nil
		},
	}
	s := newTestPublishScheduler(t, repo, rc, pub, defaultPublishConfig())
	s.tick(context.Background())
}

func TestSocialPublishScheduler_WindowCheck(t *testing.T) {
	cfg := SocialPublishSchedulerConfig{
		StartHour:       14, // 2 PM
		EndHour:         16, // 4 PM
		IntervalMinutes: 60,
		MaxDaily:        10,
		fixedHour:       8, // simulate 8 AM — outside window
	}
	fetchCalled := false
	repo := &mockPublishRepo{
		countTodayFn: func(_ context.Context) (int, error) { return 0, nil },
		fetchEligibleFn: func(_ context.Context) (*social.PostDetail, error) {
			fetchCalled = true
			return nil, nil
		},
		updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
	}
	rc := &mockRenderClient{healthFn: func(_ context.Context) error { return nil }}
	pub := &mockSocialPublisher{publishFn: func(_ context.Context, _ string) error { return nil }}
	s := newTestPublishScheduler(t, repo, rc, pub, cfg)
	s.tick(context.Background())
	if fetchCalled {
		t.Error("should not fetch draft when outside window")
	}
}

func TestSocialPublishScheduler_RenderClientUnhealthy(t *testing.T) {
	fetchCalled := false
	repo := &mockPublishRepo{
		countTodayFn: func(_ context.Context) (int, error) { return 0, nil },
		fetchEligibleFn: func(_ context.Context) (*social.PostDetail, error) {
			fetchCalled = true
			return nil, nil
		},
		updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
	}
	rc := &mockRenderClient{
		healthFn: func(_ context.Context) error { return errors.New("connection refused") },
	}
	pub := &mockSocialPublisher{publishFn: func(_ context.Context, _ string) error { return nil }}
	s := newTestPublishScheduler(t, repo, rc, pub, defaultPublishConfig())
	s.tick(context.Background())
	if fetchCalled {
		t.Error("should not fetch draft when sidecar is unhealthy")
	}
}

func TestSocialPublishScheduler_NoDraftsAvailable(t *testing.T) {
	publishCalled := false
	repo := &mockPublishRepo{
		countTodayFn:      func(_ context.Context) (int, error) { return 0, nil },
		fetchEligibleFn:   func(_ context.Context) (*social.PostDetail, error) { return nil, nil },
		updateSlideURLsFn: func(_ context.Context, _ string, _ []string) error { return nil },
	}
	rc := &mockRenderClient{
		healthFn: func(_ context.Context) error { return nil },
		renderFn: func(_ context.Context, _ string, _ social.PostDetail) ([][]byte, error) {
			return nil, nil
		},
	}
	pub := &mockSocialPublisher{
		publishFn: func(_ context.Context, _ string) error {
			publishCalled = true
			return nil
		},
	}
	s := newTestPublishScheduler(t, repo, rc, pub, defaultPublishConfig())
	s.tick(context.Background())
	if publishCalled {
		t.Error("should not publish when no draft is available")
	}
}

func TestSocialPublishScheduler_HappyPath(t *testing.T) {
	fakeJPEG := []byte{0xFF, 0xD8, 0xFF}
	post := &social.PostDetail{
		SocialPost: social.SocialPost{
			ID:        "post-abc",
			CardCount: 1,
			Caption:   "some caption",
		},
		Cards: []social.PostCardDetail{{PurchaseID: "p1"}},
	}

	slideURLsSet := false
	publishCalled := false

	repo := &mockPublishRepo{
		countTodayFn:    func(_ context.Context) (int, error) { return 0, nil },
		fetchEligibleFn: func(_ context.Context) (*social.PostDetail, error) { return post, nil },
		updateSlideURLsFn: func(_ context.Context, _ string, urls []string) error {
			if len(urls) != 2 {
				t.Errorf("expected 2 slide URLs, got %d", len(urls))
			}
			slideURLsSet = true
			return nil
		},
	}
	rc := &mockRenderClient{
		healthFn: func(_ context.Context) error { return nil },
		renderFn: func(_ context.Context, _ string, _ social.PostDetail) ([][]byte, error) {
			return [][]byte{fakeJPEG, fakeJPEG}, nil
		},
	}
	pub := &mockSocialPublisher{
		publishFn: func(_ context.Context, id string) error {
			if id != post.ID {
				t.Errorf("publish called with wrong ID: %s", id)
			}
			publishCalled = true
			return nil
		},
	}
	s := newTestPublishScheduler(t, repo, rc, pub, defaultPublishConfig())
	s.tick(context.Background())
	if !slideURLsSet {
		t.Error("slide URLs should have been set")
	}
	if !publishCalled {
		t.Error("Publish should have been called")
	}
}
