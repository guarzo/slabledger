package ai

import "context"

// ToolExecutor executes named tools with JSON arguments and returns JSON results.
type ToolExecutor interface {
	Execute(ctx context.Context, toolName string, arguments string) (string, error)
	Definitions() []ToolDefinition
}

// FilteredToolExecutor extends ToolExecutor with the ability to return
// a subset of tool definitions by name.
type FilteredToolExecutor interface {
	ToolExecutor
	// DefinitionsFor returns only the named tools. Unknown names are silently skipped.
	DefinitionsFor(names []string) []ToolDefinition
}
