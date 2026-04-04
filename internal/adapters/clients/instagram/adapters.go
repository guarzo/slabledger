package instagram

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// PublisherAdapter wraps the Instagram Client to implement social.Publisher.
type PublisherAdapter struct {
	client *Client
}

// NewPublisherAdapter creates a new publisher adapter.
func NewPublisherAdapter(client *Client) *PublisherAdapter {
	return &PublisherAdapter{client: client}
}

// PublishCarousel publishes a carousel post to Instagram.
func (a *PublisherAdapter) PublishCarousel(ctx context.Context, token, igUserID string, imageURLs []string, caption string) (*social.PublishResultInfo, error) {
	result, err := a.client.PublishCarousel(ctx, token, igUserID, imageURLs, caption)
	if err != nil {
		return nil, err
	}
	return &social.PublishResultInfo{InstagramPostID: result.InstagramPostID}, nil
}

// TokenStore abstracts the Instagram token persistence layer.
type TokenStore interface {
	GetToken(ctx context.Context) (accessToken, igUserID string, expiresAt time.Time, connected bool, err error)
	UpdateToken(ctx context.Context, token string, expiresAt time.Time) error
}

// TokenProvider implements social.InstagramTokenProvider using the token store.
type TokenProvider struct {
	store TokenStore
}

// NewTokenProvider creates a new token provider.
func NewTokenProvider(store TokenStore) *TokenProvider {
	return &TokenProvider{store: store}
}

// GetToken returns the current Instagram credentials.
func (p *TokenProvider) GetToken(ctx context.Context) (string, string, error) {
	token, igUserID, expiresAt, connected, err := p.store.GetToken(ctx)
	if err != nil {
		return "", "", fmt.Errorf("get instagram config: %w", err)
	}
	if !connected {
		return "", "", fmt.Errorf("instagram not connected")
	}
	if time.Now().After(expiresAt) {
		return "", "", fmt.Errorf("instagram token expired — reconnect in Admin settings")
	}
	return token, igUserID, nil
}

// InsightsPollerAdapter wraps the Instagram Client to implement scheduler.InsightsPoller.
// It fetches the current access token from the TokenStore and converts the
// instagram.MediaInsights response into a social.PostMetrics value.
type InsightsPollerAdapter struct {
	client *Client
	store  TokenStore
}

// NewInsightsPollerAdapter creates a new insights poller adapter.
func NewInsightsPollerAdapter(client *Client, store TokenStore) *InsightsPollerAdapter {
	return &InsightsPollerAdapter{client: client, store: store}
}

// PollInsights fetches engagement metrics for a published Instagram post.
func (a *InsightsPollerAdapter) PollInsights(ctx context.Context, mediaID string) (*social.PostMetrics, error) {
	token, _, expiresAt, connected, err := a.store.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get instagram token: %w", err)
	}
	if !connected {
		return nil, fmt.Errorf("instagram not connected")
	}
	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("instagram token expired")
	}

	insights, err := a.client.GetMediaInsights(ctx, token, mediaID)
	if err != nil {
		return nil, err
	}

	return &social.PostMetrics{
		Impressions: insights.Impressions,
		Reach:       insights.Reach,
		Likes:       insights.Likes,
		Comments:    insights.Comments,
		Saves:       insights.Saves,
		Shares:      insights.Shares,
	}, nil
}

// TokenRefresher implements scheduler.InstagramTokenRefresher.
type TokenRefresher struct {
	client *Client
	store  TokenStore
	logger observability.Logger
}

// NewTokenRefresher creates a new token refresher.
func NewTokenRefresher(client *Client, store TokenStore, logger observability.Logger) *TokenRefresher {
	return &TokenRefresher{client: client, store: store, logger: logger}
}

// RefreshIfNeeded refreshes the Instagram token if it will expire within 7 days.
func (r *TokenRefresher) RefreshIfNeeded(ctx context.Context) error {
	token, _, expiresAt, connected, err := r.store.GetToken(ctx)
	if err != nil {
		return err
	}
	if !connected {
		return nil
	}

	daysUntilExpiry := time.Until(expiresAt).Hours() / 24
	if daysUntilExpiry > 7 {
		return nil
	}

	r.logger.Info(ctx, "refreshing Instagram token",
		observability.Int("days_until_expiry", int(daysUntilExpiry)))

	tokenInfo, err := r.client.RefreshToken(ctx, token)
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}

	if err := r.store.UpdateToken(ctx, tokenInfo.AccessToken, tokenInfo.ExpiresAt); err != nil {
		return fmt.Errorf("save refreshed token: %w", err)
	}

	r.logger.Info(ctx, "Instagram token refreshed successfully",
		observability.String("new_expiry", tokenInfo.ExpiresAt.Format(time.RFC3339)))

	return nil
}
