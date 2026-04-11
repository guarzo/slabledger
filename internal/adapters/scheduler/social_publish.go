package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/renderservice"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// SocialPublishRepo defines the narrow repository interface needed by the publish scheduler.
type SocialPublishRepo interface {
	// CountPublishedToday returns the number of posts published on the current calendar day.
	CountPublishedToday(ctx context.Context) (int, error)
	// FetchEligibleDraft returns the oldest eligible draft (non-placeholder caption,
	// created within 7 days, status=draft). Returns nil, nil if none available.
	FetchEligibleDraft(ctx context.Context) (*social.PostDetail, error)
	// UpdateSlideURLs stores rendered slide image URLs for a post.
	UpdateSlideURLs(ctx context.Context, id string, urls []string) error
}

// SocialPublisher is the subset of social.Service needed by the publish scheduler.
type SocialPublisher interface {
	Publish(ctx context.Context, id string) error
}

// SocialPublishSchedulerConfig holds configuration for the publish scheduler.
type SocialPublishSchedulerConfig struct {
	StartHour        int    // Earliest publish hour (0–23, server local time)
	EndHour          int    // Latest publish hour exclusive (0–24)
	IntervalMinutes  int    // Tick interval in minutes
	MaxDaily         int    // Max posts published per calendar day
	RenderServiceURL string // Base URL of the render sidecar
	fixedHour        int    // Test-only: if >= 0, override current hour. -1 = use real clock.
}

var _ Scheduler = (*SocialPublishScheduler)(nil)

// SocialPublishScheduler automatically publishes drafted social posts.
type SocialPublishScheduler struct {
	StopHandle
	repo         SocialPublishRepo
	renderClient renderservice.Client
	publisher    SocialPublisher
	mediaDir     string
	cfg          SocialPublishSchedulerConfig
	logger       observability.Logger
}

// NewSocialPublishScheduler creates a new SocialPublishScheduler.
func NewSocialPublishScheduler(
	repo SocialPublishRepo,
	renderClient renderservice.Client,
	publisher SocialPublisher,
	mediaDir string,
	cfg SocialPublishSchedulerConfig,
	logger observability.Logger,
) *SocialPublishScheduler {
	return &SocialPublishScheduler{
		StopHandle:   NewStopHandle(),
		repo:         repo,
		renderClient: renderClient,
		publisher:    publisher,
		mediaDir:     mediaDir,
		cfg:          cfg,
		logger:       logger.With(context.Background(), observability.String("component", "social-publish")),
	}
}

// Start begins the background scheduler loop.
func (s *SocialPublishScheduler) Start(ctx context.Context) {
	interval := time.Duration(s.cfg.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 60 * time.Minute
	}
	RunLoop(ctx, LoopConfig{
		Name:         "social-publish",
		Interval:     interval,
		InitialDelay: 2 * time.Minute, // brief startup delay
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.tick)
}

func (s *SocialPublishScheduler) tick(ctx context.Context) {
	currentHour := s.cfg.fixedHour
	if currentHour < 0 {
		currentHour = time.Now().Hour()
	}

	// Check time window
	if currentHour < s.cfg.StartHour || currentHour >= s.cfg.EndHour {
		s.logger.Info(ctx, "outside publish window, skipping",
			observability.Int("current_hour", currentHour),
			observability.Int("start_hour", s.cfg.StartHour),
			observability.Int("end_hour", s.cfg.EndHour),
		)
		return
	}

	// Check daily cap
	count, err := s.repo.CountPublishedToday(ctx)
	if err != nil {
		s.logger.Error(ctx, "failed to count today's published posts", observability.Err(err))
		return
	}
	if count >= s.cfg.MaxDaily {
		s.logger.Info(ctx, "daily publish cap reached",
			observability.Int("published_today", count),
			observability.Int("max_daily", s.cfg.MaxDaily),
		)
		return
	}

	// Verify render sidecar is healthy
	if err := s.renderClient.Health(ctx); err != nil {
		s.logger.Error(ctx, "render sidecar unhealthy, skipping tick", observability.Err(err))
		return
	}

	// Fetch one eligible draft
	post, err := s.repo.FetchEligibleDraft(ctx)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch eligible draft", observability.Err(err))
		return
	}
	if post == nil {
		s.logger.Info(ctx, "no eligible drafts found")
		return
	}

	// Render slides
	blobs, err := s.renderClient.Render(ctx, post.ID, *post)
	if err != nil {
		s.logger.Error(ctx, "render failed",
			observability.String("post_id", post.ID),
			observability.Err(err),
		)
		return
	}

	// Write JPEG files to disk
	urls, err := s.saveSlides(post.ID, blobs)
	if err != nil {
		s.logger.Error(ctx, "failed to save slides",
			observability.String("post_id", post.ID),
			observability.Err(err),
		)
		return
	}

	// Update slide_urls in DB
	if err := s.repo.UpdateSlideURLs(ctx, post.ID, urls); err != nil {
		s.logger.Error(ctx, "failed to update slide URLs",
			observability.String("post_id", post.ID),
			observability.Err(err),
		)
		return
	}

	// Publish via existing service path
	if err := s.publisher.Publish(ctx, post.ID); err != nil {
		s.logger.Error(ctx, "publish failed",
			observability.String("post_id", post.ID),
			observability.Err(err),
		)
		return
	}

	s.logger.Info(ctx, "auto-published post",
		observability.String("post_id", post.ID),
		observability.Int("slides", len(blobs)),
	)
}

// safePostIDRe matches only alphanumeric characters, hyphens, and underscores.
var safePostIDRe = regexp.MustCompile(`^[A-Za-z0-9\-_]+$`)

// saveSlides writes JPEG blobs to disk and returns their public-accessible URLs.
// Files are written to <mediaDir>/social/<postID>/slide-N.jpg.
// URLs are relative paths (/api/media/social/<postID>/slide-N.jpg).
func (s *SocialPublishScheduler) saveSlides(postID string, blobs [][]byte) ([]string, error) {
	if !safePostIDRe.MatchString(postID) {
		return nil, fmt.Errorf("invalid postID %q: must match [A-Za-z0-9-_]+", postID)
	}
	dir := filepath.Join(s.mediaDir, "social", postID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create slide dir: %w", err)
	}
	urls := make([]string, 0, len(blobs))
	for i, blob := range blobs {
		filename := fmt.Sprintf("slide-%d.jpg", i)
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, blob, 0644); err != nil {
			return nil, fmt.Errorf("write slide %d: %w", i, err)
		}
		urls = append(urls, fmt.Sprintf("/api/media/social/%s/%s", postID, filename))
	}
	return urls, nil
}
