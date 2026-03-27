// Package instagram provides an Instagram Graph API client for OAuth and content publishing.
// Uses the "Instagram Login" path (graph.instagram.com) which does not require a Facebook Page.
package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	// Instagram Login OAuth endpoints
	authURL  = "https://www.instagram.com/oauth/authorize"
	tokenURL = "https://api.instagram.com/oauth/access_token"

	// Graph API endpoints
	graphURL        = "https://graph.instagram.com"
	longLivedURL    = graphURL + "/access_token"
	refreshTokenURL = graphURL + "/refresh_access_token"
)

// TokenInfo holds the result of an OAuth token exchange.
type TokenInfo struct {
	AccessToken string
	UserID      string
	Username    string
	ExpiresAt   time.Time
}

// PublishResult holds the result of publishing a post.
type PublishResult struct {
	InstagramPostID string
}

// Client is an Instagram Graph API client.
type Client struct {
	appID       string
	appSecret   string
	redirectURI string
	httpClient  *httpx.Client
	logger      observability.Logger
}

// NewClient creates a new Instagram API client.
func NewClient(appID, appSecret, redirectURI string, logger observability.Logger) *Client {
	httpCfg := httpx.DefaultConfig("Instagram")
	httpCfg.DefaultTimeout = 30 * time.Second
	return &Client{
		appID:       appID,
		appSecret:   appSecret,
		redirectURI: redirectURI,
		httpClient:  httpx.NewClient(httpCfg),
		logger:      logger,
	}
}

// GetLoginURL returns the Instagram authorization URL.
func (c *Client) GetLoginURL(state string) string {
	params := url.Values{
		"client_id":     {c.appID},
		"redirect_uri":  {c.redirectURI},
		"scope":         {"instagram_business_basic,instagram_business_content_publish"},
		"response_type": {"code"},
		"state":         {state},
	}
	return authURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for a long-lived access token.
// Flow: code → short-lived token → long-lived token, plus fetches user profile.
func (c *Client) ExchangeCode(ctx context.Context, code string) (*TokenInfo, error) {
	shortToken, userID, err := c.exchangeCodeForShortLived(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	longToken, expiresIn, err := c.exchangeForLongLived(ctx, shortToken)
	if err != nil {
		return nil, fmt.Errorf("exchange for long-lived: %w", err)
	}

	username, err := c.getUsername(ctx, longToken, userID)
	if err != nil {
		c.logger.Warn(ctx, "failed to get Instagram username", observability.Err(err))
		username = userID // fallback
	}

	return &TokenInfo{
		AccessToken: longToken,
		UserID:      userID,
		Username:    username,
		ExpiresAt:   time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

// RefreshToken refreshes a long-lived token. Returns the new token and expiry.
func (c *Client) RefreshToken(ctx context.Context, token string) (*TokenInfo, error) {
	params := url.Values{
		"grant_type":   {"ig_refresh_token"},
		"access_token": {token},
	}

	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := c.doGet(ctx, refreshTokenURL+"?"+params.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}

	return &TokenInfo{
		AccessToken: resp.AccessToken,
		ExpiresAt:   time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
	}, nil
}

// PublishCarousel publishes a carousel post with multiple images.
// imageURLs must be publicly accessible HTTPS URLs returning JPEG images.
func (c *Client) PublishCarousel(ctx context.Context, token, igUserID string, imageURLs []string, caption string) (*PublishResult, error) {
	if len(imageURLs) == 0 {
		return nil, fmt.Errorf("no images provided")
	}
	if len(imageURLs) == 1 {
		return c.PublishSingleImage(ctx, token, igUserID, imageURLs[0], caption)
	}

	c.logger.Info(ctx, "publishing carousel",
		observability.Int("imageCount", len(imageURLs)))
	for i, u := range imageURLs {
		c.logger.Info(ctx, "carousel image URL",
			observability.Int("index", i),
			observability.String("url", u))
	}

	var containerIDs []string
	for i, imgURL := range imageURLs {
		containerID, err := c.createItemContainer(ctx, token, igUserID, imgURL)
		if err != nil {
			return nil, fmt.Errorf("create item container for %s: %w", imgURL, err)
		}
		c.logger.Info(ctx, "item container created",
			observability.Int("index", i),
			observability.String("containerID", containerID))

		// Wait for Instagram to finish fetching and processing this image
		// before creating the next container or the carousel.
		if err := c.waitForContainer(ctx, token, containerID); err != nil {
			return nil, fmt.Errorf("wait for item container %s: %w", containerID, err)
		}
		c.logger.Info(ctx, "item container ready",
			observability.Int("index", i),
			observability.String("containerID", containerID))
		containerIDs = append(containerIDs, containerID)
	}

	carouselID, err := c.createCarouselContainer(ctx, token, igUserID, containerIDs, caption)
	if err != nil {
		return nil, fmt.Errorf("create carousel container: %w", err)
	}

	if err := c.waitForContainer(ctx, token, carouselID); err != nil {
		return nil, fmt.Errorf("wait for carousel: %w", err)
	}

	postID, err := c.publishContainer(ctx, token, igUserID, carouselID)
	if err != nil {
		return nil, fmt.Errorf("publish carousel: %w", err)
	}

	return &PublishResult{InstagramPostID: postID}, nil
}

// PublishSingleImage publishes a single image post.
func (c *Client) PublishSingleImage(ctx context.Context, token, igUserID, imageURL, caption string) (*PublishResult, error) {
	params := url.Values{
		"image_url":    {imageURL},
		"caption":      {caption},
		"access_token": {token},
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := c.postForm(ctx, fmt.Sprintf("%s/%s/media", graphURL, igUserID), params, &resp); err != nil {
		return nil, fmt.Errorf("create media container: %w", err)
	}

	if err := c.waitForContainer(ctx, token, resp.ID); err != nil {
		return nil, fmt.Errorf("wait for media: %w", err)
	}

	postID, err := c.publishContainer(ctx, token, igUserID, resp.ID)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}

	return &PublishResult{InstagramPostID: postID}, nil
}

// GetStatus returns the connection status (connected username or empty).
func (c *Client) GetStatus(ctx context.Context, token, igUserID string) (string, error) {
	username, err := c.getUsername(ctx, token, igUserID)
	if err != nil {
		return "", err
	}
	return username, nil
}

// --- internal helpers ---

func (c *Client) exchangeCodeForShortLived(ctx context.Context, code string) (token, userID string, err error) {
	params := url.Values{
		"client_id":     {c.appID},
		"client_secret": {c.appSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {c.redirectURI},
		"code":          {code},
	}

	var resp struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
	}
	if err := c.postForm(ctx, tokenURL, params, &resp); err != nil {
		return "", "", err
	}

	return resp.AccessToken, fmt.Sprintf("%d", resp.UserID), nil
}

func (c *Client) exchangeForLongLived(ctx context.Context, shortToken string) (token string, expiresIn int64, err error) {
	params := url.Values{
		"grant_type":    {"ig_exchange_token"},
		"client_secret": {c.appSecret},
		"access_token":  {shortToken},
	}

	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := c.doGet(ctx, longLivedURL+"?"+params.Encode(), &resp); err != nil {
		return "", 0, err
	}

	return resp.AccessToken, resp.ExpiresIn, nil
}

func (c *Client) getUsername(ctx context.Context, token, userID string) (string, error) {
	reqURL := fmt.Sprintf("%s/%s?fields=username&access_token=%s", graphURL, userID, url.QueryEscape(token))

	var resp struct {
		Username string `json:"username"`
	}
	if err := c.doGet(ctx, reqURL, &resp); err != nil {
		return "", err
	}
	return resp.Username, nil
}

func (c *Client) createItemContainer(ctx context.Context, token, igUserID, imageURL string) (string, error) {
	params := url.Values{
		"image_url":        {imageURL},
		"is_carousel_item": {"true"},
		"access_token":     {token},
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := c.postForm(ctx, fmt.Sprintf("%s/%s/media", graphURL, igUserID), params, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *Client) createCarouselContainer(ctx context.Context, token, igUserID string, childIDs []string, caption string) (string, error) {
	params := url.Values{
		"media_type":   {"CAROUSEL"},
		"children":     {strings.Join(childIDs, ",")},
		"caption":      {caption},
		"access_token": {token},
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := c.postForm(ctx, fmt.Sprintf("%s/%s/media", graphURL, igUserID), params, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *Client) waitForContainer(ctx context.Context, token, containerID string) error {
	for range 30 { // max ~30 seconds
		reqURL := fmt.Sprintf("%s/%s?fields=status_code&access_token=%s", graphURL, containerID, url.QueryEscape(token))

		var resp struct {
			StatusCode string `json:"status_code"`
		}
		if err := c.doGet(ctx, reqURL, &resp); err != nil {
			return err
		}

		switch resp.StatusCode {
		case "FINISHED":
			return nil
		case "ERROR":
			return fmt.Errorf("container %s failed processing", containerID)
		case "PUBLISHED":
			return nil // already published — no need to wait
		case "IN_PROGRESS":
			// Still processing, wait
		default:
			c.logger.Warn(ctx, "unknown container status",
				observability.String("containerID", containerID),
				observability.String("statusCode", resp.StatusCode))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return fmt.Errorf("container %s timed out waiting for FINISHED status", containerID)
}

func (c *Client) publishContainer(ctx context.Context, token, igUserID, containerID string) (string, error) {
	params := url.Values{
		"creation_id":  {containerID},
		"access_token": {token},
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := c.postForm(ctx, fmt.Sprintf("%s/%s/media_publish", graphURL, igUserID), params, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *Client) postForm(ctx context.Context, endpoint string, params url.Values, dest any) error {
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}
	body := []byte(params.Encode())
	resp, err := c.httpClient.Post(ctx, endpoint, headers, body, 0)
	if err != nil {
		if resp != nil {
			if apiErr := parseInstagramError(resp.Body); apiErr != "" {
				return fmt.Errorf("%s: %w", apiErr, err)
			}
		}
		return err
	}
	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return fmt.Errorf("decode Instagram response from %s: %w", endpoint, err)
	}
	return nil
}

func (c *Client) doGet(ctx context.Context, reqURL string, dest any) error {
	resp, err := c.httpClient.Get(ctx, reqURL, nil, 0)
	if err != nil {
		if resp != nil {
			if apiErr := parseInstagramError(resp.Body); apiErr != "" {
				return fmt.Errorf("%s: %w", apiErr, err)
			}
		}
		return err
	}
	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return fmt.Errorf("decode Instagram response from %s: %w", reqURL, err)
	}
	return nil
}

func parseInstagramError(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var apiErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
		return fmt.Sprintf("instagram API error %d: %s", apiErr.Error.Code, apiErr.Error.Message)
	}
	return ""
}
