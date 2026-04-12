package advisor

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

// mockLLMProvider is a test double for ai.LLMProvider.
// Set Responses to control per-call behavior; exhausted calls return a default delta.
type mockLLMProvider struct {
	// Calls tracks the number of times StreamCompletion was called.
	Calls int
	// Responses is a list of functions, each invoked in order.
	Responses []func(req ai.CompletionRequest, stream func(ai.CompletionChunk)) error
}

func (m *mockLLMProvider) StreamCompletion(_ context.Context, req ai.CompletionRequest, stream func(ai.CompletionChunk)) error {
	idx := m.Calls
	m.Calls++
	if idx < len(m.Responses) {
		return m.Responses[idx](req, stream)
	}
	stream(ai.CompletionChunk{Delta: "default response"})
	return nil
}

// mockToolExecutor is a test double for ai.FilteredToolExecutor.
type mockToolExecutor struct {
	// ExecuteFn is called when Execute is invoked.
	ExecuteFn func(ctx context.Context, toolName string, arguments string) (string, error)
	// ToolDefinitions are returned by Definitions and DefinitionsFor.
	ToolDefinitions []ai.ToolDefinition
	// ExecuteCalls records every call made to Execute.
	ExecuteCalls []mockToolCall
}

type mockToolCall struct {
	ToolName  string
	Arguments string
}

func (m *mockToolExecutor) Execute(ctx context.Context, toolName string, arguments string) (string, error) {
	m.ExecuteCalls = append(m.ExecuteCalls, mockToolCall{ToolName: toolName, Arguments: arguments})
	if m.ExecuteFn != nil {
		return m.ExecuteFn(ctx, toolName, arguments)
	}
	return `{"ok": true}`, nil
}

func (m *mockToolExecutor) Definitions() []ai.ToolDefinition {
	return m.ToolDefinitions
}

func (m *mockToolExecutor) DefinitionsFor(names []string) []ai.ToolDefinition {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	var result []ai.ToolDefinition
	for _, d := range m.ToolDefinitions {
		if nameSet[d.Name] {
			result = append(result, d)
		}
	}
	return result
}
