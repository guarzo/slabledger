package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
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
	// The API returns grade in firestore form ("g8"); derive the display form
	// ("PSA 8") that cl_condition and the card label require. Centralizing this
	// here keeps every downstream consumer (refresh, push) unchanged.
	if resp.Result.GemRateCondition != "" {
		resp.Result.Condition = cardutil.ConditionToAPIFormat(resp.Result.GemRateCondition)
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
		return apperrors.ProviderUnavailable("CardLadder", fmt.Errorf("rate limiter: %w", err))
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
		if resp != nil {
			if callableErr := checkCallableError(resp.Body, functionName); callableErr != nil {
				return callableErr
			}
		}
		return fmt.Errorf("http request to %s: %w", functionName, err)
	}

	// Check for Firebase callable error envelope before unmarshalling result.
	if callableErr := checkCallableError(resp.Body, functionName); callableErr != nil {
		return callableErr
	}

	// Check that "result" key is present and non-null before unmarshalling the full response.
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return apperrors.ProviderInvalidResponse("CardLadder", fmt.Errorf("unmarshal %s response: %w", functionName, err))
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return apperrors.ProviderInvalidResponse("CardLadder", fmt.Errorf("callable %s returned nil result", functionName))
	}

	// Now unmarshal the full response into the result parameter.
	if err := json.Unmarshal(resp.Body, result); err != nil {
		return apperrors.ProviderInvalidResponse("CardLadder", fmt.Errorf("unmarshal %s response: %w", functionName, err))
	}
	return nil
}

// checkCallableError checks for Firebase callable error envelopes in response body.
// Returns nil if no error is present.
func checkCallableError(body []byte, functionName string) error {
	var callableErr struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &callableErr) == nil && callableErr.Error.Message != "" {
		detail := fmt.Errorf("callable %s: %s (status: %s)", functionName, callableErr.Error.Message, callableErr.Error.Status)
		// CL's daily quota surfaces as a Firebase RESOURCE_EXHAUSTED envelope
		// (HTTP 200 body, message "Daily request limit reached"). Classify it as
		// a rate limit, not generic unavailability: rate-limit errors are
		// non-retryable (retrying burns more quota) and let callers detect the
		// quota wall and stop the cycle instead of hammering through every
		// remaining card.
		if callableErr.Error.Status == "RESOURCE_EXHAUSTED" || strings.Contains(callableErr.Error.Message, "Daily request limit") {
			return apperrors.ProviderRateLimited("CardLadder", "")
		}
		return apperrors.ProviderUnavailable("CardLadder", detail)
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
