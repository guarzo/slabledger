package azureai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// responsesRoutingMiddleware returns middleware that rewrites /openai/responses
// paths to the correct endpoint-specific format. The SDK's Azure middleware
// doesn't include /responses in its known routes, so it falls through to
// path.Join("/openai/", path) → "/openai/responses" which is wrong for both
// endpoint types.
//
// AI Foundry (.services.ai.azure.com): /openai/v1/responses (model in body).
// Azure OpenAI (.openai.azure.com etc): /openai/deployments/{name}/responses.
// responsesRoutingMiddleware returns middleware that fixes the Responses API
// path for Azure endpoints. The SDK's Azure middleware doesn't include
// /responses in its known routes, so it produces an incorrect path:
//   - path.Join("/openai/", basePath+"/responses") where basePath comes from
//     the endpoint URL (e.g. /api/projects/{proj} for AI Foundry).
//
// This middleware detects paths ending in /responses (or /responses/{id}) and
// rewrites them to the correct endpoint-specific format:
//   - AI Foundry: {basePath}/openai/v1/responses (model in body)
//   - Azure OpenAI: /openai/deployments/{name}/responses
func responsesRoutingMiddleware(deploymentName string, isFoundry bool) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		p := req.URL.Path

		// Find /responses or /responses/{id} at the end of the path.
		idx := strings.Index(p, "/responses")
		if idx >= 0 {
			suffix := p[idx+len("/responses"):] // "" or "/{id}"
			basePath := p[:idx]
			// Strip the /openai prefix that the SDK middleware prepends.
			basePath = strings.TrimPrefix(basePath, "/openai")
			if isFoundry {
				// AI Foundry: {basePath}/openai/v1/responses[/{id}]
				// The /v1 path doesn't accept api-version query parameter.
				req.URL.Path = basePath + "/openai/v1/responses" + suffix
				q := req.URL.Query()
				q.Del("api-version")
				req.URL.RawQuery = q.Encode()
			} else {
				// Azure OpenAI: /openai/deployments/{name}/responses[/{id}]
				req.URL.Path = "/openai/deployments/" + deploymentName + "/responses" + suffix
			}
		}
		return next(req)
	}
}

// doStream calls the SDK streaming endpoint and dispatches events to the
// stream callback. Returns the server-assigned response ID (empty if not
// received) and any error encountered.
func (c *Client) doStream(ctx context.Context, params responses.ResponseNewParams, stream func(ai.CompletionChunk)) (string, error) {
	sdkStream := c.client.Responses.NewStreaming(ctx, params)
	defer func() { _ = sdkStream.Close() }()

	var responseID string

	for sdkStream.Next() {
		event := sdkStream.Current()

		switch event.Type {
		case "response.created":
			responseID = event.Response.ID

		case "response.output_text.delta":
			if event.Delta != "" {
				stream(ai.CompletionChunk{Delta: event.Delta})
			}

		case "response.function_call_arguments.done":
			// Tool calls are emitted in the response.completed handler below
			// with full data (ID, Name, Arguments) from the response output.
			// Emitting them here as well would cause duplicates in consumers
			// that accumulate tool calls across chunks.

		case "response.completed":
			chunk := ai.CompletionChunk{
				Done:              true,
				ConversationState: event.Response.ID,
			}
			if event.Response.Usage.TotalTokens > 0 {
				chunk.Usage = &ai.TokenUsage{
					InputTokens:  int(event.Response.Usage.InputTokens),
					OutputTokens: int(event.Response.Usage.OutputTokens),
					TotalTokens:  int(event.Response.Usage.TotalTokens),
				}
			}
			// Collect tool calls from response output items.
			for _, out := range event.Response.Output {
				if out.Type == "function_call" {
					chunk.ToolCalls = append(chunk.ToolCalls, ai.ToolCall{
						ID:        out.CallID,
						ItemID:    out.ID,
						Name:      out.Name,
						Arguments: out.Arguments.OfString,
					})
				}
			}
			stream(chunk)
			return responseID, nil

		case "error":
			return responseID, classifyStreamError(event)
		}
	}

	if err := sdkStream.Err(); err != nil {
		return responseID, classifySDKError(err)
	}

	// Stream ended without response.completed — likely a network interruption.
	return responseID, fmt.Errorf("SSE stream ended without response.completed")
}

// classifyStreamError converts an SSE error event into a typed error for
// retry classification.
func classifyStreamError(event responses.ResponseStreamEventUnion) error {
	if event.Code == "no_capacity" {
		return &capacityError{raw: event.Message}
	}
	return &rateLimitError{raw: event.Message}
}

// classifySDKError converts an SDK stream error into a typed error.
func classifySDKError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "no_capacity") {
		return &capacityError{raw: msg}
	}
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate") {
		return &rateLimitError{raw: msg}
	}
	return err
}

// pollFallback retrieves a stored response via GET when streaming failed.
// It polls up to 10 times (10s apart, with a 20s initial wait to let Azure
// finish processing). Total budget: ~3 minutes.
func (c *Client) pollFallback(ctx context.Context, responseID string, stream func(ai.CompletionChunk)) error {
	for poll := range 10 {
		// Wait before each poll — give Azure time to finish processing.
		wait := 10 * time.Second
		if poll == 0 {
			wait = 20 * time.Second // longer initial wait
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}

		resp, err := c.client.Responses.Get(ctx, responseID, responses.ResponseGetParams{})
		if err != nil {
			msg := err.Error()
			// 404 / not_found — response not stored yet, keep polling.
			if strings.Contains(msg, "404") || strings.Contains(msg, "not_found") {
				if c.logger != nil {
					c.logger.Info(ctx, "poll fallback: response not found yet, will retry",
						observability.Int("poll", poll+1))
				}
				continue
			}
			return err
		}

		switch resp.Status {
		case "completed", "incomplete":
			if resp.Status == "incomplete" {
				reason := "unknown"
				if resp.IncompleteDetails.Reason != "" {
					reason = resp.IncompleteDetails.Reason
				}
				if len(resp.Output) == 0 {
					return fmt.Errorf("response %s: incomplete with no output (reason: %s)", responseID, reason)
				}
				if c.logger != nil {
					c.logger.Warn(ctx, "poll fallback: response incomplete, emitting partial output",
						observability.String("responseID", responseID),
						observability.String("reason", reason),
						observability.Int("outputItems", len(resp.Output)),
					)
				}
			}
			chunk := ai.CompletionChunk{
				Done:              true,
				ConversationState: resp.ID,
			}
			if resp.Usage.TotalTokens > 0 {
				chunk.Usage = &ai.TokenUsage{
					InputTokens:  int(resp.Usage.InputTokens),
					OutputTokens: int(resp.Usage.OutputTokens),
					TotalTokens:  int(resp.Usage.TotalTokens),
				}
			}
			for _, out := range resp.Output {
				switch out.Type {
				case "message":
					for _, c := range out.Content {
						if c.Type == "output_text" && c.Text != "" {
							chunk.Delta += c.Text
						}
					}
				case "function_call":
					chunk.ToolCalls = append(chunk.ToolCalls, ai.ToolCall{
						ID:        out.CallID,
						Name:      out.Name,
						Arguments: out.Arguments.OfString,
					})
				}
			}
			stream(chunk)
			return nil

		case "failed", "cancelled":
			return fmt.Errorf("response %s: status %s", responseID, resp.Status)

		default:
			// "queued", "in_progress" — keep polling.
			if c.logger != nil {
				c.logger.Info(ctx, "poll fallback waiting",
					observability.String("status", string(resp.Status)),
					observability.Int("poll", poll+1))
			}
		}
	}
	return fmt.Errorf("response %s still not completed after polling", responseID)
}
