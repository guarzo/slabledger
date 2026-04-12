package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// DHInstagramClient is a narrow interface for DH Instagram generation methods.
// Implemented by *dh.Client.
type DHInstagramClient interface {
	EnterpriseAvailable() bool
	GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error)
	PollInstagramPostStatus(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error)
}

// dhInstagramStrategy holds strategy metadata for dh_instagram post generation.
type dhInstagramStrategy struct {
	key   string // DH API strategy identifier
	title string // Human-readable cover title stored on the post
}

// dhInstagramStrategies is the fixed set of own_inventory strategies run each daily tick.
var dhInstagramStrategies = []dhInstagramStrategy{
	{key: "inventory_top_expensive", title: "Top Expensive Cards"},
	{key: "inventory_top_gainers_week", title: "Top Weekly Gainers"},
	{key: "inventory_top_gainers_month", title: "Top Monthly Gainers"},
	{key: "inventory_pokemon_top_cards", title: "Top Pokémon Cards"},
}

// defaultDHInstagramHashtags is written to all DH-generated posts.
const defaultDHInstagramHashtags = "#pokemon #pokemoncards #pokemontcg #tradingcards #cardcollector"

// DHSocialSchedulerConfig holds runtime parameters for DHSocialScheduler.
type DHSocialSchedulerConfig struct {
	Hour         int           // UTC hour to fire (0–23)
	PollInterval time.Duration // How often to poll DH for render status
	PollTimeout  time.Duration // Max wait before abandoning a DH post
}

// DHSocialScheduler generates Instagram posts via the DH Enterprise Instagram API
// and stores them as dh_instagram draft SocialPosts for the existing publish pipeline.
type DHSocialScheduler struct {
	StopHandle
	dhClient   DHInstagramClient
	socialRepo DHSocialRepo
	logger     observability.Logger
	cfg        DHSocialSchedulerConfig
}

// DHSocialRepo is the minimal subset of social.Repository needed by DHSocialScheduler.
type DHSocialRepo interface {
	CreatePost(ctx context.Context, post *social.SocialPost) error
	UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error
	UpdateSlideURLs(ctx context.Context, id string, urls []string) error
}

// NewDHSocialScheduler constructs a DHSocialScheduler.
func NewDHSocialScheduler(
	dhClient DHInstagramClient,
	socialRepo DHSocialRepo,
	logger observability.Logger,
	cfg DHSocialSchedulerConfig,
) *DHSocialScheduler {
	return &DHSocialScheduler{
		StopHandle: NewStopHandle(),
		dhClient:   dhClient,
		socialRepo: socialRepo,
		logger:     logger.With(context.Background(), observability.String("component", "dh-social")),
		cfg:        cfg,
	}
}

// Start begins the background scheduler, firing once daily at cfg.Hour (UTC).
func (s *DHSocialScheduler) Start(ctx context.Context) {
	initialDelay := timeUntilHour(time.Now(), s.cfg.Hour)

	RunLoop(ctx, LoopConfig{
		Name:         "dh-social",
		Interval:     24 * time.Hour,
		InitialDelay: initialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.tick)
}

func (s *DHSocialScheduler) tick(ctx context.Context) {
	if !s.dhClient.EnterpriseAvailable() {
		s.logger.Info(ctx, "dh social: enterprise key not configured, skipping")
		return
	}

	for _, strategy := range dhInstagramStrategies {
		if err := s.generatePost(ctx, strategy); err != nil {
			s.logger.Warn(ctx, "dh social: failed to generate post",
				observability.String("strategy", strategy.key),
				observability.Err(err),
			)
		}
	}
}

func (s *DHSocialScheduler) generatePost(ctx context.Context, strategy dhInstagramStrategy) error {
	postID, err := s.dhClient.GenerateInstagramPost(ctx, "own_inventory", strategy.key, "")
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	slideURLs, err := s.pollUntilReady(ctx, postID)
	if err != nil {
		return fmt.Errorf("poll post %d: %w", postID, err)
	}

	post := &social.SocialPost{
		PostType:   social.PostTypeDHInstagram,
		Status:     social.PostStatusDraft,
		CoverTitle: strategy.title,
		SlideURLs:  slideURLs,
	}
	if err := s.socialRepo.CreatePost(ctx, post); err != nil {
		return fmt.Errorf("create post: %w", err)
	}

	caption := buildDHCaption(strategy)
	if err := s.socialRepo.UpdatePostCaption(ctx, post.ID, caption, defaultDHInstagramHashtags); err != nil {
		// Non-fatal: post is created, caption update failure just means default empty caption.
		s.logger.Warn(ctx, "dh social: failed to set caption",
			observability.String("post_id", post.ID),
			observability.Err(err),
		)
	}

	s.logger.Info(ctx, "dh social: created draft post",
		observability.String("strategy", strategy.key),
		observability.String("post_id", post.ID),
		observability.Int("slides", len(slideURLs)),
	)
	return nil
}

func (s *DHSocialScheduler) pollUntilReady(ctx context.Context, dhPostID int64) ([]string, error) {
	deadline := time.Now().Add(s.cfg.PollTimeout)
	for time.Now().Before(deadline) {
		status, err := s.dhClient.PollInstagramPostStatus(ctx, dhPostID)
		if err != nil {
			return nil, fmt.Errorf("poll status: %w", err)
		}
		switch status.RenderStatus {
		case "ready":
			return status.SlideImageURLs, nil
		case "failed":
			return nil, fmt.Errorf("DH render failed for post_id %d", dhPostID)
		}
		// Still generating — wait before next poll.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.cfg.PollInterval):
		}
	}
	return nil, fmt.Errorf("timed out waiting for DH post %d to render", dhPostID)
}

// buildDHCaption returns a human-readable caption for a DH Instagram post.
func buildDHCaption(strategy dhInstagramStrategy) string {
	return fmt.Sprintf("%s — check out what's hot in our collection!", strategy.title)
}
