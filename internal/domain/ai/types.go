package ai

// Role identifies who sent a message in the LLM conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in an LLM conversation.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
	ToolCallID string     `json:"toolCallId,omitempty"`
}

// ToolCall represents a function call the LLM wants to make.
type ToolCall struct {
	ID        string `json:"id"`
	ItemID    string `json:"itemId,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// EventType identifies the kind of SSE event.
type EventType string

const (
	EventDelta      EventType = "delta"
	EventToolStart  EventType = "tool_start"
	EventToolResult EventType = "tool_result"
	EventDone       EventType = "done"
	EventError      EventType = "error"
)

// StreamEvent is a single event in the SSE response stream.
type StreamEvent struct {
	Type     EventType `json:"type"`
	Content  string    `json:"content,omitempty"`
	ToolName string    `json:"toolName,omitempty"`
}
