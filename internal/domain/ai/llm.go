package ai

import "context"

// ToolDefinition describes a tool the LLM can call.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// CompletionRequest is sent to the LLM provider.
type CompletionRequest struct {
	SystemPrompt      string
	Messages          []Message
	Tools             []ToolDefinition
	Temperature       *float64
	MaxTokens         int
	ConversationState any
	ReasoningEffort   string // "low", "medium", "high" — controls reasoning token budget (empty = provider default)
	Store             bool   // when true, enables server-side response storage for retrieval/resume
}

// TokenUsage reports token consumption for a completion request.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// CompletionChunk is a streamed response piece from the LLM.
type CompletionChunk struct {
	Delta             string
	ToolCalls         []ToolCall
	ConversationState any
	Done              bool
	Usage             *TokenUsage
}

// LLMProvider abstracts the LLM API (Azure AI Foundry, OpenAI, etc.).
type LLMProvider interface {
	StreamCompletion(ctx context.Context, req CompletionRequest, stream func(CompletionChunk)) error
}
