package social

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Service defines operations for social media content generation.
type Service interface {
	// DetectAndGenerate runs detection for all post types and creates draft posts.
	// Returns the number of new posts created.
	DetectAndGenerate(ctx context.Context) (int, error)

	// ListPosts returns posts filtered by optional status.
	ListPosts(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error)

	// GetPost returns a post with its card details.
	GetPost(ctx context.Context, id string) (*PostDetail, error)

	// UpdateCaption updates the caption and hashtags of a draft.
	UpdateCaption(ctx context.Context, id string, caption, hashtags string) error

	// Delete removes a post and its card associations.
	Delete(ctx context.Context, id string) error

	// Publish starts async publishing of a post to Instagram.
	// Sets status to "publishing" and returns immediately.
	// The actual publish happens in a background goroutine.
	Publish(ctx context.Context, id string) error

	// RegenerateCaption regenerates the AI caption for a draft, streaming via callback.
	RegenerateCaption(ctx context.Context, id string, stream func(ai.StreamEvent)) error

	// Wait blocks until all background caption tasks have finished.
	Wait()
}

// ServiceOption configures optional service dependencies.
type ServiceOption func(*service)

// WithLLM sets the LLM provider for AI caption generation.
func WithLLM(llm ai.LLMProvider) ServiceOption {
	return func(s *service) { s.llm = llm }
}

// WithLogger enables structured logging.
func WithLogger(l observability.Logger) ServiceOption {
	return func(s *service) { s.logger = l }
}

// Publisher abstracts Instagram publishing so the domain doesn't depend on the client directly.
type Publisher interface {
	PublishCarousel(ctx context.Context, token, igUserID string, imageURLs []string, caption string) (*PublishResultInfo, error)
}

// PublishResultInfo holds the result of publishing a post.
type PublishResultInfo struct {
	InstagramPostID string
}

// InstagramTokenProvider provides the current Instagram connection credentials.
type InstagramTokenProvider interface {
	GetToken(ctx context.Context) (token, igUserID string, err error)
}

// WithPublisher sets the Instagram publisher.
func WithPublisher(p Publisher, tp InstagramTokenProvider) ServiceOption {
	return func(s *service) {
		s.publisher = p
		s.tokenProvider = tp
	}
}

// WithAITracker enables recording of AI call metrics.
func WithAITracker(t ai.AICallTracker) ServiceOption {
	return func(s *service) { s.tracker = t }
}

// WithImageGenerator enables AI background image generation.
func WithImageGenerator(ig ai.ImageGenerator, quality, mediaDir, baseURL string) ServiceOption {
	return func(s *service) {
		s.imageGen = ig
		s.imageQuality = quality
		s.mediaDir = mediaDir
		s.baseURL = baseURL
	}
}

// WithMediaStore sets the media storage backend for generated images.
func WithMediaStore(ms MediaStore) ServiceOption {
	return func(s *service) { s.mediaStore = ms }
}
