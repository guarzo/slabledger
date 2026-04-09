package azureai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Config configures the Azure AI Foundry client.
type Config struct {
	Endpoint       string // https://<resource>.openai.azure.com
	APIKey         string // Azure API key
	DeploymentName string // e.g. "gpt-4o"
	APIVersion     string // e.g. "2024-12-01-preview"
}

// responsesAPIVersion is the minimum API version supporting the Responses API.
const responsesAPIVersion = "2025-04-01-preview"

// Option configures the client.
type Option func(*clientOptions)

// clientOptions holds optional configuration for the client.
type clientOptions struct {
	logger  observability.Logger
	baseURL string // for testing with httptest
}

// WithLogger sets the logger.
func WithLogger(l observability.Logger) Option {
	return func(o *clientOptions) { o.logger = l }
}

// withBaseURL sets the base URL for the SDK client (unexported, for tests).
func withBaseURL(url string) Option {
	return func(o *clientOptions) { o.baseURL = url }
}

// Client implements ai.LLMProvider for Azure AI Foundry using the openai-go SDK.
type Client struct {
	client         *openai.Client
	deploymentName string
	logger         observability.Logger
}

var _ ai.LLMProvider = (*Client)(nil)

// rateLimitError signals a 429 from the API (HTTP or SSE stream).
type rateLimitError struct{ raw string }

func (e *rateLimitError) Error() string {
	return "responses API rate limited: " + e.raw
}

// capacityError signals an Azure "no_capacity" error — the request exceeds the
// maximum usage size allowed during peak load on pay-as-you-go tier.
type capacityError struct{ raw string }

func (e *capacityError) Error() string {
	return "azure ai capacity exceeded: " + e.raw
}

const maxStreamRetries = 5

// NewClient creates a new Azure AI Foundry client backed by the openai-go SDK.
func NewClient(cfg Config, opts ...Option) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("azureai: Endpoint is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("azureai: APIKey is required")
	}
	if cfg.DeploymentName == "" {
		return nil, fmt.Errorf("azureai: DeploymentName is required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = responsesAPIVersion
	}

	var co clientOptions
	for _, opt := range opts {
		opt(&co)
	}

	// Build SDK client options. Disable SDK-level retries — we manage our own
	// retry loop with backoff classification in StreamCompletion.
	sdkOpts := []option.RequestOption{
		azure.WithEndpoint(cfg.Endpoint, cfg.APIVersion),
		azure.WithAPIKey(cfg.APIKey),
		option.WithMiddleware(responsesRoutingMiddleware(cfg.DeploymentName)),
		option.WithMaxRetries(0),
	}
	if co.baseURL != "" {
		sdkOpts = append(sdkOpts, option.WithBaseURL(co.baseURL))
	}

	client := openai.NewClient(sdkOpts...)

	return &Client{
		client:         &client,
		deploymentName: cfg.DeploymentName,
		logger:         co.logger,
	}, nil
}

// StreamCompletion streams a completion response from Azure AI using the
// Responses API. Retries automatically on transient errors with exponential
// backoff. When a response ID is captured and store=true, falls back to
// polling on stream failure.
func (c *Client) StreamCompletion(ctx context.Context, req ai.CompletionRequest, stream func(ai.CompletionChunk)) error {
	params := c.buildParams(req)

	var lastErr error
	var lastResponseID string

	for attempt := range maxStreamRetries {
		completed := false
		wrappedStream := func(chunk ai.CompletionChunk) {
			if chunk.Done {
				completed = true
			}
			stream(chunk)
		}

		respID, err := c.doStream(ctx, params, wrappedStream)
		if respID != "" {
			lastResponseID = respID
		}
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't retry after completion or on last attempt.
		if completed || attempt == maxStreamRetries-1 {
			break
		}
		// Don't retry permanent client errors (4xx except 429).
		if isPermanentError(lastErr) {
			break
		}
		// If we captured a response ID and store is enabled, skip further
		// retries and go straight to poll fallback to avoid duplicate POSTs.
		if lastResponseID != "" && req.Store {
			break
		}

		backoff := classifyBackoff(lastErr, attempt)
		if c.logger != nil {
			c.logger.Warn(ctx, "retrying after transient error",
				observability.Int("attempt", attempt+1),
				observability.Int("backoffSec", int(backoff.Seconds())),
				observability.Err(lastErr),
			)
		}
		select {
		case <-ctx.Done():
			lastErr = ctx.Err()
			goto pollFallback
		case <-time.After(backoff):
		}
	}

pollFallback:
	// Poll-based fallback: if streaming failed but we captured a response ID
	// and the request used store=true, the response may have completed server-side.
	if lastResponseID != "" && req.Store {
		pollCtx, pollCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer pollCancel()
		if c.logger != nil {
			c.logger.Info(pollCtx, "attempting poll fallback for stored response",
				observability.String("responseID", lastResponseID))
		}
		if pollErr := c.pollFallback(pollCtx, lastResponseID, stream); pollErr == nil {
			return nil
		} else {
			if c.logger != nil {
				c.logger.Warn(pollCtx, "poll fallback failed", observability.Err(pollErr))
			}
			return pollErr
		}
	}

	return lastErr
}

// buildParams maps a domain CompletionRequest to SDK ResponseNewParams.
func (c *Client) buildParams(req ai.CompletionRequest) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(c.deploymentName),
	}

	if req.SystemPrompt != "" {
		params.Instructions = openai.String(req.SystemPrompt)
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if req.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(req.MaxTokens))
	}
	if req.Store {
		params.Store = openai.Bool(true)
	}
	if req.ReasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffort(req.ReasoningEffort),
		}
	}
	if id, ok := req.ConversationState.(string); ok && id != "" {
		params.PreviousResponseID = openai.String(id)
	}

	// Convert tool definitions.
	for _, td := range req.Tools {
		toolParams := ensureProperties(td.Parameters)
		params.Tools = append(params.Tools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        td.Name,
				Description: openai.String(td.Description),
				Parameters:  toolParams.(map[string]any),
			},
		})
	}

	// Convert messages to input items.
	chaining := false
	if id, ok := req.ConversationState.(string); ok && id != "" {
		chaining = true
	}

	var items responses.ResponseInputParam
	for _, msg := range req.Messages {
		switch msg.Role {
		case ai.RoleUser:
			items = append(items, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role:    responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(msg.Content)},
				},
			})

		case ai.RoleAssistant:
			if msg.Content != "" {
				items = append(items, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role:    responses.EasyInputMessageRoleAssistant,
						Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(msg.Content)},
					},
				})
			}
			if !chaining {
				for _, tc := range msg.ToolCalls {
					items = append(items, responses.ResponseInputItemUnionParam{
						OfFunctionCall: &responses.ResponseFunctionToolCallParam{
							ID:        openai.String("fc_" + tc.ID),
							CallID:    tc.ID,
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
			}

		case ai.RoleTool:
			items = append(items, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: msg.ToolCallID,
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: openai.String(msg.Content),
					},
				},
			})
		}
	}

	if len(items) > 0 {
		params.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: items,
		}
	}

	return params
}

// classifyBackoff returns the backoff duration based on error type and attempt.
func classifyBackoff(err error, attempt int) time.Duration {
	var capErr *capacityError
	var rlErr *rateLimitError
	if errors.As(err, &capErr) {
		return time.Duration(60<<attempt) * time.Second // 60s, 120s, 240s
	}
	if errors.As(err, &rlErr) {
		return time.Duration(30<<attempt) * time.Second // 30s, 60s, 120s
	}
	return time.Duration(5<<attempt) * time.Second // 5s, 10s, 20s
}

// isPermanentError returns true for non-retriable client errors (4xx except 429).
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}
	if errors.As(err, new(*rateLimitError)) || errors.As(err, new(*capacityError)) {
		return false
	}
	msg := err.Error()
	// SDK error format: '400 Bad Request', '401 Unauthorized', etc.
	if strings.Contains(msg, "400 Bad Request") ||
		strings.Contains(msg, "401 Unauthorized") ||
		strings.Contains(msg, "403 Forbidden") ||
		strings.Contains(msg, "404 Not Found") ||
		strings.Contains(msg, "405 Method Not Allowed") ||
		strings.Contains(msg, "422 Unprocessable") {
		return true
	}
	// Also check for generic status code patterns.
	if strings.Contains(msg, "status code: 400") ||
		strings.Contains(msg, "status code: 401") ||
		strings.Contains(msg, "status code: 403") ||
		strings.Contains(msg, "status code: 404") ||
		strings.Contains(msg, "status code: 405") ||
		strings.Contains(msg, "status code: 422") {
		return true
	}
	return false
}

// isAzureOpenAI returns true if the endpoint is an Azure OpenAI-compatible service
// that uses the /openai/deployments/{name}/chat/completions URL pattern and api-key auth.
// Matches: .openai.azure.com, .cognitiveservices.azure.com, and .services.ai.azure.com (AI Foundry).
func isAzureOpenAI(endpoint string) bool {
	return strings.Contains(endpoint, ".openai.azure.com") ||
		strings.Contains(endpoint, ".cognitiveservices.azure.com") ||
		strings.Contains(endpoint, ".services.ai.azure.com")
}
