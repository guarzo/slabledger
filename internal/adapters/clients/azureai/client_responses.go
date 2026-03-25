package azureai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// responsesAPIVersion is the minimum API version supporting the Responses API.
const responsesAPIVersion = "2025-04-01-preview"

// --- Request types ---

type responsesRequest struct {
	Model              string          `json:"model,omitempty"`
	Instructions       string          `json:"instructions,omitempty"`
	Input              []any           `json:"input"`
	Tools              []responsesTool `json:"tools,omitempty"`
	Stream             bool            `json:"stream"`
	MaxOutputTokens    int             `json:"max_output_tokens,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
}

// responsesTool is the Responses API tool format — name/description/parameters
// are top-level fields (not nested under "function" like Chat Completions).
type responsesTool struct {
	Type        string `json:"type"` // "function"
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type responsesInputItem struct {
	Type string `json:"type"` // "message", "function_call", "function_call_output"

	// "message" fields
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`

	// "function_call" fields
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// "function_call_output" fields
	Output string `json:"output,omitempty"`
}

// --- SSE event data types ---

type responsesOutputItemAdded struct {
	OutputIndex int `json:"output_index"`
	Item        struct {
		Type   string `json:"type"` // "message" or "function_call"
		ID     string `json:"id"`
		CallID string `json:"call_id"`
		Name   string `json:"name"`
	} `json:"item"`
}

type responsesTextDelta struct {
	Delta string `json:"delta"`
}

type responsesFuncArgsDelta struct {
	ItemID string `json:"item_id"`
	Delta  string `json:"delta"`
}

// --- Request building ---

func (c *Client) buildResponsesRequest(req ai.CompletionRequest) responsesRequest {
	r := responsesRequest{
		Instructions: req.SystemPrompt,
		Stream:       true,
	}
	if id, ok := req.ConversationState.(string); ok {
		r.PreviousResponseID = id
	}

	// Note: temperature is omitted — some models (e.g. gpt-5.4-pro) don't support it.
	if req.MaxTokens > 0 {
		r.MaxOutputTokens = req.MaxTokens
	}

	// AI Foundry uses /openai/v1/responses (model in body).
	// Azure OpenAI uses /openai/deployments/{name}/responses (model in URL).
	if strings.Contains(c.config.Endpoint, ".services.ai.azure.com") {
		r.Model = c.config.DeploymentName
	}

	// Convert domain messages to Responses API input items.
	// When chaining via previous_response_id, the API already has the
	// function_call items from the prior response — re-sending them causes
	// a "Duplicate item" 400 error. Only send function_call_output items
	// and any new user messages in that case.
	chaining := r.PreviousResponseID != ""
	for _, msg := range req.Messages {
		switch msg.Role {
		case ai.RoleUser, ai.RoleAssistant:
			if msg.Content != "" {
				r.Input = append(r.Input, responsesInputItem{
					Type:    "message",
					Role:    string(msg.Role),
					Content: msg.Content,
				})
			}
			if !chaining {
				for _, tc := range msg.ToolCalls {
					// When not chaining via previous_response_id, we must
					// NOT reuse the server-generated fc_... item IDs —
					// the API may already have them and would reject them
					// as duplicates. Generate a fresh ID based on the
					// unique call_id instead.
					r.Input = append(r.Input, responsesInputItem{
						Type:      "function_call",
						ID:        "fc_" + tc.ID, // fresh ID derived from call_id
						CallID:    tc.ID,         // call_... for matching results
						Name:      tc.Name,
						Arguments: tc.Arguments,
					})
				}
			}
		case ai.RoleTool:
			r.Input = append(r.Input, responsesInputItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: msg.Content,
			})
		}
	}

	// Convert tool definitions (Responses API uses flat format, not nested "function").
	// The Responses API requires "properties" on all object schemas, so we normalize.
	for _, td := range req.Tools {
		r.Tools = append(r.Tools, responsesTool{
			Type:        "function",
			Name:        td.Name,
			Description: td.Description,
			Parameters:  ensureProperties(td.Parameters),
		})
	}

	return r
}

// buildResponsesURL returns the Responses API URL.
// For AI Foundry (.services.ai.azure.com), uses /openai/v1/responses (model in body).
// For Azure OpenAI, uses /openai/deployments/{name}/responses?api-version={ver}.
func (c *Client) buildResponsesURL(endpoint string) string {
	if strings.Contains(endpoint, ".services.ai.azure.com") {
		// AI Foundry: model goes in request body, not URL path.
		return endpoint + "/openai/v1/responses"
	}
	return fmt.Sprintf("%s/openai/deployments/%s/responses?api-version=%s",
		endpoint, c.config.DeploymentName, responsesAPIVersion)
}

// --- SSE stream parsing ---

func (c *Client) parseResponsesSSEStream(ctx context.Context, body io.Reader, stream func(ai.CompletionChunk)) error {
	reader := bufio.NewReader(body)

	var currentEvent string
	toolCalls := make(map[string]*ai.ToolCall) // keyed by item ID
	toolCallOrder := make(map[string]int)      // item ID → output_index for sorting

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line, err := reader.ReadString('\n')
		// Trim trailing newline/carriage-return.
		line = strings.TrimRight(line, "\r\n")

		// Process the line even on EOF (may be a final unterminated line).
		if line == "" && err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("reading responses SSE stream: %w", err)
		}

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			if err == io.EOF {
				break
			}
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			if err == io.EOF {
				break
			}
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		switch currentEvent {
		case "response.output_text.delta":
			var d responsesTextDelta
			if jsonErr := json.Unmarshal([]byte(data), &d); jsonErr != nil {
				c.logWarn(ctx, "failed to parse text delta", data, jsonErr)
				continue
			}
			if d.Delta != "" {
				stream(ai.CompletionChunk{Delta: d.Delta})
			}

		case "response.output_item.added":
			var d responsesOutputItemAdded
			if jsonErr := json.Unmarshal([]byte(data), &d); jsonErr != nil {
				c.logWarn(ctx, "failed to parse output item", data, jsonErr)
				continue
			}
			if d.Item.Type == "function_call" {
				toolCalls[d.Item.ID] = &ai.ToolCall{
					ID:     d.Item.CallID, // call_id — for matching tool results
					ItemID: d.Item.ID,     // fc_... — needed when sending function_call back in input
					Name:   d.Item.Name,
				}
				toolCallOrder[d.Item.ID] = d.OutputIndex
			}

		case "response.function_call_arguments.delta":
			var d responsesFuncArgsDelta
			if jsonErr := json.Unmarshal([]byte(data), &d); jsonErr != nil {
				c.logWarn(ctx, "failed to parse func args delta", data, jsonErr)
				continue
			}
			if tc, ok := toolCalls[d.ItemID]; ok {
				tc.Arguments += d.Delta
			}

		case "response.completed":
			// Extract response ID for chaining via previous_response_id.
			// Try nested format first ({"response":{"id":"..."}}) then
			// top-level ({"id":"..."}) — Azure AI Foundry may use either.
			if c.logger != nil {
				c.logger.Debug(ctx, "response.completed raw data", observability.String("data", data))
			}
			var respID string
			var nested struct {
				Response struct {
					ID    string `json:"id"`
					Usage struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
						TotalTokens  int `json:"total_tokens"`
					} `json:"usage"`
				} `json:"response"`
			}
			if jsonErr := json.Unmarshal([]byte(data), &nested); jsonErr != nil {
				c.logWarn(ctx, "failed to parse response.completed", data, jsonErr)
			} else {
				respID = nested.Response.ID
			}
			if respID == "" {
				var topLevel struct {
					ID string `json:"id"`
				}
				if jsonErr := json.Unmarshal([]byte(data), &topLevel); jsonErr == nil {
					respID = topLevel.ID
				}
			}

			chunk := ai.CompletionChunk{
				Done: true,
			}
			if respID != "" {
				chunk.ConversationState = respID
			}
			if nested.Response.Usage.TotalTokens > 0 {
				chunk.Usage = &ai.TokenUsage{
					InputTokens:  nested.Response.Usage.InputTokens,
					OutputTokens: nested.Response.Usage.OutputTokens,
					TotalTokens:  nested.Response.Usage.TotalTokens,
				}
			}
			// Collect tool calls sorted by Azure output index.
			type indexedTC struct {
				idx int
				tc  *ai.ToolCall
			}
			sorted := make([]indexedTC, 0, len(toolCalls))
			for k, tc := range toolCalls {
				sorted = append(sorted, indexedTC{idx: toolCallOrder[k], tc: tc})
			}
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].idx < sorted[j].idx })
			for _, s := range sorted {
				chunk.ToolCalls = append(chunk.ToolCalls, *s.tc)
			}
			stream(chunk)
			return nil

		case "error":
			// All SSE-level errors before content are transient (rate limits,
			// server overload). Return rateLimitError to trigger retry with backoff.
			return &rateLimitError{raw: data}
		}

		currentEvent = ""

		if err == io.EOF {
			break
		}
	}

	// Stream ended without response.completed — this indicates a network
	// disconnect, server timeout, or other interruption. Return an error
	// so the caller knows the response is incomplete.
	c.logWarn(ctx, "SSE stream ended without response.completed", fmt.Sprintf("pending_tool_calls=%d", len(toolCalls)), nil)
	return fmt.Errorf("SSE stream ended without response.completed (possible network interruption)")
}

// ensureProperties ensures all "object" schemas in a tool's parameters have
// a "properties" key. The Responses API rejects schemas with missing properties.
// Works recursively on nested schemas. Accepts the Parameters field (any type)
// and returns a corrected copy.
func ensureProperties(params any) any {
	m, ok := params.(map[string]any)
	if ok {
		return ensurePropertiesMap(m)
	}
	// Try JSON-roundtrip for struct types (like jsonSchema from advisortool).
	b, err := json.Marshal(params)
	if err != nil {
		return params
	}
	var generic map[string]any
	if err := json.Unmarshal(b, &generic); err != nil {
		return params
	}
	return ensurePropertiesMap(generic)
}

func ensurePropertiesMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	// If type=object, ensure properties exists.
	if typ, ok := result["type"].(string); ok && typ == "object" {
		if _, ok := result["properties"]; !ok {
			result["properties"] = map[string]any{}
		} else if result["properties"] == nil {
			result["properties"] = map[string]any{}
		}
	}
	// Recurse into properties values.
	if props, ok := result["properties"].(map[string]any); ok {
		fixed := make(map[string]any, len(props))
		for k, v := range props {
			if sub, ok := v.(map[string]any); ok {
				fixed[k] = ensurePropertiesMap(sub)
			} else {
				fixed[k] = v
			}
		}
		result["properties"] = fixed
	}
	// Recurse into "items" (array element schema).
	if items, ok := result["items"]; ok {
		switch v := items.(type) {
		case map[string]any:
			result["items"] = ensurePropertiesMap(v)
		case []any:
			fixed := make([]any, len(v))
			for i, item := range v {
				if sub, ok := item.(map[string]any); ok {
					fixed[i] = ensurePropertiesMap(sub)
				} else {
					fixed[i] = item
				}
			}
			result["items"] = fixed
		}
	}
	// Recurse into "additionalProperties" if it's a schema.
	if ap, ok := result["additionalProperties"]; ok {
		if sub, ok := ap.(map[string]any); ok {
			result["additionalProperties"] = ensurePropertiesMap(sub)
		}
	}
	// Recurse into combinators: anyOf, oneOf, allOf.
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := result[key].([]any); ok {
			fixed := make([]any, len(arr))
			for i, item := range arr {
				if sub, ok := item.(map[string]any); ok {
					fixed[i] = ensurePropertiesMap(sub)
				} else {
					fixed[i] = item
				}
			}
			result[key] = fixed
		}
	}
	return result
}

func (c *Client) logWarn(ctx context.Context, msg, data string, err error) {
	if c.logger != nil {
		c.logger.Warn(ctx, msg, observability.String("data", data), observability.Err(err))
	}
}
