package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

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
	if resp.Username == "" {
		return "", apperrors.ProviderInvalidResponse("Instagram",
			fmt.Errorf("user info returned empty username"))
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
	if resp.ID == "" {
		return "", apperrors.ProviderInvalidResponse("Instagram",
			fmt.Errorf("container creation returned empty ID"))
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
	if resp.ID == "" {
		return "", apperrors.ProviderInvalidResponse("Instagram",
			fmt.Errorf("carousel container creation returned empty ID"))
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
	if resp.ID == "" {
		return "", apperrors.ProviderInvalidResponse("Instagram",
			fmt.Errorf("publish returned empty media ID"))
	}
	return resp.ID, nil
}

func (c *Client) postForm(ctx context.Context, endpoint string, params url.Values, dest any) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return apperrors.ProviderUnavailable("Instagram", fmt.Errorf("rate limiter: %w", err))
	}
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
	return decodeJSON(resp.Body, dest, endpoint)
}

func (c *Client) doGet(ctx context.Context, reqURL string, dest any) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return apperrors.ProviderUnavailable("Instagram", fmt.Errorf("rate limiter: %w", err))
	}
	resp, err := c.httpClient.Get(ctx, reqURL, nil, 0)
	if err != nil {
		if resp != nil {
			if apiErr := parseInstagramError(resp.Body); apiErr != "" {
				return fmt.Errorf("%s: %w", apiErr, err)
			}
		}
		return err
	}
	return decodeJSON(resp.Body, dest, reqURL)
}

func decodeJSON(body []byte, dest any, endpoint string) error {
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode Instagram response from %s: %w", endpoint, err)
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
