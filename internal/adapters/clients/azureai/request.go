package azureai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// useResponsesAPI returns true for AI Foundry endpoints that require the Responses API
// (newer models like gpt-5.4-pro only support this API).
func (c *Client) useResponsesAPI() bool {
	return strings.Contains(c.config.Endpoint, ".services.ai.azure.com")
}

// doStreamCompletion performs a single streaming completion call.
// Returns the server-assigned response ID (Responses API only, empty otherwise) and any error.
func (c *Client) doStreamCompletion(ctx context.Context, req ai.CompletionRequest, stream func(ai.CompletionChunk)) (string, error) {
	endpoint := strings.TrimRight(c.config.Endpoint, "/")

	var body []byte
	var url string
	var err error

	if c.useResponsesAPI() {
		apiReq := c.buildResponsesRequest(req)
		// Log request shape for diagnostics.
		if c.logger != nil {
			fcCount := 0
			fcoCount := 0
			msgCount := 0
			for _, item := range apiReq.Input {
				if v, ok := item.(responsesInputItem); ok {
					switch v.Type {
					case "function_call":
						fcCount++
					case "function_call_output":
						fcoCount++
					case "message":
						msgCount++
					}
				}
			}
			c.logger.Info(ctx, "responses api request",
				observability.String("previousResponseID", apiReq.PreviousResponseID),
				observability.Int("inputItems", len(apiReq.Input)),
				observability.Int("functionCalls", fcCount),
				observability.Int("functionCallOutputs", fcoCount),
				observability.Int("messages", msgCount),
			)
		}
		body, err = json.Marshal(apiReq)
		url = c.buildResponsesURL(endpoint)
	} else {
		apiReq := c.buildRequest(req)
		body, err = json.Marshal(apiReq)
		url = c.buildURL(endpoint)
	}
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Azure OpenAI domains use "api-key" header; other endpoints use Bearer token.
	if isAzureOpenAI(endpoint) {
		httpReq.Header.Set("api-key", c.config.APIKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response

	if resp.StatusCode == http.StatusTooManyRequests {
		respBody, _ := io.ReadAll(resp.Body) //nolint:errcheck
		return "", &rateLimitError{raw: string(respBody)}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		if c.logger != nil {
			c.logger.Error(ctx, "azure ai request failed",
				observability.Int("status", resp.StatusCode),
				observability.Int("request_body_size", len(body)),
			)
		}
		return "", fmt.Errorf("azure ai returned %d: %s", resp.StatusCode, string(respBody))
	}

	if c.useResponsesAPI() {
		sr, parseErr := c.parseResponsesSSEStream(ctx, resp.Body, stream)
		return sr.responseID, parseErr
	}
	return "", c.parseSSEStream(ctx, resp.Body, stream)
}

// buildRequest converts our domain types to the Azure OpenAI API format.
func (c *Client) buildRequest(req ai.CompletionRequest) chatCompletionRequest {
	var messages []apiMessage

	if req.SystemPrompt != "" {
		messages = append(messages, apiMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	for _, msg := range req.Messages {
		apiMsg := apiMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}
		if msg.ToolCallID != "" {
			apiMsg.ToolCallID = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				apiMsg.ToolCalls = append(apiMsg.ToolCalls, apiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: apiFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}
		messages = append(messages, apiMsg)
	}

	var tools []apiTool
	for _, td := range req.Tools {
		tools = append(tools, apiTool{
			Type: "function",
			Function: apiToolFunction{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Parameters,
			},
		})
	}

	ccr := chatCompletionRequest{
		Messages:    messages,
		Tools:       tools,
		Stream:      true,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	// AI Foundry inference API requires the model name in the request body.
	// Azure OpenAI encodes it in the URL path instead.
	if !isAzureOpenAI(c.config.Endpoint) {
		ccr.Model = c.config.DeploymentName
	}

	return ccr
}

// isAzureOpenAI returns true if the endpoint is an Azure OpenAI-compatible service
// that uses the /openai/deployments/{name}/chat/completions URL pattern and api-key auth.
// Matches: .openai.azure.com, .cognitiveservices.azure.com, and .services.ai.azure.com (AI Foundry).
func isAzureOpenAI(endpoint string) bool {
	return strings.Contains(endpoint, ".openai.azure.com") ||
		strings.Contains(endpoint, ".cognitiveservices.azure.com") ||
		strings.Contains(endpoint, ".services.ai.azure.com")
}

// buildURL returns the chat completions URL for the configured endpoint.
// Azure OpenAI: {endpoint}/openai/deployments/{deployment}/chat/completions?api-version={version}
// AI Foundry:   {endpoint}/chat/completions
func (c *Client) buildURL(endpoint string) string {
	if isAzureOpenAI(endpoint) {
		return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
			endpoint, c.config.DeploymentName, c.config.APIVersion)
	}
	return endpoint + "/chat/completions"
}
