package social

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// generateCaptionAsync runs caption generation in a background goroutine with its own context.
func (s *service) generateCaptionAsync(post *SocialPost) {
	if s.llm == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cards, err := s.repo.ListPostCards(ctx, post.ID)
	if err != nil {
		s.logError(ctx, "list cards for caption", post.PostType, err)
		return
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
		s.logError(ctx, "generate caption", post.PostType, err)
		errCtx, errCancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.repo.UpdatePostCaption(errCtx, post.ID, placeholderCaption, "") //nolint:errcheck // best-effort
		errCancel()
		return
	}

	ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialCaption, nil, start, 0, &usage)

	// Use a fresh context for DB writes so they succeed even if the LLM
	// context is near its 3-minute deadline.
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	title, caption, hashtags := parseCaptionResponse(result.String())
	if err := s.repo.UpdatePostCaption(dbCtx, post.ID, caption, hashtags); err != nil {
		if s.logger != nil {
			s.logger.Error(dbCtx, "social: failed to save generated caption",
				observability.String("postId", post.ID),
				observability.Int("captionLen", len(caption)),
				observability.Err(err))
		}
	}

	// Update cover title if the LLM provided one
	if title != "" {
		if err := s.repo.UpdateCoverTitle(dbCtx, post.ID, title); err != nil {
			if s.logger != nil {
				s.logger.Error(dbCtx, "social: failed to save cover title",
					observability.String("postId", post.ID),
					observability.Err(err))
			}
		}
	}

	if s.logger != nil {
		s.logger.Info(ctx, "social caption generated",
			observability.String("postType", string(post.PostType)),
			observability.Int("captionLen", len(caption)))
	}
}

// generateBackgroundsAsync generates AI background images for a post in a background goroutine.
func (s *service) generateBackgroundsAsync(post *SocialPost) {
	if s.imageGen == nil || s.mediaStore == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cards, err := s.repo.ListPostCards(ctx, post.ID)
	if err != nil {
		s.logError(ctx, "list cards for backgrounds", post.PostType, err)
		return
	}

	postDir := fmt.Sprintf("social/%s", post.ID)
	if err := s.mediaStore.EnsureDir(ctx, postDir); err != nil {
		s.logError(ctx, "create background dir", post.PostType, err)
		return
	}

	quality := s.imageQuality
	if quality == "" {
		quality = "medium"
	}

	var urls []string

	// Generate cover background
	coverPrompt := buildBackgroundPrompt(post.PostType, cards)
	coverResult, err := s.imageGen.GenerateImage(ctx, ai.ImageRequest{
		Prompt:  coverPrompt,
		Size:    "1024x1024",
		Quality: quality,
	})
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "social: cover background generation failed, skipping",
				observability.String("postId", post.ID),
				observability.Err(err))
		}
		urls = append(urls, "")
	} else {
		coverPath := fmt.Sprintf("%s/bg-cover.%s", postDir, coverResult.Format)
		if err := s.mediaStore.WriteFile(ctx, coverPath, coverResult.ImageData); err != nil {
			s.logError(ctx, "save cover background", post.PostType, err)
			urls = append(urls, "")
		} else {
			coverURL := fmt.Sprintf("%s/api/media/social/%s/bg-cover.%s", s.baseURL, post.ID, coverResult.Format)
			urls = append(urls, coverURL)
		}
	}

	// Generate card backgrounds sequentially
	for i, card := range cards {
		if ctx.Err() != nil {
			// Fill remaining slots with empty strings to preserve [cover, card1, ...] alignment
			for j := i; j < len(cards); j++ {
				urls = append(urls, "")
			}
			break
		}
		cardPrompt := buildCardBackgroundPrompt(post.PostType, card)
		cardResult, err := s.imageGen.GenerateImage(ctx, ai.ImageRequest{
			Prompt:  cardPrompt,
			Size:    "1024x1024",
			Quality: quality,
		})
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "social: card background generation failed, skipping",
					observability.String("postId", post.ID),
					observability.Int("cardIndex", i),
					observability.Err(err))
			}
			urls = append(urls, "")
			continue
		}

		cardPath := fmt.Sprintf("%s/bg-%d.%s", postDir, i+1, cardResult.Format)
		if err := s.mediaStore.WriteFile(ctx, cardPath, cardResult.ImageData); err != nil {
			s.logError(ctx, "save card background", post.PostType, err)
			urls = append(urls, "")
			continue
		}
		cardURL := fmt.Sprintf("%s/api/media/social/%s/bg-%d.%s", s.baseURL, post.ID, i+1, cardResult.Format)
		urls = append(urls, cardURL)
	}

	// Store URLs in DB
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	if err := s.repo.UpdateBackgroundURLs(dbCtx, post.ID, urls); err != nil {
		if s.logger != nil {
			s.logger.Error(dbCtx, "social: failed to save background URLs",
				observability.String("postId", post.ID),
				observability.Err(err))
		}
	}

	if s.logger != nil {
		nonEmpty := 0
		for _, u := range urls {
			if u != "" {
				nonEmpty++
			}
		}
		s.logger.Info(ctx, "social backgrounds generated",
			observability.String("postId", post.ID),
			observability.Int("total", len(urls)),
			observability.Int("success", nonEmpty))
	}
}

func (s *service) logError(ctx context.Context, msg string, pt PostType, err error) {
	if s.logger != nil {
		s.logger.Error(ctx, "social: "+msg,
			observability.String("postType", string(pt)),
			observability.Err(err))
	}
}

// safeGo runs fn in a background goroutine with panic recovery.
func (s *service) safeGo(name, postID string, fn func()) {
	s.wg.Go(func() {
		defer func() {
			if r := recover(); r != nil && s.logger != nil {
				s.logger.Error(context.Background(), "social: "+name+" panicked",
					observability.String("postId", postID),
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		fn()
	})
}

func (s *service) Wait() { s.wg.Wait() }

// parseCaption splits AI output into caption text and hashtags line.
func parseCaption(raw string) (caption, hashtags string) {
	raw = strings.TrimSpace(raw)
	lines := strings.Split(raw, "\n")

	hashtagLine := -1
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" && strings.HasPrefix(trimmed, "#") {
			hashtagLine = i
			break
		}
		if trimmed != "" {
			break
		}
	}

	if hashtagLine >= 0 {
		caption = strings.TrimSpace(strings.Join(lines[:hashtagLine], "\n"))
		hashtags = strings.TrimSpace(lines[hashtagLine])
	} else {
		caption = raw
	}

	caption = truncateCaption(caption)
	return caption, hashtags
}

// truncateCaption limits caption length for Instagram (≤300 runes).
// Truncates at the last word boundary if needed. Uses rune-based
// indexing to avoid splitting multi-byte characters (emoji).
func truncateCaption(caption string) string {
	const maxCaptionLen = 300
	runes := []rune(caption)
	if len(runes) <= maxCaptionLen {
		return caption
	}
	truncated := runes[:maxCaptionLen]
	// Find last space in rune-space to avoid splitting words.
	lastSpace := -1
	for i := len(truncated) - 1; i >= maxCaptionLen/2; i-- {
		if truncated[i] == ' ' {
			lastSpace = i
			break
		}
	}
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}
	return strings.TrimSpace(string(truncated)) + "…"
}

// captionResponse is the expected JSON structure from the LLM.
type captionResponse struct {
	Title    string `json:"title"`
	Caption  string `json:"caption"`
	Hashtags string `json:"hashtags"`
}

// parseCaptionResponse extracts title, caption, and hashtags from LLM output.
// Tries JSON parse first; falls back to text splitting (no title) if JSON fails.
func parseCaptionResponse(raw string) (title, caption, hashtags string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}

	cleaned := sanitizeLLMJSON(stripMarkdownFences(raw))

	var resp captionResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err == nil && resp.Caption != "" {
		return strings.TrimSpace(resp.Title), truncateCaption(strings.TrimSpace(resp.Caption)), strings.TrimSpace(resp.Hashtags)
	}

	// Fallback: text-based parsing (no title) — use fence-stripped string
	caption, hashtags = parseCaption(cleaned)
	return "", caption, hashtags
}

// stripMarkdownFences removes ```json / ``` wrappers from LLM output.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// sanitizeLLMJSON fixes common JSON issues from LLM output, such as
// literal newlines and tabs inside string values that break json.Unmarshal.
func sanitizeLLMJSON(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			buf.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			buf.WriteByte(ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				buf.WriteString(`\n`)
			case '\r':
				buf.WriteString(`\r`)
			case '\t':
				buf.WriteString(`\t`)
			default:
				buf.WriteByte(ch)
			}
		} else {
			buf.WriteByte(ch)
		}
	}
	return buf.String()
}

// generateID creates a unique ID using UUID v4.
func generateID() string {
	return uuid.NewString()
}
