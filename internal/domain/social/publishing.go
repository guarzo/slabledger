package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) Publish(ctx context.Context, id string) error {
	if s.publisher == nil || s.tokenProvider == nil {
		return ErrNotConfigured
	}

	// Check for placeholder caption before publishing.
	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}
	if post == nil {
		return ErrPostNotFound
	}
	if post.Caption == placeholderCaption || post.Caption == "" {
		return ErrNotPublishable
	}

	// Atomically set to "publishing" — prevents double-publish.
	// Only draft and failed posts can be published.
	if err := s.repo.SetPublishing(ctx, id); err != nil {
		return err
	}

	// Run the actual Instagram publish in a background goroutine.
	s.safeGo("publishAsync", id, func() { s.publishAsync(id) })

	return nil
}

func (s *service) publishAsync(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Fetch post first (need SlideURLs)
	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		s.setPublishError(ctx, id, fmt.Sprintf("get post: %v", err))
		return
	}
	if post == nil {
		s.setPublishError(ctx, id, "post not found")
		return
	}

	cards, err := s.repo.ListPostCards(ctx, id)
	if err != nil {
		s.setPublishError(ctx, id, fmt.Sprintf("list post cards: %v", err))
		return
	}

	// Prefer rendered slide URLs; fall back to raw card images.
	// Filter empty strings — background generation failures can leave empty
	// entries in SlideURLs (see generateBackgroundsAsync). If all slide URLs
	// are empty after filtering, fall back to card front images.
	var imageURLs []string
	for _, u := range post.SlideURLs {
		if u != "" {
			imageURLs = append(imageURLs, u)
		}
	}
	if len(imageURLs) == 0 {
		for _, c := range cards {
			if c.FrontImageURL != "" {
				imageURLs = append(imageURLs, c.FrontImageURL)
			}
		}
	}
	if len(imageURLs) == 0 {
		s.setPublishError(ctx, id, "no card images available for publishing")
		return
	}

	caption := post.Caption
	if post.Hashtags != "" {
		caption += "\n\n" + post.Hashtags
	}

	token, igUserID, err := s.tokenProvider.GetToken(ctx)
	if err != nil {
		s.setPublishError(ctx, id, fmt.Sprintf("get Instagram token: %v", err))
		return
	}

	result, err := s.publisher.PublishCarousel(ctx, token, igUserID, imageURLs, caption)

	// Use a fresh context for terminal DB writes so the publish timeout
	// doesn't prevent persisting the final state.
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	if err != nil {
		s.setPublishError(dbCtx, id, fmt.Sprintf("publish to Instagram: %v", err))
		return
	}

	if err := s.repo.SetPublished(dbCtx, id, result.InstagramPostID); err != nil {
		// The Instagram post was published successfully but we failed to record it.
		// Use setPublishError so the user sees the situation and avoids re-publishing.
		s.setPublishError(dbCtx, id, fmt.Sprintf(
			"published to Instagram (post %s) but failed to update DB: %v — do NOT re-publish",
			result.InstagramPostID, err))
		return
	}

	if s.logger != nil {
		s.logger.Info(dbCtx, "social post published to Instagram",
			observability.String("postId", id),
			observability.String("instagramPostId", result.InstagramPostID))
	}
}

func (s *service) setPublishError(ctx context.Context, id, errMsg string) {
	if s.logger != nil {
		s.logger.Error(ctx, "social publish failed",
			observability.String("postId", id),
			observability.String("error", errMsg))
	}
	if dbErr := s.repo.SetError(ctx, id, errMsg); dbErr != nil {
		// ErrPostNotFound means the post was deleted while publishing was
		// in flight — nothing to mark, nothing stuck. Quietly move on.
		if errors.Is(dbErr, ErrPostNotFound) {
			return
		}
		if s.logger != nil {
			s.logger.Error(ctx, "social: failed to persist publish error — post stuck in publishing state",
				observability.String("postId", id),
				observability.Err(dbErr))
		}
	}
}

func (s *service) RegenerateCaption(ctx context.Context, id string, stream func(ai.StreamEvent)) error {
	if s.llm == nil {
		return fmt.Errorf("AI caption generation not configured")
	}

	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}
	if post == nil {
		return fmt.Errorf("post not found")
	}

	cards, err := s.repo.ListPostCards(ctx, id)
	if err != nil {
		return fmt.Errorf("list post cards: %w", err)
	}

	userPrompt := buildUserPrompt(post.PostType, cards)
	var result strings.Builder
	var usage ai.TokenUsage
	start := time.Now()

	err = s.llm.StreamCompletion(ctx, ai.CompletionRequest{
		SystemPrompt: captionSystemPrompt,
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: userPrompt},
		},
		MaxTokens: 512,
	}, func(chunk ai.CompletionChunk) {
		result.WriteString(chunk.Delta)
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
	})
	if err != nil {
		ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialCaption, err, start, 0, &usage)
		return fmt.Errorf("stream caption: %w", err)
	}

	ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialCaption, nil, start, 0, &usage)

	title, caption, hashtags := parseCaptionResponse(result.String())
	if updateErr := s.repo.UpdatePostCaption(ctx, id, caption, hashtags); updateErr != nil {
		return fmt.Errorf("save caption: %w", updateErr)
	}
	if title != "" {
		if err := s.repo.UpdateCoverTitle(ctx, id, title); err != nil {
			return fmt.Errorf("save cover title: %w", err)
		}
	}

	// Send the parsed result as a single "done" event with all fields in JSON content
	resultJSON, err := json.Marshal(map[string]string{
		"caption":  caption,
		"hashtags": hashtags,
		"title":    title,
	})
	if err != nil {
		return fmt.Errorf("marshal caption result: %w", err)
	}
	stream(ai.StreamEvent{Type: ai.EventDone, Content: string(resultJSON)})
	return nil
}

func (s *service) ListPosts(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error) {
	return s.repo.ListPosts(ctx, status, limit, offset)
}

func (s *service) GetPost(ctx context.Context, id string) (*PostDetail, error) {
	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, nil
	}

	cards, err := s.repo.ListPostCards(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list post cards: %w", err)
	}

	return &PostDetail{
		SocialPost: *post,
		Cards:      cards,
	}, nil
}

func (s *service) UpdateCaption(ctx context.Context, id string, caption, hashtags string) error {
	return s.repo.UpdatePostCaption(ctx, id, caption, hashtags)
}

func (s *service) Delete(ctx context.Context, id string) error {
	return s.repo.DeletePost(ctx, id)
}
