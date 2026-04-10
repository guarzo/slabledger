package cardladder

import (
	"context"
	"encoding/json"
	"fmt"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

const defaultFunctionsBaseURL = "https://us-central1-cardladder-71d53.cloudfunctions.net"

// BuildCollectionCard resolves a PSA cert number into card metadata
// by calling the httpbuildcollectioncard Cloud Function.
func (c *Client) BuildCollectionCard(ctx context.Context, cert, grader string) (*BuildCardResponse, error) {
	var resp callableResponse[BuildCardResponse]
	err := c.doCallable(ctx, "httpbuildcollectioncard", BuildCardRequest{
		Cert:   cert,
		Grader: grader,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("build collection card for cert %s: %w", cert, err)
	}
	return &resp.Result, nil
}

// CardEstimate fetches the current market estimate for a graded card
// by calling the httpcardestimate Cloud Function.
func (c *Client) CardEstimate(ctx context.Context, req CardEstimateRequest) (*CardEstimateResponse, error) {
	var resp callableResponse[CardEstimateResponse]
	err := c.doCallable(ctx, "httpcardestimate", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("card estimate for %s: %w", req.GemRateID, err)
	}
	return &resp.Result, nil
}

// doCallable calls a Firebase callable Cloud Function using the standard
// {data: ...} / {result: ...} protocol.
func (c *Client) doCallable(ctx context.Context, functionName string, data any, result any) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("get auth token: %w", err)
	}

	u := c.functionsBaseURL() + "/" + functionName
	body := callableRequest{Data: data}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return apperrors.ProviderInvalidRequest("CardLadder", err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	resp, err := c.httpClient.Post(ctx, u, headers, bodyBytes, 0)
	if err != nil {
		// If resp is available, check for Firebase callable error envelope before discarding.
		if resp != nil {
			var callableErr struct {
				Error struct {
					Message string `json:"message"`
					Status  string `json:"status"`
				} `json:"error"`
			}
			if json.Unmarshal(resp.Body, &callableErr) == nil && callableErr.Error.Message != "" {
				return apperrors.ProviderUnavailable("CardLadder", fmt.Errorf("callable %s: %s (status: %s)", functionName, callableErr.Error.Message, callableErr.Error.Status))
			}
		}
		return fmt.Errorf("http request to %s: %w", functionName, err)
	}

	// Check for Firebase callable error envelope before unmarshalling result.
	var callableErr struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(resp.Body, &callableErr) == nil && callableErr.Error.Message != "" {
		return apperrors.ProviderUnavailable("CardLadder", fmt.Errorf("callable %s: %s (status: %s)", functionName, callableErr.Error.Message, callableErr.Error.Status))
	}

	if err := json.Unmarshal(resp.Body, result); err != nil {
		return apperrors.ProviderInvalidResponse("CardLadder", fmt.Errorf("unmarshal %s response: %w", functionName, err))
	}
	return nil
}

// functionsBaseURL returns the Cloud Functions base URL.
// Falls back to the default if none configured.
func (c *Client) functionsBaseURL() string {
	if c.functionsURL != "" {
		return c.functionsURL
	}
	return defaultFunctionsBaseURL
}
